package bittorrent

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"runtime"
	"time"
)

var (
	MAX_OUTGOING_BUFFER = 25 // max pending messages to write out on connection
	MAX_INCOMING_BUFFER = 25 // max pending messages to send upstream
	READ_BUFFER_SIZE    = 4096

	ONE_SECOND          = 1 * time.Second
	KEEP_ALIVE_PERIOD   = 1 * time.Minute
	DIAL_TIMEOUT        = 10 * time.Second
	HANDSHAKE_TIMEOUT   = 5 * time.Second
	ONE_HUNDRED_MILLIS  = 100 * time.Millisecond
	FIVE_HUNDRED_MILLIS = 5 * ONE_HUNDRED_MILLIS

	errHashesNotEquals          = errors.New("Info hashes not equal")
	errHandshakeNotReceived     = errors.New(fmt.Sprintf("Handshake not received in: %v", HANDSHAKE_TIMEOUT))
	errFirstMessageNotHandshake = errors.New("First message is not handshake")
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

	c := make(chan struct{}, 2) // 2 close messages - one for reader, other for writer
	pc := &PeerConnection{
		close: c,
		in: IncomingPeerConnection{
			close:         c,
			c:             nil,
			conn:          conn,
			buffer:        make([]byte, 0),
			pending:       make([]ProtocolMessage, 0, MAX_INCOMING_BUFFER),
			readHandshake: false,
		},
		out: OutgoingPeerConnection{
			close: c,
			c:     nil,
			conn:  conn,
			curr:  make([]byte, 0),
		},
	}
	return pc, nil
}

func (pc *PeerConnection) Establish(in <-chan ProtocolMessage,
	out chan<- ProtocolMessage,
	e chan<- PeerError,
	handshake *HandshakeMessage,
	logDir string) (*PeerIdentity, error) {

	// Create log file & create loggers
	f, err := os.Create(fmt.Sprintf("%v/%v.log", logDir, pc.in.conn.RemoteAddr()))
	if err != nil {
		return nil, err
	}
	pc.in.logger = log.New(f, " in  ->", log.Ldate|log.Ltime)
	pc.out.logger = log.New(f, " out <-", log.Ldate|log.Ltime)
	pc.logger = log.New(f, "  -  --", log.Ldate|log.Ltime)

	// Ensure we handshake properly
	id, err := pc.completeHandshake(handshake)
	if err != nil {
		pc.logger.Println(err)
		return nil, err
	}

	// Connect up channels
	pc.in.c = out
	pc.out.c = in

	// Start goroutines
	go pc.in.loop(e, id)
	go pc.out.loop(e, id)

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
	//	pc.out.append(outHandshake)
	pc.out.writeOrReceiveFor(HANDSHAKE_TIMEOUT)
	pc.in.readOrSleepFor(HANDSHAKE_TIMEOUT)
	if len(pc.in.pending) == 0 {
		return nil, errHandshakeNotReceived
	}
	_ = pc.in.pending[0]
	pc.in.pending = pc.in.pending[1:]

	// Assert handshake
	var inHandshake HandshakeMessage
	var ok bool
	//	inHandshake, ok := msg.(*HandshakeMessage)
	if !ok {
		return nil, errFirstMessageNotHandshake
	}

	// Assert hashes
	if !bytes.Equal(outHandshake.infoHash, inHandshake.infoHash) {
		return nil, errHashesNotEquals
	}

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
	close         <-chan struct{}
	c             chan<- ProtocolMessage
	conn          net.Conn
	buffer        []byte
	pending       []ProtocolMessage
	readHandshake bool
	logger        *log.Logger
}

func (ic *IncomingPeerConnection) loop(err chan<- PeerError, id *PeerIdentity) {
	defer onLoopExit(err, id)
	var keepAlive = time.After(KEEP_ALIVE_PERIOD)
	for {
		c, next := ic.maybeEnableSend()
		select {
		case <-ic.close:
			break
		case <-keepAlive:
			panic(errKeepAliveExpired)
		case c <- next:
			ic.pending = ic.pending[1:]
		default:
			if n := ic.readOrSleepFor(ONE_SECOND); n > 0 {
				keepAlive = time.After(KEEP_ALIVE_PERIOD)
				ic.maybeReadMessage(id)
			}
		}
	}
	ic.logger.Println("Loop exit")
}

func (ic *IncomingPeerConnection) readOrSleepFor(d time.Duration) int {
	if len(ic.pending) >= MAX_INCOMING_BUFFER {
		ic.logger.Println("Incoming buffer full - sleeping...")
		time.Sleep(d) // too many outstanding messages
		return 0
	}

	// Set deadline, read as much as possible & attempt to unmarshal message
	ic.conn.SetReadDeadline(time.Now().Add(d))
	buf, n := read(ic.conn)
	ic.buffer = append(ic.buffer, buf...)
	return n
}

func (ic *IncomingPeerConnection) maybeEnableSend() (chan<- ProtocolMessage, ProtocolMessage) {
	var c chan<- ProtocolMessage
	var next ProtocolMessage
	if len(ic.pending) > 0 {
		next = ic.pending[0]
		c = ic.c
	}
	return c, next
}

func (ic *IncomingPeerConnection) maybeReadMessage(id *PeerIdentity) {
	var msg ProtocolMessage
	ic.buffer, msg = Unmarshal(id, ic.buffer)

	if msg != nil {
		ic.logger.Print(msg)
	}

	// Remove keepalive
	if msg != nil && msg != KeepAlive {
		ic.pending = append(ic.pending, msg)
	}
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
	var keepAlive = time.After(KEEP_ALIVE_PERIOD)
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
				keepAlive = time.After(KEEP_ALIVE_PERIOD)
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
	if opErr, ok := err.(*net.OpError); ok && !opErr.Timeout() || !ok {
		panic(err)
	}
}
