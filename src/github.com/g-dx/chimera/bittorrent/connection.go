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
	keepAlivePeriod   = 1 * time.Minute
	DIAL_TIMEOUT        = 10 * time.Second
	handshakeTimeout   = 5 * time.Second
	ONE_HUNDRED_MILLIS  = 100 * time.Millisecond
	FIVE_HUNDRED_MILLIS = 5 * ONE_HUNDRED_MILLIS

	oneMegaByte = 1 * 1024 * 1024
	fiveHundredMills = 500 * time.Millisecond

	errHashesNotEquals          = errors.New("Info hashes not equal")
	errHandshakeNotReceived     = errors.New(fmt.Sprintf("Handshake not received in: %v", handshakeTimeout))
	errKeepAliveExpired         = errors.New("Read KeepAlive expired")
)

////////////////////////////////////////////////////////////////////////////////////////////////
// PeerError - captures a low-level error specific to a peer
////////////////////////////////////////////////////////////////////////////////////////////////

type PeerError struct {
	err error
	id  *PeerIdentity
}

func (pe *PeerError) Error() error {
	return pe.err
}

func (pe *PeerError) Id() *PeerIdentity {
	return pe.id
}

////////////////////////////////////////////////////////////////////////////////////////////////
// PeerIdentity - represents a unique name for a connection to a peer
////////////////////////////////////////////////////////////////////////////////////////////////

type PeerIdentity struct {
	id      [20]byte // from handshake
	address string   // ip:port
}

func (pi *PeerIdentity) Equals(ip *PeerIdentity) bool {
	return pi.id == ip.id && pi.address == ip.address
}

func (pi *PeerIdentity) String() string {
	return pi.address
}

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
		in: IncomingPeerConnection{ c, nil, conn, nil },
		out: OutgoingPeerConnection{ c, nil, conn, make([]byte, 0), nil},
	}
}


func (pc *PeerConnection) Establish(in <-chan ProtocolMessage,
	out chan<- *MessageList,
	e chan<- PeerError,
	handshake *HandshakeMessage,
	w io.Writer,
    outgoing bool) (*PeerIdentity, error) {

	pc.in.log = log.New(w, " in  ->", log.Ldate|log.Ltime)
	pc.out.logger = log.New(w, " out <-" + pc.in.conn.RemoteAddr().String() + " => ", log.Ldate|log.Ltime|log.Lmicroseconds)
	pc.logger = log.New(w, "  -  --" + pc.in.conn.RemoteAddr().String() + " => ", log.Ldate|log.Ltime|log.Lmicroseconds)

	// Ensure we handshake properly
	id, err := pc.completeHandshake(handshake, outgoing)
	if err != nil {
		pc.logger.Println(err)
		return nil, err
	}

	// Connect up channels
	pc.in.c = out
	pc.out.c = in

	// Create buffers
	inBuf := bytes.NewBuffer(make([]byte, 0, oneMegaByte))
	//out := bytes.NewBuffer(make([]byte, 0, oneMegaByte))

	// Start goroutines
	go pc.in.loop(inBuf, e, id)
	go pc.out.loop(e, id)

	pc.logger.Println("Established")
	return id, nil
}

func (pc *PeerConnection) completeHandshake(handshake *HandshakeMessage, outgoing bool) (pi *PeerIdentity, err error) {

	// TODO: we should possibly be first reading if this is an incoming connection
	// Write outgoing handshake & attempt to read incoming handshake
	handshakeBuf := bytes.NewBuffer(make([]byte, 0, handshakeLength))
	if outgoing {
		pc.logger.Printf("Writing handshake for [%v]", handshakeTimeout)
		pc.out.curr = WriteHandshake(handshake)
		pc.out.writeOrReceiveFor(handshakeTimeout)
		pc.logger.Printf("Written handshake")
		n, err := pc.in.readFor(handshakeBuf, handshakeTimeout)
		if err != nil {
			return nil, err
		}
		pc.logger.Printf("Read [%v] handshake bytes", n)
		if handshakeBuf.Len() != int(handshakeLength) {
			return nil, errHandshakeNotReceived
		}
	} else {
		pc.logger.Printf("Waiting for [%v] for handshake", handshakeTimeout)
		n, err := pc.in.readFor(handshakeBuf, 2500 * time.Millisecond)
		if err != nil {
			return nil, err
		}
		pc.logger.Printf("Read [%v] handshake bytes", n)
		if handshakeBuf.Len() != int(handshakeLength) {
			return nil, errHandshakeNotReceived
		}
		pc.logger.Printf("Read handshake")
		pc.out.curr = WriteHandshake(handshake)
		pc.out.writeOrReceiveFor(handshakeTimeout)
		pc.logger.Printf("Written handshake - done")
	}

	// Create buffer for handshake

	// Read handshake & assert hashes
	inHandshake := ReadHandshake(handshakeBuf.Bytes())
	if !bytes.Equal(handshake.infoHash, inHandshake.infoHash) {
		return nil, errHashesNotEquals
	}

	// Create ID
	id := &PeerIdentity{address: pc.in.conn.RemoteAddr().String()}
	copy(id.id[:], inHandshake.infoHash)
	return id, nil
}

func (pc *PeerConnection) Close() error {

	pc.logger.Println("Closing connection")

	// Shutdown reader & writer
	pc.close <- struct{}{}
	pc.close <- struct{}{}

	// Finally, shutdown connection
	err := pc.in.conn.Close()
	if err != nil {
		pc.logger.Println(err)
		return err
	}
	pc.logger.Println("Connection closed")
	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Incoming Connection
////////////////////////////////////////////////////////////////////////////////////////////////

type IncomingPeerConnection struct {
	done <-chan struct{}
	c    chan<- *MessageList
	conn net.Conn
	log  *log.Logger
}

func (ic *IncomingPeerConnection) loop(buf *bytes.Buffer, err chan<- PeerError, id *PeerIdentity) {
	defer onLoopExit(err, id)

	keepAlive := time.NewTimer(keepAlivePeriod)
	for {
		// Read for a max of 500ms
		n, err := ic.readFor(buf, fiveHundredMills)
		if !isTimeout(err) {
			panic(err)
		}

		// If we read some bytes, reset keepalive
		if n > 0 {
			keepAlive.Reset(keepAlivePeriod)
		}

		// Decode messages & send if we have any
		l := ic.decodeMessages(buf, id)
		if l != nil {
			select {
			case <-keepAlive.C:
				panic(errKeepAliveExpired)
			case <-ic.done:
				ic.log.Println("Connection closed")
				return
			case ic.c <- l:
			}
		}
	}
	ic.log.Println("Loop exit")
}

func (ic *IncomingPeerConnection) readFor(buf *bytes.Buffer, d time.Duration) (int64, error) {
	t := time.Now().Add(d)
	ic.log.Printf("Set read deadline for [%v]", t)
	ic.conn.SetReadDeadline(t)
	n, err := buf.ReadFrom(ic.conn)
	return n, err
}


 func (ic *IncomingPeerConnection) decodeMessages(buf *bytes.Buffer, id *PeerIdentity) *MessageList {
	var msgs *MessageList
	var msg ProtocolMessage
	for {
		msg = Unmarshal(buf)
		if msg == nil {
			break
		}
		if msgs == nil {
			msgs = NewMessageList(id)
		}
		if msg == KeepAlive {
			continue
		}

		// Log and append
		ic.log.Print(msg)
		msgs.Add(msg)
	}
	return msgs
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Outgoing Connection
////////////////////////////////////////////////////////////////////////////////////////////////

type OutgoingPeerConnection struct {
	close  <-chan struct{}
	c      <-chan ProtocolMessage
	conn   net.Conn
	curr   []byte
	logger *log.Logger
}

func (oc *OutgoingPeerConnection) loop(err chan<- PeerError, id *PeerIdentity) {
	defer onLoopExit(err, id)
	var keepAlive = time.After(keepAlivePeriod)
	for {
		c := oc.maybeEnableReceive()
		select {
		case <-oc.close:
			break
		case <-keepAlive:
			oc.append(KeepAlive)
		case msg := <-c:
			oc.append(msg)
		default:
			if n := oc.writeOrReceiveFor(FIVE_HUNDRED_MILLIS); n > 0 {
				keepAlive = time.After(keepAlivePeriod)
			}
		}
	}
	oc.logger.Println("Loop exit")
}

func (oc *OutgoingPeerConnection) maybeEnableReceive() <-chan ProtocolMessage {
	var c <-chan ProtocolMessage
	if len(oc.curr) == 0 {
		c = oc.c
	}
	return c
}

func (oc *OutgoingPeerConnection) append(msg ProtocolMessage) {
	oc.logger.Print(msg)

	buffer := make([]byte, int(4+msg.Len()))
	Marshal(msg, buffer)
	oc.curr = append(oc.curr, buffer...)
}

func (oc *OutgoingPeerConnection) writeOrReceiveFor(d time.Duration) (bytes int) {

	if len(oc.curr) == 0 {

		// Receive until timeout
		select {
		case msg := <-oc.c:
			buffer := make([]byte, int(4+msg.Len()))
			Marshal(msg, buffer)
			oc.curr = buffer
		case <-time.After(d):
		}
	} else {

		// Set deadline and write until timeout
		t := time.Now().Add(ONE_HUNDRED_MILLIS)
		oc.conn.SetWriteDeadline(t)
		oc.logger.Printf("Set write deadline for [%v]", t)
		timeout := false
		n := 0
		for !timeout && len(oc.curr) > 0 {
			oc.curr, n, timeout = write(oc.conn, oc.curr)
			oc.logger.Printf("Written [%v] bytes", n)
			bytes += n
		}
	}
	oc.logger.Printf("Total write [%v] bytes", bytes)
	return bytes
}

func onLoopExit(c chan<- PeerError, id *PeerIdentity) {
	if r := recover(); r != nil {
		if _, ok := r.(runtime.Error); ok {
			panic(r)
		}
		c <- PeerError{err: r.(error), id: id}
	}
}

// Actually does the write & checks for timeout
func write(w io.Writer, buf []byte) ([]byte, int, bool) {
	timeout := false
	n, err := w.Write(buf)
	if nil != err {
		panicIfNotTimeout(err)
		timeout = true
	}
	return buf[n:], n, timeout
}

func isTimeout(err error) bool {
	opErr, ok := err.(*net.OpError);
	return ok && opErr.Timeout()
}

// Panic if the error is not a timeout
func panicIfNotTimeout(err error) {
	if opErr, ok := err.(*net.OpError); ok && !opErr.Timeout() || !ok {
		panic(err)
	}
}

type ConnectionListener struct {
	l net.Listener
	conns chan<- *PeerConnection
	done chan struct{}
}

func NewConnectionListener(conns chan<- *PeerConnection, laddr string) (*ConnectionListener, error) {

	// Create listener
	ln, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, err
	}

	cl := &ConnectionListener{ ln, conns, make(chan struct{}) }
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
