package bittorrent

import (
	"fmt"
	"log"
	"time"
)

const (
	ReceiveBufferSize = 25
	RequestQueueSize = 25
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
	remoteQ, localQ * MessageQueue
	remoteSink, localSink * MessageSink

	// Connection error channels
	err <-chan error

	// State of peer
	state PeerState

	// Overall torrent piece map
	pieceMap *PieceMap

	// ID
	id PeerIdentity

	// Peer statistics concerning upload, download, etc...
	statistics * Statistics

	logger * log.Logger

	// Cleanup function called on close
	onCloseFn func(error)
}

func NewPeer(id PeerIdentity,
		     out chan<- ProtocolMessage,
			 disk chan<- DiskMessage,
			 mi * MetaInfo,
	         pieceMap * PieceMap,
			 e <-chan error,
			 logger * log.Logger,
			 onCloseFn func(error)) *Peer {

	return &Peer {
		inBuf      : NewRingBuffer(ReceiveBufferSize),

		remoteQ    : NewMessageQueue(RequestQueueSize),
		remoteSink : NewRemoteMessageSink(id, out, disk),
		localQ     : NewMessageQueue(RequestQueueSize),
		localSink  : NewLocalMessageSink(id, out, disk),

		id         : id,
		pieceMap   : pieceMap,
		state      : NewPeerState(NewBitSet(uint32(len(mi.Hashes)))),

		logger : logger,
		err : e,

		statistics : &Statistics{},
		onCloseFn : onCloseFn,
	}
}

func (p * Peer) Id() PeerIdentity {
	return p.id
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

	// Drain local & remote traffic to appropriate destinations
	remoteW := true
	localW := true
	for remoteW || localW {
		if localW {
			localW = p.localQ.Write(p.localSink)
			n++
		}
		if remoteW {
			remoteW = p.remoteQ.Write(p.remoteSink)
			n++
		}
	}

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

	// Validate
	if !p.state.bitfield.IsValid(index) {
		p.Close(newError("Invalid index received: %v", index))
	}

	if !p.state.bitfield.Have(index) {

		// Update bitfield, check for remote seed & update availability
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

	// Create & validate bitfield
	var err error
	p.state.bitfield, err = NewBitSetFrom(bits, p.state.bitfield.Size())
	if err != nil {
		p.Close(err)
	}

	// Update availability in global piece map
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

	// Update availability & return all blocks
	p.pieceMap.DecAll(p.state.bitfield)
	p.pieceMap.ReturnBlocks(p.localQ.ClearAll())

	// ...

	// Invoke on close
	p.onCloseFn(err)
	fmt.Printf("Peer (%v) closed.\n", p.id)
}

func (p * Peer) BlocksRequired() uint {
	return uint(p.localQ.Capacity() - p.localQ.Size())
}

func (p * Peer) BlockWritten(index, begin uint32) {
	// Update piece and check if finished
	piece := p.pieceMap.Piece(index)
	piece.BlockDone(begin)
	if piece.IsComplete() {

		// TODO: Must validate hash is correct.

		p.localQ.Add(Have(index))
	}
}

func (p * Peer) CanDownload() bool {
	return !p.state.localChoke && p.state.localInterest && !p.localQ.IsFull()
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
