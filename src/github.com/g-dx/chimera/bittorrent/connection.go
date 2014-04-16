package bittorrent

import (
	"net"
//	"bufio"
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

	errHashesNotEquals = errors.New("Info hashes not equal")
	errHandshakeNotReceived =
		errors.New(fmt.Sprintf("Handshake not received in: %v", HANDSHAKE_TIMEOUT))
	errFirstMessageNotHandshake =
		errors.New("First message is not handshake")
)

type PeerIdentity struct {
	id []byte      // from handshake
	address string // ip:port
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

	// TODO: Fix this close logic...
	c := make(chan struct{}, 2) // 2 close messages - one for reader, other for writer
	pc := &PeerConnection{
		close : c,
		in : IncomingPeerConnection {
			close : c,
			c : nil,
			conn : conn,
			buffer : make([]byte, 0),
			pending : make([]ProtocolMessage, 0, MAX_INCOMING_BUFFER),
			readHandshake : false,
		},
		out : OutgoingPeerConnection {
			close : c,
			c : nil,
			conn : conn,
			pending : make([]ProtocolMessage, 0, MAX_OUTGOING_BUFFER),
			curr : make([]byte, 0),
		},
	}
	return pc, nil
}

func (pc * PeerConnection) Establish(in <-chan ProtocolMessage,
									 out chan<- ProtocolMessage,
									 e chan<- error,
                                     handshake *HandshakeMessage,
									 logDir string) (*PeerIdentity, error) {


	// Create log file & create loggers
	f, err := os.Create(fmt.Sprintf("%v/%v.log", logDir, pc.in.conn.RemoteAddr()))
	if err != nil {
		return nil, err
	}
	pc.in.logger  = log.New(f, " <- ", log.Ldate | log.Ltime)
	pc.out.logger = log.New(f, " -> ", log.Ldate | log.Ltime)
	pc.logger     = log.New(f, " -- ", log.Ldate | log.Ltime)

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
	go pc.in.loop(e)
	go pc.out.loop(e)

	pc.logger.Println("Established")
	return id, nil
}

func (pc *PeerConnection) completeHandshake(outHandshake *HandshakeMessage) (*PeerIdentity, error) {

	// TODO: we should possibly be first reading if this is an incoming connection
	// Attempt to write outgoing handshake
	pc.out.add(outHandshake)
	pc.out.writeOrSleepFor(HANDSHAKE_TIMEOUT)
	if len(pc.out.curr) != 0 {
		return nil, errHandshakeNotReceived
	}

	// Attempt to read incoming handshake
	pc.in.readOrSleepFor(HANDSHAKE_TIMEOUT)
	if len(pc.in.pending) == 0 {
		return nil, errHandshakeNotReceived
	}
	msg := pc.in.pending[0]
	pc.in.pending = pc.in.pending[1:]

	// Assert handshake
	inHandshake, ok := msg.(*HandshakeMessage)
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

	// TODO: Should a channel be passed here to ensure a synchronous close?
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
	c chan<- ProtocolMessage
	conn net.Conn
	buffer []byte
	pending []ProtocolMessage
	readHandshake bool
	logger *log.Logger
}

func (ic * IncomingPeerConnection) loop(c chan<- error) {
	defer onExit(c)
	var keepAlive = time.After(KEEP_ALIVE_PERIOD)
	for {
		c, next := ic.maybeEnableSend()
		select {
		case <- ic.close: break
		case <- keepAlive: // TODO: Close the connection!
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
	ic.logger.Printf("Read (%v) bytes\n", n)
	ic.buffer = append(ic.buffer, buf...)
	ic.maybeReadMessage()
	return n
}

func (ic * IncomingPeerConnection) maybeEnableSend() (chan<- ProtocolMessage, ProtocolMessage) {
	var c chan<- ProtocolMessage
	var next ProtocolMessage
	if len(ic.pending) > 0 {
		next = ic.pending[0]
		c = ic.c
	}
	return c, next
}

func (ic * IncomingPeerConnection) maybeReadMessage() {
	var msg ProtocolMessage
	if !ic.readHandshake {
		ic.buffer, msg = ReadHandshake(ic.buffer)
		if msg != nil {
			ic.readHandshake = true
		}
	} else {
		ic.buffer, msg = Unmarshal(ic.buffer)
	}

	if msg != nil {
		ic.logger.Print(msg)
	}

	// Remove keepalive
	if msg != nil && msg != KeepAliveMessage {
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
	pending []ProtocolMessage
	curr []byte
	logger *log.Logger
}

func (oc * OutgoingPeerConnection) loop(c chan<- error) {
	defer onExit(c)
	var keepAlive = time.After(KEEP_ALIVE_PERIOD)
	for {
		c := oc.maybeEnableReceive()
		select {
		case <- oc.close: break
		case <- keepAlive: oc.add(KeepAliveMessage)
		case msg := <- c: oc.add(msg)
		default:
			if n := oc.writeOrSleepFor(ONE_SECOND); n > 0 {
				keepAlive = time.After(KEEP_ALIVE_PERIOD)
			}
		}
	}
	oc.logger.Println("Loop exit")
}

func (oc * OutgoingPeerConnection) maybeEnableReceive() <-chan ProtocolMessage {
	var c <-chan ProtocolMessage
	if len(oc.pending) < MAX_OUTGOING_BUFFER {
		c = oc.c
	}
	return c
}

func (oc * OutgoingPeerConnection) add(msg ProtocolMessage) {
	oc.logger.Print(msg)
	oc.pending = append(oc.pending, msg)
}

func (oc * OutgoingPeerConnection) 	writeOrSleepFor(d time.Duration) (bytes int) {

	if !oc.hasDataToWrite() {
		oc.logger.Println("No data to write - sleeping...")
		time.Sleep(d)
		return 0
	}

	// Set deadline and write until we timeout or have nothing to write
	oc.conn.SetWriteDeadline(time.Now().Add(d))
	timeout := false
	for !timeout && oc.hasDataToWrite() {
		n := 0
		if len(oc.curr) > 0 {
			oc.curr, n, timeout = write(oc.conn, oc.curr)
		} else if len(oc.pending) > 0 {
			oc.curr = Marshal(oc.pending[0])
			oc.pending = oc.pending[1:]
			oc.curr, n, timeout = write(oc.conn, oc.curr)
		}
		bytes += n
	}
	oc.logger.Printf("Wrote (%v) bytes\n", bytes)
	return bytes
}

func (oc * OutgoingPeerConnection) hasDataToWrite() bool {
	return len(oc.curr) > 0 || len(oc.pending) > 0
}

func onExit(c chan<- error) {
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
		if opErr, ok := err.(*net.OpError); (ok && !opErr.Timeout() || !ok) {
			panic(err)
		}
		timeout = true
	}
	return buf[n:], n, timeout
}

// Actually does the write & checks for timeout
func read(r io.Reader) ([]byte, int) {
	buf := make([]byte, READ_BUFFER_SIZE)
	n, err := r.Read(buf)
	if nil != err {
		if opErr, ok := err.(*net.OpError); (ok && !opErr.Timeout() || !ok) {
			panic(err)
		}
	}
	return buf[:n], n
}
