package bittorrent

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"runtime"
	"time"
)

var (
	keepAlivePeriod     = 1 * time.Minute
	DIAL_TIMEOUT        = 10 * time.Second
	handshakeTimeout    = 5 * time.Second

	oneMegaByte      = 1 * 1024 * 1024
	fiveHundredMills = 500 * time.Millisecond
	_160KB = _16KB * 10

	errHashesNotEquals      = errors.New("Info hashes not equal")
	errHandshakeNotReceived = errors.New(fmt.Sprintf("Handshake not received in: %v", handshakeTimeout))
	errKeepAliveExpired     = errors.New("Read KeepAlive expired")
)

////////////////////////////////////////////////////////////////////////////////////////////////
// PeerError - captures a low-level error specific to a peer
////////////////////////////////////////////////////////////////////////////////////////////////

type PeerError struct {
	err error
	id  PeerIdentity
}

func (pe *PeerError) Error() error {
	return pe.err
}

func (pe *PeerError) Id() PeerIdentity {
	return pe.id
}

////////////////////////////////////////////////////////////////////////////////////////////////
// PeerIdentity - represents a unique name for a connection to a peer
////////////////////////////////////////////////////////////////////////////////////////////////

type PeerIdentity string
const nilPeerId = PeerIdentity("")

////////////////////////////////////////////////////////////////////////////////////////////////
// PeerConnection - represents a read+write connection to a peer
////////////////////////////////////////////////////////////////////////////////////////////////

type PeerConnection struct {
	close  chan<- struct{}
	in     IncomingPeerConnection
	out    OutgoingPeerConnection
	logger *log.Logger
}

func NewConnection(addr string) (*PeerConnection, error) {

	fmt.Printf("Connecting to: %v\n", addr)
	conn, err := net.DialTimeout("tcp", addr, DIAL_TIMEOUT)
	if err != nil {
		return nil, err
	}

	// Configure TCP buffer to equal the read buffer
	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetReadBuffer(oneMegaByte)
	}

	return createConnection(conn), nil
}

func createConnection(conn net.Conn) *PeerConnection {
	c := make(chan struct{}, 2) // 2 close messages - one for reader, other for writer
	return &PeerConnection{
		close: c,
		in:    IncomingPeerConnection{c, nil, conn, nil},
		out:   OutgoingPeerConnection{c, nil, conn, nil},
	}
}

func (pc *PeerConnection) Establish(handshake *HandshakeMessage, outgoing bool) (PeerIdentity, error) {

	// Ensure we handshake properly
	id, err := pc.completeHandshake(handshake, outgoing)
	if err != nil {
		pc.logger.Println(err)
		return nilPeerId, err
	}
	return id, nil
}

func (pc *PeerConnection) Start(id PeerIdentity, in chan ProtocolMessage, out chan *MessageList, e chan<- PeerError, w io.Writer) {

	pc.in.log = log.New(w, " in  ->", log.Ldate|log.Ltime)
	pc.out.logger = log.New(w, " out <-"+pc.in.conn.RemoteAddr().String()+" => ", log.Ldate|log.Ltime|log.Lmicroseconds)
	pc.logger = log.New(w, "  -  --"+pc.in.conn.RemoteAddr().String()+" => ", log.Ldate|log.Ltime|log.Lmicroseconds)

	// Connect up channels
	pc.in.c = out
	pc.out.c = in

	// Create buffers
	// TODO: These values should come from parameter config
	inBuf := bytes.NewBuffer(make([]byte, 0, oneMegaByte))
	outBuf := bytes.NewBuffer(make([]byte, 0, _160KB))

	// Start goroutines
	go pc.in.loop(inBuf, e, id)
	go pc.out.loop(outBuf, e, id)

	pc.logger.Println("Started")
}

func (pc *PeerConnection) completeHandshake(handshake *HandshakeMessage, outgoing bool) (pi PeerIdentity, err error) {

	var inHandshake *HandshakeMessage
	if outgoing {
		WriteHandshake(pc.out.conn, handshake)
		inHandshake, err = pc.readHandshake()
		if err != nil {
			return nilPeerId, err
		}
	} else {
		inHandshake, err = pc.readHandshake()
		if err != nil {
			return nilPeerId, err
		}
		WriteHandshake(pc.out.conn, handshake)
	}

	// Assert hashes
	if !bytes.Equal(handshake.infoHash, inHandshake.infoHash) {
		return nilPeerId, errHashesNotEquals
	}

	// Create ID
	id := fmt.Sprintf("%v:%x", pc.in.conn.RemoteAddr().String(), inHandshake.infoHash)
	return PeerIdentity(id), nil
}

func (pc *PeerConnection) readHandshake() (in *HandshakeMessage, err error) {

	// Need to read handshake with a different timeout
	buf := make([]byte, handshakeLength)
	pc.in.conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
	_, err = pc.in.conn.Read(buf)
	if err != nil {
		return nil, err
	}
	if len(buf) != int(handshakeLength) {
		return nil, errHandshakeNotReceived
	}
	return ReadHandshake(buf), nil
}

func (pc *PeerConnection) Close() error {

	pc.logger.Println("Closing connection")

	// Shutdown reader & writer
	close(pc.close)

	// TODO: Should really be waiting until connection read or write times out

	// Finally, shutdown connection
	err := pc.in.conn.Close()
	if err != nil {
		pc.logger.Println(err)
		return err
	}
	pc.logger.Println("Connection closed")
	return nil
}

func (pc *PeerConnection) Send(msg ProtocolMessage) {
	pc.out.c <- msg
}

func (pc *PeerConnection) Receive() *MessageList {
	return <-pc.in.c
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Incoming Connection
////////////////////////////////////////////////////////////////////////////////////////////////

type IncomingPeerConnection struct {
	done <-chan struct{}
	c    chan *MessageList
	conn net.Conn
	log  *log.Logger
}

func (ic *IncomingPeerConnection) loop(buf *bytes.Buffer, err chan<- PeerError, id PeerIdentity) {
	defer onLoopExit(err, id)

	keepAlive := time.NewTimer(keepAlivePeriod)
	for {
		// Read for a max of 500ms
		ic.conn.SetReadDeadline(time.Now().Add(fiveHundredMills))
		n, err := buf.ReadFrom(ic.conn)
		ic.log.Printf("Read [%v] bytes, error: %v", n, err)

		if err != nil && !isTimeout(err) {
			panic(err)
		}

		// If we read some bytes, reset keepalive
		if n > 0 {
			keepAlive.Reset(keepAlivePeriod)
		}

		// Decode messages & send if we have any
		msgs := Unmarshal(buf)
		if len(msgs) == 0 {
			continue
		}

		l := &MessageList{msgs, id}
		ic.log.Printf("Decoded [%v] messages", l)
		select {
		case <-keepAlive.C:
			panic(errKeepAliveExpired)
		case <-ic.done:
			ic.log.Println("Connection closed")
			return
		case ic.c <- l:
		}
	}
	ic.log.Println("Loop exit")
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Outgoing Connection
////////////////////////////////////////////////////////////////////////////////////////////////

type OutgoingPeerConnection struct {
	close  <-chan struct{}
	c      chan ProtocolMessage
	conn   net.Conn
	logger *log.Logger
}

func (oc *OutgoingPeerConnection) loop(buf *bytes.Buffer, err chan<- PeerError, id PeerIdentity) {
	defer onLoopExit(err, id)
	keepAlive := time.After(keepAlivePeriod)
	flushTimer := time.Tick(fiveHundredMills)

	b := make([]byte, _16KB+13) // Max size = Block(_16Kb)
	for {
		select {
		case <-oc.close:
			break

		case <-flushTimer:
			// Flush buffer
			n, err := oc.conn.Write(buf.Bytes())
			if err != nil {
				panic(err) // TODO: Should only error in genuine case?
			}
			buf.Next(n)
			keepAlive = time.After(keepAlivePeriod)

		case <-keepAlive:
			n := Marshal(KeepAlive{}, b)
			buf.Write(b[:n])

		case msg := <-oc.c:
			// Flush buffer if no space
			if (_160KB-buf.Len()) < 4+Len(msg) {
				n, err := oc.conn.Write(buf.Bytes())
				if err != nil {
					panic(err) // TODO: Should only error in genuine case?
				}
				buf.Next(n)
				keepAlive = time.After(keepAlivePeriod)
			}
			// Write message into buffer
			n := Marshal(msg, b)
			buf.Write(b[:n])
		}
	}
	oc.logger.Println("Loop exit")
}

func onLoopExit(c chan<- PeerError, id PeerIdentity) {
	if r := recover(); r != nil {
		if _, ok := r.(runtime.Error); ok {
			panic(r)
		}
		c <- PeerError{err: r.(error), id: id}
	}
}

func isTimeout(err error) bool {
	opErr, ok := err.(*net.OpError)
	return ok && opErr.Timeout()
}

type ConnectionListener struct {
	l     net.Listener
	conns chan<- *PeerConnection
	done  chan struct{}
}

func NewConnectionListener(conns chan<- *PeerConnection, laddr string) (*ConnectionListener, error) {

	// Create listener
	ln, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, err
	}

	cl := &ConnectionListener{ln, conns, make(chan struct{})}
	go cl.loop()
	return cl, nil
}

func (cl *ConnectionListener) loop() {
	for {
		conn, err := cl.l.Accept()
		if err != nil {
			select {
			case <-cl.done:
				return
			default:
				log.Printf("Error on accept: %v", err)
				continue
			}
		}

		cl.conns <- createConnection(conn)
	}
}

func (cl *ConnectionListener) Close() error {
	close(cl.done)
	return cl.l.Close()
}
