package bittorrent

import (
	"testing"
	"fmt"
	"os"
)

func TestConnectionEstablish(t *testing.T) {

	infoHash := make([]byte, 20)
	c := make(chan *PeerConnection)

	// Choose random port 1024 - 65535
	port := 60002
	laddr := fmt.Sprintf("localhost:%v", port)
	listener, err := NewConnectionListener(c, laddr)
	if err != nil {
		t.Fatalf("Failed to open listener: %v", err)
	}
	defer listener.Close()

	// Setup a goroutine for other side
	go func(){
		pc := <- c
		defer pc.Close()

		in  := make(chan ProtocolMessage)
		out := make(chan *MessageList)
		e   := make(chan PeerError)
		pc.Establish(in, out, e, Handshake(infoHash), os.Stdout, false)
	}()


	var pc *PeerConnection
	pc, err = NewConnection(laddr)
	if err != nil {
		t.Fatalf("Failed open connection: %v", err)
	}
	defer pc.Close()

	// Need to pass logger
	in  := make(chan ProtocolMessage)
	out := make(chan *MessageList)
	e   := make(chan PeerError)
	_, err = pc.Establish(in, out, e, Handshake(infoHash), os.Stdout, true)
	if err != nil {
		t.Errorf("Failed establish connection: %v", err)
	}
}

//func TestConnectionCompleteHandshake(t *testing.T) {
//
//
//}
//
//func TestConnectionCompleteHandshake(t *testing.T) {
//
//}
//
//func BenchmarkRead(t *testing.T) {
//
//
//
//}
