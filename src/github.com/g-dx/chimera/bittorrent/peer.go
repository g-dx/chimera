package bittorrent

import (
	"fmt"
	"log"
)

var (
	maxOutgoingQueue = 25
	maxRemoteRequestQueue = 10
	maxOutstandingLocalRequests = 10
)

type ProtocolHandler interface {
	Choke()
	Unchoke()
	Interested()
	Uninterested()
	Have(index uint32)
	Bitfield(bits []byte)
	Cancel(index, begin, length uint32)
	Request(index, begin, length uint32)
	Block(index, begin uint32, block []byte)
}

type Statistics struct {
	totalBytesDownloaded uint64
	bytesDownloadedPerUpdate uint
	bytesDownloaded uint
}

func (s * Statistics) Update() {

	// Update & reset
	s.bytesDownloadedPerUpdate = s.bytesDownloaded
	s.bytesDownloaded = 0
}

func (s * Statistics) Downloaded(n uint) {
	s.bytesDownloaded += n
}

type Peer struct {

	// Outgoing message buffer
	buffer []ProtocolMessage

	// Request queues
	remoteQ, localQ * PeerRequestQueue

	// Incoming, outgoing & connection error channels
	in  <-chan ProtocolMessage
	out chan<- ProtocolMessage
	err <-chan error

	pieceMap *PieceMap
	bitfield *BitSet
	state PeerState
	id PeerIdentity
	stats * Statistics

	logger * log.Logger

	onCloseFn func(error)
}

func NewPeer(id PeerIdentity,
		     out chan<- ProtocolMessage,
			 in <-chan ProtocolMessage,
			 mi * MetaInfo,
	         pieceMap *PieceMap,
			 e <-chan error,
			 logger * log.Logger,
			 onCloseFn func(error)) *Peer {
	return &Peer {
		buffer : make([]ProtocolMessage, 0, 25),
		remoteQ : NewPeerRequestQueue(maxRemoteRequestQueue),
		localQ : NewPeerRequestQueue(maxOutstandingLocalRequests),
		out : out,
		in : in,
		pieceMap : pieceMap,
		bitfield : NewBitSet(uint32(len(mi.Hashes))),
		state : NewPeerState(),
		id : id,
		logger : logger,
		err : e,
		stats : new(Statistics),
		onCloseFn : onCloseFn,
	}
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

func (p * Peer) handleMessage(pm ProtocolMessage) {
	p.logger.Printf("%v, Handling Msg: %v\n", p.id, pm)
	switch msg := pm.(type) {
	case *ChokeMessage: p.Choke()
	case *UnchokeMessage: p.Unchoke()
	case *InterestedMessage: p.Interested()
	case *UninterestedMessage: p.Uninterested()
	case *BitfieldMessage: p.Bitfield(msg.Bits())
	case *HaveMessage: p.Have(msg.Index())
	case *CancelMessage: p.Cancel(msg.Index(), msg.Begin(), msg.Length())
	case *RequestMessage: p.Request(msg.Index(), msg.Begin(), msg.Length())
	case *BlockMessage: p.Block(msg.Index(), msg.Begin(), msg.Block())
	default:
		panic(fmt.Sprintf("Unknown protocol message: %v", pm))
	}
}

func (p * Peer) Choke() {
	p.pieceMap.ReturnBlocks(p.localQ.Clear())
	p.state.localChoke = true
}

func (p * Peer) Unchoke() {
	p.state.localChoke = false
}

func (p * Peer) Interested() {
	p.state.remoteInterest = !p.bitfield.IsComplete()
}

func (p * Peer) Uninterested() {
	p.state.remoteInterest = false
}

func (p * Peer) Have(index uint32) {
	if !p.bitfield.IsValid(index) {
		p.Close(newError("Invalid index received: %v", index))
	}

	if !p.bitfield.Have(index) {

		p.bitfield.Set(index)
		p.state.remoteInterest = !p.bitfield.IsComplete()
		p.pieceMap.Inc(index)

		if p.isNowInteresting(index) {
			p.buffer = append(p.buffer, Interested)
		}
	}
}

func (p * Peer) Cancel(index, begin, length uint32) {
	p.remoteQ.Remove(index, begin, length)
}

func (p * Peer) Request(index, begin, length uint32) {
	if !p.pieceMap.IsValid(index, begin, length) {
		p.Close(newError("Invalid request received: %v, %v, %v", index, begin, length))
	}

	if !p.state.remoteChoke {
		p.remoteQ.AddRequest(Request(index, begin, length))
	}
}

func (p * Peer) Block(index, begin uint32, block []byte) {
	if !p.pieceMap.IsValid(index, begin, uint32(len(block))) {
		p.Close(newError("Invalid piece received: %v, %v, %v", index, begin, block))
	}

	p.localQ.AddBlock(Block(index, begin, block))
	p.Statistics().Downloaded(uint(len(block)))
}

func (p * Peer) Bitfield(bits []byte) {
	// Create new bitfield
	var err error
	p.bitfield, err = NewFromBytes(bits, p.bitfield.Size())
	if err != nil {
		p.Close(err)
	}

	// Add to global piece map
	p.pieceMap.IncAll(p.bitfield)

	// Check if we are interested
	for i := uint32(0); i < p.bitfield.Size(); i++ {
		if p.pieceMap.Piece(i).BlocksNeeded() {
			p.state.localInterest = true
			break
		}
	}
}

func (p * Peer) ProcessMessages(diskIn chan<- *RequestMessage, diskOut chan<- *BlockMessage) int {

	// Number of operations we performed
	ops := 0

	// Check for errors
	select {
	case err := <- p.err:
		p.Close(err)
		return 0
	default:
	}

	// Enable message read if we have space
	in := p.MaybeEnableMessageRead()
	select {
	case msg := <- in:
		p.handleMessage(msg)
		ops++
	default:
	}

	// Process read & write requests (if any)
	p.remoteQ.Pump(diskIn, p.out)
	p.localQ.Pump(p.out, diskOut)


	// Enable message write based on queue sizes
	out := p.MaybeEnableMessageWrite()
	select {
	case out <- p.buffer[0]:
		p.buffer = p.buffer[1:]
		ops++
	default:
	}

	return ops
}

func (p * Peer) MaybeEnableMessageWrite() chan<- ProtocolMessage {
	var c chan<- ProtocolMessage
	if len(p.buffer) > 0 {
		c = p.out
	}
	return c
}

func (p * Peer) MaybeEnableMessageRead() <-chan ProtocolMessage {
	var c <-chan ProtocolMessage
	if !p.remoteQ.IsFull() && len(p.buffer) < maxOutgoingQueue {
		c = p.in
	}
	return c
}

func (p Peer) Statistics() *Statistics {
	return p.stats
}

func (p * Peer) isNowInteresting(index uint32) bool {
	return !p.state.localInterest && p.pieceMap.Piece(index).BlocksNeeded()
}

func (p * Peer) Close(err error) {

	// Update piece map
	p.pieceMap.DecAll(p.bitfield)

	// Return outstanding blocks
	p.pieceMap.ReturnBlocks(p.localQ.Clear())

	// Invoke on close
	p.onCloseFn(err)
	fmt.Printf("Peer (%v) closed.\n", p.id)
}

func (p * Peer) BlocksRequired() uint {
	return uint(p.localQ.Capacity() - p.localQ.Size())
}

func (p * Peer) CanDownload() bool {
	return !p.state.localChoke && p.state.localInterest && p.BlocksRequired() > 0
}

func (p * Peer) String() string {
	return fmt.Sprintf("")
}
