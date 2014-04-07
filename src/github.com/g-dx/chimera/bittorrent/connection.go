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
)

var (
	MAX_OUTGOING_BUFFER = 10 // max pending writes

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

type PeerConnection struct {
	close chan<- struct{}
	in IncomingPeerConnection
	out OutgoingPeerConnection
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
			pending : make([]ProtocolMessage, 0, 25),
			readHandshake : false,
		},
		out : OutgoingPeerConnection {
			close : c,
			c : nil,
			conn : conn,
			buffer : make([]ProtocolMessage, 0, MAX_OUTGOING_BUFFER),
			next : make([]byte, 0),
		},
	}
	return pc, nil
}

func (pc * PeerConnection) Establish(in <-chan ProtocolMessage,
									 out chan<- ProtocolMessage,
									 e chan<- error,
                                     outHandshake *HandshakeMessage) (err error) {

	// Ensure we handshake properly
	err = pc.completeHandshake(outHandshake)
	if err != nil {
		return err
	}

	// Connect up channels and start go routines
	pc.in.c = out
	pc.out.c = in
	go pc.in.loop(e)
	go pc.out.loop(e)

	fmt.Printf("Started connection\n")
	return nil
}

func (pc *PeerConnection) completeHandshake(outHandshake *HandshakeMessage) error {

	// TODO: we should possibly be first reading if this is an incoming connection

	// Write handshake
	pc.out.add(outHandshake)
	pc.out.writeOrSleepFor(HANDSHAKE_TIMEOUT)
	if len(pc.out.next) != 0 {
		// Failed to write handshake in 5 seconds - close connection
		return nil
	}

	// Read handshake
	pc.in.readForMaximumOf(HANDSHAKE_TIMEOUT)
	if len(pc.in.pending) == 0 {
		return errHandshakeNotReceived
	}
	msg := pc.in.pending[0]
	pc.in.pending = pc.in.pending[1:]

	// Assert handshake
	inHandshake, ok := msg.(HandshakeMessage)
	if !ok {
		return errFirstMessageNotHandshake
	}

	// Assert hashes
	if !bytes.Equal(outHandshake.infoHash, inHandshake.infoHash) {
		return errHashesNotEquals
	}
	return nil
}

func (pc *PeerConnection) Close() error {

	// TODO: Should a channel be passed here to ensure a synchronous close?
	// Shutdown reader & writer
	pc.close <- struct{}{}
	pc.close <- struct{}{}

	// Finally, shutdown connection
	err := pc.in.conn.Close()
	if err != nil {
		// TODO: Log me
	}
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
}

func (ic * IncomingPeerConnection) loop(c chan<- error) {
	defer onExit(c)
	for {
		// Enable sending if we have messages to send
		c, next := ic.maybeEnableSend()

		select {
		case <-ic.close: break
		case c <- next: ic.pending = ic.pending[1:]
		default: ic.readForMaximumOf(ONE_SECOND)
		}
	}
}

func (ic * IncomingPeerConnection) readForMaximumOf(d time.Duration) {
	ic.conn.SetReadDeadline(time.Now().Add(d))
	ic.buffer = append(ic.buffer, read(ic.conn)...)

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
		ic.pending = append(ic.pending, msg)
	}
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

////////////////////////////////////////////////////////////////////////////////////////////////
// Outgoing Connection
////////////////////////////////////////////////////////////////////////////////////////////////

type OutgoingPeerConnection struct {
	close <-chan struct{}
	c <-chan ProtocolMessage
	conn net.Conn
	buffer []ProtocolMessage
	next []byte
}

func (oc * OutgoingPeerConnection) loop(c chan<- error) {
	defer onExit(c)
	for {
		// Reset keepalive and enable receive if buffer isn't full
		keepAlive := time.After(KEEP_ALIVE_PERIOD)
		c := oc.maybeEnableReceive()

		select {
		case <- oc.close: break
		case <- keepAlive: oc.add(KeepAliveMessage)
		case msg := <- c: oc.add(msg)
		default: oc.writeOrSleepFor(ONE_SECOND)
		}
	}
}

func (oc * OutgoingPeerConnection) maybeEnableReceive() <-chan ProtocolMessage {
	var c <-chan ProtocolMessage
	if len(oc.buffer) < MAX_OUTGOING_BUFFER {
		c = oc.c
	}
	return c
}

func (oc * OutgoingPeerConnection) add(msg ProtocolMessage) {
	oc.buffer = append(oc.buffer, msg)
}

func (oc * OutgoingPeerConnection) writeOrSleepFor(d time.Duration) {
	oc.conn.SetWriteDeadline(time.Now().Add(d))
	if len(oc.next) > 0 {
		oc.next = write(oc.conn, oc.next)
	} else if len(oc.buffer) > 0 {
//		oc.log("->", oc.buffer[0].String())
		oc.next = Marshal(oc.buffer[0])
		oc.buffer = oc.buffer[1:]
		oc.next = write(oc.conn, oc.next)
	} else {
		time.Sleep(d)
	}
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
func write(w io.Writer, buf []byte) []byte {
	n, err := w.Write(buf)
	if nil != err {
		if opErr, ok := err.(*net.OpError); (ok && !opErr.Timeout() || !ok) {
			panic(err)
		}
	}
	return buf[n:]
}

// Actually does the write & checks for timeout
func read(r io.Reader) []byte {
	buf := make([]byte, 4096)
	n, err := r.Read(buf)
	if nil != err {
		if opErr, ok := err.(*net.OpError); (ok && !opErr.Timeout() || !ok) {
			panic(err)
		}
	}
	return buf[:n]
}

func (pc * PeerConnection) logRead(msg ProtocolMessage) {
	pc.log("<-", msg.String())
}

func (pc * PeerConnection) logWrite(msg ProtocolMessage) {
	pc.log("->", msg.String())
}

func (pc * PeerConnection) log(prefix string, msg string) {
	fmt.Printf("%v [%v]%v %v\n", pc.in.conn.RemoteAddr(), time.Now(), prefix, msg)
}
