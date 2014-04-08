package bittorrent

import "time"

type ProtocolHandler interface {
	Choke()
	Unchoke()
	Interested()
	Uninterested()
	Have(index uint32)
	Bitfield(b []byte)
	Cancel(index, begin, length uint32)
	Request(index, begin, length uint32)
	Piece(index, begin uint32, block []byte)
}


type Peer struct {
	pc PeerConnection
	c chan<- ProtocolMessage
	s PeerState
}

type PeerState struct {
	remoteChoke, localChoke, remoteInterest, localInterest bool
}

func NewPeerState() PeerState {
	return PeerState {
		remoteChoke : true,
		localChoke : true,
		remoteInterest : false,
		localInterest : false,
	}
}

func (p * Peer) loop() {

	for {
		select {
			case msg := <- p.c: p.handleMessage(msg)
		}
	}
}

func (p * Peer) handleMessage(pm ProtocolMessage) {
	switch msg := pm.(type) {
	case ChokeMessage: p.Choke()
	case UnchokeMessage: p.Unchoke()
	case InterestedMessage: p.Interested()
	case UninterestedMessage:

	}
}

func (p * Peer) Choke() {
	// Cancel all pending "request" messages on connection
	p.s.localChoke = true
}

func (p * Peer) Unchoke() {
	p.s.localChoke = false
}

func (p * Peer) Interested() {
	p.s.remoteInterest = true // TODO: This should be => !p.bitfield.IsComplete()
}

func (p * Peer) Uninterested() {
	p.s.remoteInterest = false
}

func (p * Peer) Have(index uint32) {

	// Check index is valid - if not close connection

	// if !p.bitfield.Has(index) { ...

	// Check if we are now interested
	if !p.s.localInterest {
		// TODO: Ask someone if we have this piece - if so, send "interested"
	}

	// Set bit & check for peer becoming a seed
	// p.bitfield.Set(index)
	// p.s.remoteInterest = !p.bitfield.IsComplete()
}

func (p * Peer) Cancel(index, begin, length uint32) {
	// Remove from pending reads queue?
}

func (p * Peer) Request(index, begin, length uint32) {
	// Check index, begin & length is valid - if not close connection

	if !p.s.remoteChoke {
		// Add request to pending reads queue
	}
}

func (p * Peer) Piece(index, begin uint32, block []byte) {
	// Check index, begin & length is valid and we requested this piece

	// Remove from pending write queue & ask someone to write it
}

func (p * Peer) Bitfield(b []byte) {
	// Check required high bits are set to zero

	// Add to global piece map

	// p.bitfield = b
}

func loop(c chan<- bool) {


	for {

		select {
		case <- time.After(1 * time.Second):
			// Run downloader...
		case <- time.After(10 * time.Second):
			// Run choking algorithm
		case <- rmPeer:
		case <- addPeer:
		case <- newPeer:
		case <- outgoingMessage:
		default:
			// Route message to peer
		}
	}

}
