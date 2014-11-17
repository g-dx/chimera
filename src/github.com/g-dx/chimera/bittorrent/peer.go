package bittorrent

import (
	"fmt"
	"log"
	"time"
)

const (
	ReceiveBufferSize = 25
	RequestQueueSize  = 25
	ThirtySeconds     = 30 * time.Second
)

// ----------------------------------------------------------------------------------
// Peer State - key protocol state
// ----------------------------------------------------------------------------------

type PeerState struct {
	remoteChoke, localChoke, remoteInterest, localInterest bool
	bitfield *BitSet
}

func NewPeerState(bits *BitSet) PeerState {
	return PeerState{
		remoteChoke:    true,
		localChoke:     true,
		remoteInterest: false,
		localInterest:  false,
		bitfield:       bits,
	}
}

// ----------------------------------------------------------------------------------
// Peer
// ----------------------------------------------------------------------------------

type Peer struct {

	// Request queues & sinks
	remoteQ, localQ       *MessageQueue
	remoteSink, localSink *MessageSink

	// State of peer
	state PeerState

	// Overall torrent piece map
	pieceMap *PieceMap

	// ID
	id *PeerIdentity

	// Peer statistics concerning upload, download, etc...
	statistics *Statistics

	logger *log.Logger
}

func NewPeer(id *PeerIdentity,
	out chan<- ProtocolMessage,
	mi *MetaInfo,
	pieceMap *PieceMap,
	logger *log.Logger) *Peer {

	return &Peer{

		remoteQ:    NewMessageQueue(RequestQueueSize),
		remoteSink: NewRemoteMessageSink(id, out, nil),
		localQ:     NewMessageQueue(RequestQueueSize),
		localSink:  NewLocalMessageSink(id, out, nil),
		id:       id,
		pieceMap: pieceMap,
		state:    NewPeerState(NewBitSet(uint32(len(mi.Hashes)))),
		logger: logger,
		statistics: &Statistics{},
	}
}

func (p *Peer) Id() *PeerIdentity {
	return p.id
}

func (p *Peer) OnMessage(pm ProtocolMessage) error {
	p.logger.Printf("%v, Handling Msg: %v\n", p.id, pm)
	switch msg := pm.(type) {
	case *ChokeMessage:
		return p.onChoke()
	case *UnchokeMessage:
		return p.onUnchoke()
	case *InterestedMessage:
		return p.onInterested()
	case *UninterestedMessage:
		return p.onUninterested()
	case *BitfieldMessage:
		return p.onBitfield(msg.Bits())
	case *HaveMessage:
		return p.onHave(msg.Index())
	case *CancelMessage:
		return p.onCancel(msg.Index(), msg.Begin(), msg.Length())
	case *RequestMessage:
		return p.onRequest(msg.Index(), msg.Begin(), msg.Length())
	case *BlockMessage:
		return p.onBlock(msg.Index(), msg.Begin(), msg.Block())
	default:
		return newError(fmt.Sprintf("Unknown protocol message: %v", pm))
	}
}

func (p *Peer) onChoke() error {
	p.pieceMap.ReturnBlocks(p.localQ.ClearNew())
	p.state.localChoke = true
	return nil
}

func (p *Peer) onUnchoke() error {
	p.state.localChoke = false
	return nil
}

func (p *Peer) onInterested() error {
	p.state.remoteInterest = !p.state.bitfield.IsComplete()
	return nil
}

func (p *Peer) onUninterested() error {
	p.state.remoteInterest = false
	return nil
}

func (p *Peer) onHave(index uint32) error {

	// Validate
	if !p.state.bitfield.IsValid(index) {
		return newError("Invalid index received: %v", index)
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
	return nil
}

func (p *Peer) onCancel(index, begin, length uint32) error {
	p.remoteQ.Remove(index, begin, length)
	return nil
}

func (p *Peer) onRequest(index, begin, length uint32) error {
	if !p.pieceMap.IsValid(index, begin, length) {
		return newError("Invalid request received: %v, %v, %v", index, begin, length)
	}

	if !p.state.remoteChoke {
		p.remoteQ.Add(Request(index, begin, length))
	}
	return nil
}

func (p *Peer) onBlock(index, begin uint32, block []byte) error {
	if !p.pieceMap.IsValid(index, begin, uint32(len(block))) {
		return newError("Invalid block received: %v, %v, %v", index, begin, block)
	}

	p.localQ.Add(Block(index, begin, block))
	p.Statistics().Downloaded(uint(len(block)))
	return nil
}

func (p *Peer) onBitfield(bits []byte) error {

	// Create & validate bitfield
	var err error
	p.state.bitfield, err = NewBitSetFrom(bits, p.state.bitfield.Size())
	if err != nil {
		return err
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
	return nil
}

func (p Peer) Statistics() *Statistics {
	return p.statistics
}

func (p *Peer) isNowInteresting(index uint32) bool {
	return !p.state.localInterest && p.pieceMap.Piece(index).BlocksNeeded()
}

func (p *Peer) Close() {

	// Update availability & return all blocks
	p.pieceMap.DecAll(p.state.bitfield)
	p.pieceMap.ReturnBlocks(p.localQ.ClearAll())

	// ...

	fmt.Printf("Peer (%v) closed.\n", p.id)
}

func (p *Peer) BlocksRequired() uint {
	return uint(p.localQ.Capacity() - p.localQ.Size())
}

func (p *Peer) BlockWritten(index, begin uint32) {
	// Update piece and check if finished
	piece := p.pieceMap.Piece(index)
	piece.BlockDone(begin)
	if piece.IsComplete() {

		// TODO: Must validate hash is correct.

		p.localQ.Add(Have(index))
	}
}

func (p *Peer) CanDownload() bool {
	return !p.state.localChoke && p.state.localInterest && !p.localQ.IsFull()
}

// ----------------------------------------------------------------------------------
// PeerStatistics
// ----------------------------------------------------------------------------------

type Statistics struct {
	totalBytesDownloaded     uint64
	bytesDownloadedPerUpdate uint
	bytesDownloaded          uint

	totalBytesWritten     uint64
	bytesWrittenPerUpdate uint
	bytesWritten          uint
}

func (s *Statistics) Update() {

	// Update & reset
	s.bytesDownloadedPerUpdate = s.bytesDownloaded
	s.bytesDownloaded = 0
	s.bytesWrittenPerUpdate = s.bytesWritten
	s.bytesWritten = 0
}

func (s *Statistics) Downloaded(n uint) {
	s.bytesDownloaded += n
}

func (s *Statistics) Written(n uint) {
	s.bytesWritten += n
}
