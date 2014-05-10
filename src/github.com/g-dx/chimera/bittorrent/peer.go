package bittorrent

import (
	"fmt"
	"log"
	"time"
)

const (
	maxOutgoingQueue = 25
	maxRemoteRequestQueue = 10
	maxOutstandingLocalRequests = 10

	ThirtySeconds = 30 * time.Second
)


// ----------------------------------------------------------------------------------
// Peer State - key protocol state
// ----------------------------------------------------------------------------------

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

// ----------------------------------------------------------------------------------
// Peer
// ----------------------------------------------------------------------------------

type Peer struct {

	// Message buffer & message sinks
	inBuf * RingBuffer

	// Request queues & sinks
	remoteQ, localQ * PeerRequestQueue
	remoteSink, localSink * MessageSink

	// Incoming & connection error channels
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
			 in <-chan ProtocolMessage,
		     out chan<- ProtocolMessage,
			 disk chan<- DiskMessage,
			 mi * MetaInfo,
	         pieceMap * PieceMap,
			 e <-chan error,
			 logger * log.Logger,
			 onCloseFn func(error)) *Peer {

	return &Peer {
		inBuf      : NewRingBuffer(maxOutgoingQueue),

		remoteQ    : NewPeerRequestQueue(maxRemoteRequestQueue),
		remoteSink : NewRemoteMessageSink(id, out, disk),
		localQ     : NewPeerRequestQueue(maxOutstandingLocalRequests),
		localSink  : NewLocalMessageSink(id, out, disk),

		in      : in,

		id         : id,
		pieceMap   : pieceMap,
		state      : NewPeerState(NewBitSet(uint32(len(mi.Hashes)))),

		logger : logger,
		err : e,

		statistics : &Statistics{},
		onCloseFn : onCloseFn,
	}
}

func (p * Peer) ProcessMessages() int {

	// Number of operations we performed
	n := 0

	// Check for errors
	select {
	case err := <- p.err:
		p.Close(err)
		return 0
	default:
	}

	// Return expired requests to map
	p.pieceMap.ReturnBlocks(p.localQ.ClearExpired(ThirtySeconds))

	// Attempt to fill message buffer & then process them
	p.inBuf.Fill(p.in)
	for !p.inBuf.IsEmpty() {
		p.HandleMessage(p.inBuf.Next())
		n++
	}

	// Drain local & remote traffic to appropriate destinations
	n += p.localQ.Drain(p.localSink)
	n += p.remoteQ.Drain(p.remoteSink)
	return n
}

func (p * Peer) HandleMessage(pm ProtocolMessage) {
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
	p.pieceMap.ReturnBlocks(p.localQ.ClearNew())
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
			p.localQ.Add(Interested)
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
		p.remoteQ.Add(Request(index, begin, length))
	}
}

func (p * Peer) Block(index, begin uint32, block []byte) {
	if !p.pieceMap.IsValid(index, begin, uint32(len(block))) {
		p.Close(newError("Invalid block received: %v, %v, %v", index, begin, block))
	}

	p.localQ.Add(Block(index, begin, block))
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
			p.localQ.Add(Interested)
			break
		}
	}
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
	p.pieceMap.ReturnBlocks(p.localQ.ClearAll())

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

// ----------------------------------------------------------------------------------
// PeerStatistics
// ----------------------------------------------------------------------------------

type Statistics struct {
	totalBytesDownloaded uint64
	bytesDownloadedPerUpdate uint
	bytesDownloaded uint

	totalBytesWritten uint64
	bytesWrittenPerUpdate uint
	bytesWritten uint
}

func (s * Statistics) Update() {

	// Update & reset
	s.bytesDownloadedPerUpdate = s.bytesDownloaded
	s.bytesDownloaded = 0
	s.bytesWrittenPerUpdate = s.bytesWritten
	s.bytesWritten = 0
}

func (s * Statistics) Downloaded(n uint) {
	s.bytesDownloaded += n
}

func (s * Statistics) Written(n uint) {
	s.bytesWritten += n
}
