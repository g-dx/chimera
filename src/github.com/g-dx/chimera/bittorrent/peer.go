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
	remoteChoke, localChoke, remoteInterest, localInterest, optimistic, new bool
	bitfield                                                                *BitSet
}

func NewPeerState(bits *BitSet) PeerState {
	return PeerState{
		remoteChoke:    true,
		localChoke:     true,
		remoteInterest: false,
		localInterest:  false,
		optimistic:     false,
		new:            true,
		bitfield:       bits,
	}
}

// ----------------------------------------------------------------------------------
// Peer
// ----------------------------------------------------------------------------------

type Peer struct {
	queue *PeerQueue

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
	queue *PeerQueue,
	mi *MetaInfo,
	pieceMap *PieceMap,
	logger *log.Logger) *Peer {

	return &Peer{
		id:         id,
		pieceMap:   pieceMap,
		state:      NewPeerState(NewBitSet(uint32(len(mi.Hashes)))),
		logger:     logger,
		statistics: &Statistics{},
		queue:      queue,
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
	p.pieceMap.ReturnBlocks(p.queue.Choke())
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
			p.queue.Add(Interested(p.id))
		}
	}
	return nil
}

func (p *Peer) onCancel(index, begin, length uint32) error {
	// NOTE: Handled lower down the stack...
	// p.disk.Cancel(index, begin, length, p.id)
	return nil
}

func (p *Peer) onRequest(index, begin, length uint32) error {
	// NOTE: Already on the way to disk...
	// p.disk.Read(index, begin, length, p.id, p.queue.out)
	return nil
}

func (p *Peer) onBlock(index, begin uint32, block []byte) error {
	// NOTE: Already on the way to disk...
	// p.disk.Write(index, begin, length, p.id)
	p.Stats().Downloaded(uint(len(block)))
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
		if p.pieceMap.Piece(i).RequestsRequired() {
			p.state.localInterest = true
			p.queue.Add(Interested(p.id))
			break
		}
	}
	return nil
}

func (p Peer) Stats() *Statistics {
	return p.statistics
}

func (p *Peer) isNowInteresting(index uint32) bool {
	return !p.state.localInterest && p.pieceMap.Piece(index).RequestsRequired()
}

func (p *Peer) Close() {

	// Update availability & return all blocks
	p.pieceMap.DecAll(p.state.bitfield)
	p.pieceMap.ReturnBlocks(p.queue.Close())

	// ...

	fmt.Printf("Peer (%v) closed.\n", p.id)
}

func (p *Peer) BlocksRequired() int {
	// TODO: Fix me!
	return p.queue.NumRequests()
}

func (p *Peer) CanDownload() bool {
	return !p.state.localChoke && p.state.localInterest
}

func (p *Peer) Choke(snubbed bool) error {
	// TODO: Mark so that the choker knows
	return p.Add(Choke(p.id))
}

func (p *Peer) UnChoke(optimistic bool) error {
	p.state.optimistic = optimistic
	p.state.new = false
	return p.Add(Unchoke(p.id))
}

func (p *Peer) IsInterested() bool {
	return p.state.remoteInterest
}

func (p *Peer) IsChoked() bool {
	return p.state.remoteChoke
}

func (p *Peer) IsOptimistic() bool {
	return p.state.optimistic
}

func (p *Peer) IsNew() bool {
	return p.state.new
}

func (p *Peer) Cancel(index, begin, len uint32) error {
	return p.Add(Cancel(p.id, index, begin, len))
}

func (p *Peer) Add(pm ProtocolMessage) error {
	return p.queue.Add(pm)
}

// ----------------------------------------------------------------------------------
// PeerStatistics
// ----------------------------------------------------------------------------------

type Statistics struct {
	totalBytesDownloaded     uint64
	bytesDownloadedPerUpdate uint
	bytesDownloaded          uint

	totalBytesUploaded     uint64
	bytesUploadedPerUpdate uint
	bytesUploaded          uint

	totalBytesWritten     uint64
	bytesWrittenPerUpdate uint
	bytesWritten          uint
}

func (s *Statistics) Update() {

	// Update & reset
	s.bytesDownloadedPerUpdate = s.bytesDownloaded
	s.bytesDownloaded = 0
	s.bytesUploadedPerUpdate = s.bytesUploaded
	s.bytesUploaded = 0
	s.bytesWrittenPerUpdate = s.bytesWritten
	s.bytesWritten = 0
}

func (s *Statistics) Uploaded(n uint) {
	s.bytesUploaded += n
}

func (s *Statistics) Downloaded(n uint) {
	s.bytesDownloaded += n
}

func (s *Statistics) Written(n uint) {
	s.bytesWritten += n
}

func (s *Statistics) Upload() uint {
	return s.bytesUploadedPerUpdate
}

func (s *Statistics) Download() uint {
	return s.bytesDownloadedPerUpdate
}