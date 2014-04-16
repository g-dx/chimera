package bittorrent

import (
	"fmt"
	"log"
)

var (
	maxOutgoingQueue = 25
	maxRemoteRequestQueue = 25
	maxOutstandingLocalRequests = 25
)

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

type bitfield interface {
	IsComplete() bool
	IsValid(index uint32) bool
	Has(index uint32) bool
	Set(index uint32)
}

type Peer interface {
	CanWrite() bool
	ProcessMessages(diskReader chan<- RequestMessage, diskWriter chan<- PieceMessage) int
	CanRead() bool
	ReadNextMessage() bool
	WriteNextMessage() bool
}

type peer struct {

	// General protocol traffic
	outBuf	   []ProtocolMessage

	// Remote requests received & requests submitted to disk reader
	remotePendingReads []*RequestMessage
	remoteSubmittedReads []*RequestMessage

	// Local reads sent & received pieces
	localPendingReads []*RequestMessage
	localPendingWrites []*PieceMessage

	out 	    chan<- ProtocolMessage
	in 			<-chan ProtocolMessage
	pieceMap 	*PieceMap
	bitfield 	bitfield
	state PeerState
	id PeerIdentity
	logger * log.Logger
}

func NewPeer(id PeerIdentity, out chan<- ProtocolMessage, in <-chan ProtocolMessage, logger * log.Logger) Peer {
	return &peer {
		outBuf : make([]ProtocolMessage, 0, 25),
		remotePendingReads : make([]*RequestMessage, 0, maxRemoteRequestQueue),
		remoteSubmittedReads : make([]*RequestMessage, 0, maxRemoteRequestQueue),
		localPendingReads : make([]*RequestMessage, 0, maxRemoteRequestQueue),
		localPendingWrites : make([]*PieceMessage, 0, maxRemoteRequestQueue),
		out : out,
		in : in,
		pieceMap : nil,
		bitfield : nil,
		state : NewPeerState(),
		id : id,
		logger : logger,
	}
}

func (p * peer) CanWrite() bool {
	return len(p.outBuf) > 0
}

func (p * peer) CanRead() bool {
	// Can read if there is space in request queue
	// TODO: should we consider the length of the protocol queue?
	return len(p.remotePendingReads) < maxRemoteRequestQueue && len(p.outBuf) < maxOutgoingQueue
}

func (p * peer) ReadNextMessage() (readMessage bool) {

	select {
	case msg := <- p.in:
		readMessage = true
		p.handleMessage(msg)
	default:
		// Non-blocking read...
	}
	return readMessage
}

func (p * peer) WriteNextMessage() (wroteMessage bool) {

	select {
	case p.out <- p.outBuf[0]:
		p.outBuf = p.outBuf[1:]
		wroteMessage = true
	default: // Non-blocking read...
	}
	return wroteMessage
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

func (p * peer) handleMessage(pm ProtocolMessage) {
	switch msg := pm.(type) {
	case ChokeMessage: p.Choke()
	case UnchokeMessage: p.Unchoke()
	case InterestedMessage: p.Interested()
	case UninterestedMessage: p.Uninterested()
	case BitfieldMessage: p.Bitfield(msg.Bits())
	case HaveMessage: p.Have(msg.Index())
	case CancelMessage: p.Cancel(msg.Index(), msg.Begin(), msg.Length())
	case RequestMessage: p.Request(msg.Index(), msg.Begin(), msg.Length())
	case PieceMessage: p.Piece(msg.Index(), msg.Begin(), msg.Block())
	default:
		panic(fmt.Sprintf("Unknown protocol message: %v", pm))
	}
}

func (p * peer) Choke() {
	// Choke:
	// 1. clear local request queue
	// 2. set state
	p.localPendingReads = make([]*RequestMessage, 0, maxOutstandingLocalRequests)
	p.state.localChoke = true
}

func (p * peer) Unchoke() {
	// Unchoke:
	// 1. set state
	// 2. TODO: Should have requests ready to write
	p.state.localChoke = false
}

func (p * peer) Interested() {
	// Interested
	// 1. Set state, ensure they aren't a peer
	p.state.remoteInterest = !p.bitfield.IsComplete()
}

func (p * peer) Uninterested() {
	// Uninterested
	// 1. Set state, ensure they aren't a peer
	p.state.remoteInterest = false
}

func (p * peer) Have(index uint32) {
	// Have
	// 1. ensure valid index & not already in possession
	// 2. check if we need piece - if so send interest
	if !p.bitfield.IsValid(index) {
		die(fmt.Sprintf("Invalid index received: %v", index))
	}

	if !p.bitfield.Has(index) {
		if p.isNowInteresting(index) {
			p.send(Interested)
		}
		p.bitfield.Set(index)
		p.state.remoteInterest = !p.bitfield.IsComplete()
		p.pieceMap.Inc(index)
	}
}

func (p * peer) Cancel(index, begin, length uint32) {
	// Cancel:
	// 1. Remove all remote peer requests for this piece
	for i, req := range p.remotePendingReads {
		if req.Index() == index && req.Begin() == begin && req.Length() == length {
			p.remotePendingReads = append(p.remotePendingReads[:i], p.remotePendingReads[i+1:]...)
		}
	}

	for i, req := range p.remoteSubmittedReads {
		if req.Index() == index && req.Begin() == begin && req.Length() == length {
			p.remoteSubmittedReads = append(p.remoteSubmittedReads[:i], p.remoteSubmittedReads[i+1:]...)
		}
	}
}

func (p * peer) Request(index, begin, length uint32) {
	// Check index, begin & length is valid - if not close connection
	if !p.pieceMap.IsValid(index, begin, length) {
		die(fmt.Sprintf("Invalid request received: %v, %v, %v", index, begin, length))
	}

	if !p.state.remoteChoke {
		// Add request to pending reads queue
		p.remotePendingReads = append(p.remotePendingReads, Request(int(index), int(begin), int(length))) // TODO: We are we doing this? We should just pass the message in
	}
}

func (p * peer) Piece(index, begin uint32, block []byte) {
	// Check index, begin & length is valid and we requested this piece
	if !p.pieceMap.IsValid(index, begin, uint32(len(block))) {
		die(fmt.Sprintf("Invalid piece received: %v, %v, %v", index, begin, block))
	}

	// Remove from pending write queue
	// TODO: Move this into its own function; req, err := p.removeOutgoingRequest(index, begin)
	for i, req := range p.localPendingReads {
		if req.Index() == index && req.Begin() == begin {
			p.localPendingReads = append(p.localPendingReads[:i], p.localPendingReads[i+1:]...)
			break
		}
	}

	// To pending queue
	p.localPendingWrites = append(p.localPendingWrites, Piece(int(index), int(begin), block))
}

func (p * peer) Bitfield(b []byte) {
	// Check required high bits are set to zero
	// TODO: Validate bitfield

	// Add to global piece map
	p.pieceMap.IncBitfield(b)
}

func (p * peer) ProcessMessages(diskReader chan<- RequestMessage, diskWriter chan<- PieceMessage) int {

	// Process reads
	io := false
	if p.CanRead() {
		io = io || p.ReadNextMessage()
	}

	// Process read & write requests (if any)
	p.readBlock(diskReader)
	p.writeBlock(diskWriter)
	io = io || len(p.remotePendingReads) > 0
	io = io || len(p.localPendingWrites) > 0

	// Process writes
	if p.CanWrite() {
		io = io || p.WriteNextMessage()
	}

	// Dear god....
	if io {
		return 1
	} else {
		return 0
	}
}

func (p * peer) isNowInteresting(index uint32) bool {
	return !p.state.localInterest && p.pieceMap.Piece(index).BlocksNeeded()
}

func (p * peer) send(msg ProtocolMessage) {
	p.outBuf = append(p.outBuf, msg)
}

func die(msg string) {
	fmt.Println(msg)
}

func (p * peer) readBlock(diskReader chan<- RequestMessage) {
	if len(p.remotePendingReads) > 0 {
		select {
		case diskReader <- *p.remotePendingReads[0]:
			p.remoteSubmittedReads = append(p.remoteSubmittedReads, p.remotePendingReads[0])
			p.remotePendingReads = p.remotePendingReads[1:]
		default:
			// Non-blocking...
		}
	}
}

func (p * peer) writeBlock(diskWriter chan<- PieceMessage) {
	if len(p.localPendingWrites) > 0 {
		select {
		case diskWriter <- *p.localPendingWrites[0]:
			p.localPendingWrites = p.localPendingWrites[1:]
		default:
			// Non-blocking write...
		}
	}
}

func (p * peer) onBlockRead(index, begin uint32, block []byte) {

	// Attempt to remove from read requests
	removed := false
	for i, req := range p.remoteSubmittedReads {
		if req.Index() == index && req.Begin() == begin {
			p.remoteSubmittedReads = append(p.remoteSubmittedReads[:i], p.remoteSubmittedReads[i+1:]...)
			removed = true
		}
	}

	// if found add to outgoing queue
	if removed {
		p.send(Piece(int(index), int(begin), block))
	}
}

func (p * peer) addOutgoingRequest(req *RequestMessage) {
	p.localPendingReads = append(p.localPendingReads, req)
	p.outBuf = append(p.outBuf, req)
}
