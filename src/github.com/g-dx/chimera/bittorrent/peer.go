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
		state:      NewPeerState(NewBitSet(len(pieceMap.pieces))),
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
	case Choke:
		return p.onChoke()
	case Unchoke:
		return p.onUnchoke()
	case Interested:
		return p.onInterested()
	case Uninterested:
		return p.onUninterested()
	case Bitfield:
		return p.onBitfield(msg)
	case Have:
		return p.onHave(int(msg))
	case Cancel:
		return p.onCancel(msg.index, msg.begin, msg.length)
	case Request:
		return p.onRequest(msg.index, msg.begin, msg.length)
	case Block:
		return p.onBlock(msg.index, msg.begin, msg.block)
	case KeepAlive:
		return nil
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

func (p *Peer) onHave(index int) error {

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
			p.queue.Add(Interested{})
		}
	}
	return nil
}

func (p *Peer) onCancel(index, begin, length int) error {
	// TODO: Implement cancel support
	return nil
}

func (p *Peer) onRequest(index, begin, length int) error {

	// TODO: Check request valid
	//	p.pieceMap.IsValid()

	// Get block message and pass to disk to fill
//	p.disk.Read(index, begin, p.id)
	return nil
}

func (p *Peer) onBlock(index, begin int, block []byte) error {
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
	for i := 0; i < p.state.bitfield.Size(); i++ {
		if p.pieceMap.Piece(i).RequestsRequired() {
			p.state.localInterest = true
			p.queue.Add(Interested{})
			break
		}
	}
	return nil
}

func (p Peer) Stats() *Statistics {
	return p.statistics
}

func (p *Peer) isNowInteresting(index int) bool {
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
	return p.Add(Choke{})
}

func (p *Peer) UnChoke(optimistic bool) error {
	p.state.optimistic = optimistic
	p.state.new = false
	p.state.remoteChoke = false
	return p.Add(Unchoke{})
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

func (p *Peer) Cancel(index, begin, len int) error {
	return p.Add(Cancel{index, begin, len})
}

func (p *Peer) Add(pm ProtocolMessage) error {
	p.queue.Add(pm)
	return nil // TODO: Fix this!
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
