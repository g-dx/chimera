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
	remoteChoke,
	localChoke,
	remoteInterest,
	localInterest,
	optimistic,
	new bool
	bitfield *BitSet
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
	queue      *PeerQueue
	state      PeerState
	pieceMap   *PieceMap
	id         *PeerIdentity
	statistics *Statistics
	logger     *log.Logger
}

func NewPeer(id *PeerIdentity,
	queue *PeerQueue,
	pieceMap *PieceMap,
	logger *log.Logger) *Peer {

	return &Peer{
		id:         id,
		pieceMap:   pieceMap,
		state:      NewPeerState(NewBitSet(uint32(len(pieceMap.pieces)))),
		logger:     logger,
		statistics: NewStatistics(),
		queue:      queue,
	}
}

func (p *Peer) Id() *PeerIdentity {
	return p.id
}

func (p *Peer) OnMessage(pm ProtocolMessage) error {
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
			p.state.localInterest = true
			p.queue.Add(Interested())
		}
	}
	return nil
}

func (p *Peer) onCancel(index, begin, length uint32) error {
	// TODO: Implement cancel support
	return nil
}

func (p *Peer) onRequest(index, begin, length uint32) error {

	// TODO: Check request valid
	//	p.pieceMap.IsValid()

	// Get block message and pass to disk to fill
//	p.disk.Read(index, begin, p.id)
	return nil
}

func (p *Peer) onBlock(index, begin uint32, block []byte) error {
	// NOTE: Already on the way to disk...
	// p.disk.Write(index, begin, length, p.id)
//	p.disk.Write(index, begin, block)
	p.Stats().Download.Add(len(block))
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
			p.queue.Add(Interested())
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

	p.logger.Printf("Peer (%v) closed.\n", p.id)
}

func (p *Peer) QueuedRequests() int {
	return p.queue.QueuedRequests()
}

func (p *Peer) CanDownload() bool {
	return !p.state.localChoke && p.state.localInterest
}

func (p *Peer) Choke() error {
	p.state.remoteChoke = true
	return p.Add(Choke())
}

func (p *Peer) UnChoke(optimistic bool) error {
	p.state.optimistic = optimistic
	p.state.new = false
	p.state.remoteChoke = false
	return p.Add(Unchoke())
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

func (p *Peer) ClearOptimistic() {
	p.state.optimistic = false
}

func (p *Peer) IsNew() bool {
	return p.state.new
}

func (p *Peer) Cancel(index, begin, len uint32) error {
	return p.Add(Cancel(index, begin, len))
}

func (p *Peer) Add(pm ProtocolMessage) error {
	p.queue.Add(pm)
	return nil // TODO: Fix this!
}

// ----------------------------------------------------------------------------------
// Disk Callbacks
// ----------------------------------------------------------------------------------

func (p *Peer) onBlockRead(index, begin int, block []byte) {
	p.Stats().Upload.Add(len(block))
	p.Add(Block(uint32(index), uint32(begin), block))
}

func (p *Peer) onBlockWritten(index, begin, len int) {
	p.Stats().Written.Add(len)
}

// ----------------------------------------------------------------------------------
// PeerStatistics
// ----------------------------------------------------------------------------------

type Statistics struct {
	Download *Counter
	Upload   *Counter
	Written  *Counter
	all      []*Counter
}

func NewStatistics() *Statistics {
	d := &Counter{}
	u := &Counter{}
	w := &Counter{}
	return &Statistics{d, u, w, append(make([]*Counter, 0, 3), d, u, w)}
}

func (s *Statistics) Update() {
	for _, c := range s.all {
		c.Update()
	}
}

// ----------------------------------------------------------------------------------
// Counter
// ----------------------------------------------------------------------------------

type Counter struct {
	total int64
	rate  int
	n     int
}

func (s *Counter) Rate() int {
	return s.rate
}

func (s *Counter) Total() int64 {
	return s.total
}

func (s *Counter) Add(n int) {
	s.n += n
}

func (s *Counter) Update() {
	s.rate = s.n
	s.total += int64(s.rate)
	s.n = 0
}
