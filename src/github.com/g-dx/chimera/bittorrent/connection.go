package bittorrent

import (
	"net"
	"fmt"
	"time"
	"io"
	"runtime"
	"bytes"
	"errors"
	"log"
	"os"
)

var (
	MAX_OUTGOING_BUFFER = 25 // max pending messages to write out on connection
	MAX_INCOMING_BUFFER = 25 // max pending messages to send upstream
	READ_BUFFER_SIZE = 4096

	ONE_SECOND = 1 * time.Second
	KEEP_ALIVE_PERIOD = 1 * time.Minute
	DIAL_TIMEOUT = 10 * time.Second
	HANDSHAKE_TIMEOUT = 5 * time.Second
	ONE_HUNDRED_MILLIS = 100 * time.Millisecond
	FIVE_HUNDRED_MILLIS = 5 * ONE_HUNDRED_MILLIS

	errHashesNotEquals = errors.New("Info hashes not equal")
	errHandshakeNotReceived =
		errors.New(fmt.Sprintf("Handshake not received in: %v", HANDSHAKE_TIMEOUT))
	errFirstMessageNotHandshake =
		errors.New("First message is not handshake")
	errKeepAliveExpired =
		errors.New("Read KeepAlive expired")
)

type PeerIdentity struct {
	id []byte      // from handshake
	address string // ip:port
}

func (pi PeerIdentity) Equals(ip PeerIdentity) bool {
	return bytes.Equal(pi.id, ip.id) && pi.address == ip.address
}

func (pi PeerIdentity) String() string {
	return pi.address
}

type PeerConnection struct {
	close chan<- struct{}
	in IncomingPeerConnection
	out OutgoingPeerConnection
	logger *log.Logger
}

func NewConnection(addr string) (*PeerConnection, error) {

	fmt.Printf("Connecting to: %v\n", addr)
	conn, err := net.DialTimeout("tcp", addr, DIAL_TIMEOUT)
	if err != nil {
		return nil, err
	}

	c := make(chan struct{}, 2) // 2 close messages - one for reader, other for writer
	pc := &PeerConnection{
		close : c,
		in : IncomingPeerConnection {
			close : c,
			c : nil,
			conn : conn,
			buffer : make([]byte, 0),
			pending : make([]*ProtocolMessageWithId, 0, MAX_INCOMING_BUFFER),
			readHandshake : false,
			id : PeerIdentity { nil, "" },
		},
		out : OutgoingPeerConnection {
			close : c,
			c : nil,
			conn : conn,
			curr : make([]byte, 0),
		},
	}
	return pc, nil
}

func (pc * PeerConnection) Establish(in <-chan ProtocolMessage,
									 out chan<- *ProtocolMessageWithId,
									 e chan<- error,
                                     handshake *HandshakeMessage,
									 logDir string) (*PeerIdentity, error) {

	// Create log file & create loggers
	f, err := os.Create(fmt.Sprintf("%v/%v.log", logDir, pc.in.conn.RemoteAddr()))
	if err != nil {
		return nil, err
	}
	pc.in.logger  = log.New(f, " in  ->", log.Ldate | log.Ltime)
	pc.out.logger = log.New(f, " out <-", log.Ldate | log.Ltime)
	pc.logger     = log.New(f, "  -  --", log.Ldate | log.Ltime)

	// Ensure we handshake properly
	id, err := pc.completeHandshake(handshake)
	if err != nil {
		pc.logger.Println(err)
		return nil, err
	}

	// Connect up channels
	pc.in.c = out
	pc.in.id = *id
	pc.out.c = in

	// Start goroutines
	go pc.in.loop(e)
	go pc.out.loop(e)

	pc.logger.Println("Established")
	return id, nil
}

func (pc *PeerConnection) completeHandshake(outHandshake *HandshakeMessage) (pi *PeerIdentity, err error) {
	// TODO: Refactor this to common logic...
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				panic(r)
			}
			err = r.(error)
		}
	}()

	// TODO: we should possibly be first reading if this is an incoming connection
	// Write outgoing handshake & attempt to read incoming handshake
	pc.out.append(outHandshake)
	pc.out.writeOrReceiveFor(HANDSHAKE_TIMEOUT)
	pc.in.readOrSleepFor(HANDSHAKE_TIMEOUT)
	if len(pc.in.pending) == 0 {
		return nil, errHandshakeNotReceived
	}
	msg := pc.in.pending[0]
	pc.in.pending = pc.in.pending[1:]

	// Assert handshake
	inHandshake, ok := msg.Msg().(*HandshakeMessage)
	if !ok {
		return nil, errFirstMessageNotHandshake
	}

	// Assert hashes
	if !bytes.Equal(outHandshake.infoHash, inHandshake.infoHash) {
		return nil, errHashesNotEquals
	}

	return &PeerIdentity { inHandshake.infoHash, pc.in.conn.RemoteAddr().String() }, nil
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
	close <-chan struct{}
	c chan<- *ProtocolMessageWithId
	conn net.Conn
	buffer []byte
	pending []*ProtocolMessageWithId
	readHandshake bool
	logger *log.Logger
	id PeerIdentity
}

func (ic * IncomingPeerConnection) loop(err chan<- error) {
	defer onLoopExit(err)
	var keepAlive = time.After(KEEP_ALIVE_PERIOD)
	for {
		c, next := ic.maybeEnableSend()
		select {
		case <- ic.close: break
		case <- keepAlive:
			err <- errKeepAliveExpired
			break
		case c <- next: ic.pending = ic.pending[1:]
		default:
			if n := ic.readOrSleepFor(ONE_SECOND); n > 0 {
				keepAlive = time.After(KEEP_ALIVE_PERIOD)
			}
		}
	}
	ic.logger.Println("Loop exit")
}

func (ic * IncomingPeerConnection) readOrSleepFor(d time.Duration) (int) {
	if len(ic.pending) >= MAX_INCOMING_BUFFER {
		ic.logger.Println("Incoming buffer full - sleeping...")
		time.Sleep(d)	// too many outstanding messages
		return 0
	}

	// Set deadline, read as much as possible & attempt to unmarshal message
	ic.conn.SetReadDeadline(time.Now().Add(d))
	buf, n := read(ic.conn)
	ic.buffer = append(ic.buffer, buf...)
	ic.maybeReadMessage()
	return n
}

func (ic * IncomingPeerConnection) maybeEnableSend() (chan<- *ProtocolMessageWithId, *ProtocolMessageWithId) {
	var c chan<- *ProtocolMessageWithId
	var next *ProtocolMessageWithId
	if len(ic.pending) > 0 {
		next = ic.pending[0]
		c = ic.c
	}
	return c, next
}

func (ic * IncomingPeerConnection) maybeReadMessage() {
	var msg *ProtocolMessageWithId
	if !ic.readHandshake {
		ic.buffer, msg = ReadHandshake(ic.buffer, ic.id)
		if msg != nil {
			ic.readHandshake = true
		}
	} else {
		ic.buffer, msg = UnmarshalWithId(ic.buffer, ic.id)
	}

	if msg != nil {
		ic.logger.Print(msg)
	}

	// Remove keepalive
	if msg != nil && msg.Msg() != KeepAliveMessage {
		ic.pending = append(ic.pending, msg)
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Outgoing Connection
////////////////////////////////////////////////////////////////////////////////////////////////

type OutgoingPeerConnection struct {
	close <-chan struct{}
	c <-chan ProtocolMessage
	conn net.Conn
	curr []byte
	logger *log.Logger
}

func (oc * OutgoingPeerConnection) loop(err chan<- error) {
	defer onLoopExit(err)
	var keepAlive = time.After(KEEP_ALIVE_PERIOD)
	for {
		c := oc.maybeEnableReceive()
		select {
		case <- oc.close: break
		case <- keepAlive: oc.append(KeepAliveMessage)
		case msg := <- c: oc.append(msg)
		default:
			if n := oc.writeOrReceiveFor(FIVE_HUNDRED_MILLIS); n > 0 {
				keepAlive = time.After(KEEP_ALIVE_PERIOD)
			}
		}
	}
	oc.logger.Println("Loop exit")
}

func (oc * OutgoingPeerConnection) maybeEnableReceive() <-chan ProtocolMessage {
	var c <-chan ProtocolMessage
	if len(oc.curr) == 0 {
		c = oc.c
	}
	return c
}

func (oc * OutgoingPeerConnection) append(msg ProtocolMessage) {
	oc.logger.Print(msg)
	oc.curr = append(oc.curr, Marshal(msg)...)
}

func (oc * OutgoingPeerConnection) writeOrReceiveFor(d time.Duration) (bytes int) {

	if len(oc.curr) == 0 {

		// Receive until timeout
		select {
		case msg := <- oc.c: oc.curr = Marshal(msg)
		case <- time.After(d):
		}
	} else {

		// Set deadline and write until timeout
		oc.conn.SetWriteDeadline(time.Now().Add(ONE_HUNDRED_MILLIS))
		timeout := false
		n := 0
		for !timeout && len(oc.curr) > 0 {
			oc.curr, n, timeout = write(oc.conn, oc.curr)
			bytes += n
		}
	}
	return bytes
}

func onLoopExit(c chan<- error) {
	if r := recover(); r != nil {
		if _, ok := r.(runtime.Error); ok {
			panic(r)
		}
		c <- r.(error)
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

// Actually does the write & checks for timeout
func read(r io.Reader) ([]byte, int) {
	buf := make([]byte, READ_BUFFER_SIZE)
	n, err := r.Read(buf)
	if nil != err {
		panicIfNotTimeout(err)
	}
	return buf[:n], n
}

// Panic if the error is not a timeout
func panicIfNotTimeout(err error) {
	if opErr, ok := err.(*net.OpError); (ok && !opErr.Timeout() || !ok) {
		panic(err)
	}
}
