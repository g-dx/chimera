package bittorrent

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"testing"
)

func TestConnectionEstablish(t *testing.T) {

	// Create connected peers
	pc1, pc2 := createTestPeerConnections(t)
	defer pc1.Close()
	defer pc2.Close()

	// Test send and receive
	msg1 := Have(5)
	pc1.Send(msg1)
	list := pc2.Receive()
	intEquals(t, 1, len(list.msgs))
	msgEquals(t, msg1, list.msgs[0])

	msg2 := Unchoke()
	pc2.Send(msg2)
	list = pc1.Receive()
	intEquals(t, 1, len(list.msgs))
	msgEquals(t, msg2, list.msgs[0])
}

func createTestPeerConnections(t testing.TB) (*PeerConnection, *PeerConnection) {
	infoHash := make([]byte, 20)
	c := make(chan *PeerConnection)

	// Choose random port 1024 - 65535
	laddr := fmt.Sprintf("localhost:%v", 1024+rand.Intn(65535-1024))
	listener, err := NewConnectionListener(c, laddr)
	if err != nil {
		t.Fatalf("Failed to open listener: %v", err)
	}
	defer listener.Close()

	// Setup a goroutine for other side
	res := make(chan *PeerConnection)
	go func() {
		pc := <-c
		establish(pc, infoHash, t)
		res <- pc
	}()

	// Open & establish "sending" side
	pc := open(laddr, t)
	establish(pc, infoHash, t)

	// Return all parts
	return <-res, pc
}

func open(laddr string, t testing.TB) *PeerConnection {
	pc, err := NewConnection(laddr)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	return pc
}

func establish(pc *PeerConnection, infoHash []byte, t testing.TB) {
	_, err := pc.Establish(make(chan ProtocolMessage), make(chan *MessageList), make(chan PeerError),
		Handshake(infoHash), ioutil.Discard, true)
	if err != nil {
		t.Fatalf("Failed to establish connection: %v", err)
	}
}
