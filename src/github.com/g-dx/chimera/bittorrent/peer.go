package bittorrent

import (
	"fmt"
	"log"
)

const (
	maxOutgoingQueue = 25
	maxRemoteRequestQueue = 10
	maxOutstandingLocalRequests = 10

	// Max number of messages to write during "Process"
	maxMessageWrites = 5

	// Max number of disk reads during "Process"
	maxDiskReads = 5
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
	out, disk * OutgoingBuffer


	// Request queues
	remoteQ, localQ * PeerRequestQueue

	// Incoming, outgoing & connection error channels
	in  <-chan ProtocolMessage
	err <-chan error

	// State of peer
	state PeerState

	// Overall torrent piece map
	pieceMap *PieceMap

	//
	id PeerIdentity

	// Peer statistics concerning upload, download, etc...
	statistics * Statistics

	logger * log.Logger

	// Cleanup function called on close
	onCloseFn func(error)
}

func NewPeer(id PeerIdentity,
		     out, disk chan<- ProtocolMessage,
			 in <-chan ProtocolMessage,
			 mi * MetaInfo,
	         pieceMap *PieceMap,
			 e <-chan error,
			 logger * log.Logger,
			 onCloseFn func(error)) *Peer {
	return &Peer {
		out : NewOutgoingBuffer(out, maxOutstandingLocalRequests),
		disk : NewOutgoingBuffer(disk, maxOutstandingLocalRequests),
		remoteQ : NewPeerRequestQueue(maxRemoteRequestQueue),
		localQ : NewPeerRequestQueue(maxOutstandingLocalRequests),
		in : in,
		pieceMap : pieceMap,
		state : NewPeerState(NewBitSet(uint32(len(mi.Hashes)))),
		id : id,
		logger : logger,
		err : e,
		statistics : new(Statistics),
		onCloseFn : onCloseFn,
	}
}

type PeerState struct {
	remoteChoke, localChoke, remoteInterest, localInterest bool
	bitfield *BitSet
}

func NewPeerState(bits *BitSet) PeerState {
	return PeerState {
		remoteChoke : true,
		localChoke : true,
		remoteInterest : false,
		localInterest : false,
		bitfield : bits,
	}
}

func (p * Peer) handleMessage(pm ProtocolMessage) {
	p.logger.Printf("%v, Handling Msg: %v\n", p.id, pm)
	switch msg := pm.(type) {
	case *ChokeMessage: p.Choke()
	case *UnchokeMessage: p.Unchoke()
	case *InterestedMessage: p.Interested()
	case *UninterestedMessage: p.Uninterested()
	case *BitfieldMessage: p.state.bitfield(msg.Bits())
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
	p.state.remoteInterest = !p.state.bitfield.IsComplete()
}

func (p * Peer) Uninterested() {
	p.state.remoteInterest = false
}

func (p * Peer) Have(index uint32) {
	if !p.state.bitfield.IsValid(index) {
		p.Close(newError("Invalid index received: %v", index))
	}

	if !p.state.bitfield.Have(index) {

		p.state.bitfield.Set(index)
		p.state.remoteInterest = !p.state.bitfield.IsComplete()
		p.pieceMap.Inc(index)

		if p.isNowInteresting(index) {
			p.out.Add(Interested)
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
		p.Close(newError("Invalid block received: %v, %v, %v", index, begin, block))
	}

	p.localQ.AddBlock(Block(index, begin, block))
	p.Statistics().Downloaded(uint(len(block)))
}

func (p * Peer) Bitfield(bits []byte) {
	// Create new bitfield
	var err error
	p.state.bitfield, err = NewFromBytes(bits, p.state.bitfield.Size())
	if err != nil {
		p.Close(err)
	}

	// Add to global piece map
	p.pieceMap.IncAll(p.state.bitfield)

	// Check if we are interested
	for i := uint32(0); i < p.state.bitfield.Size(); i++ {
		if p.pieceMap.Piece(i).BlocksNeeded() {
			p.state.localInterest = true
			break
		}
	}
}

	func (p * Peer) ProcessMessages() int {

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
	in := MaybeEnable(p.in, func() bool { return !p.remoteQ.IsFull() && p.out.Size() < maxOutgoingQueue })
	select {
	case msg := <- in:
		p.handleMessage(msg)
		ops++
	default:
	}

	// Drain requests & blocks to appropriate destinations
	p.remoteQ.Drain(p.disk, p.out)
	p.localQ.Drain(p.out, p.disk)

	// Pump disk & out queue
	ops += p.out.Pump(maxMessageWrites)
	ops += p.disk.Pump(maxDiskReads)

	return ops
}
func (p Peer) Statistics() *Statistics {
	return p.statistics
}

func (p * Peer) isNowInteresting(index uint32) bool {
	return !p.state.localInterest && p.pieceMap.Piece(index).BlocksNeeded()
}

func (p * Peer) Close(err error) {

	// Update piece map
	p.pieceMap.DecAll(p.state.bitfield)

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
