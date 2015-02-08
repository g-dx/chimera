package bittorrent

import (
	"log"
)

// ----------------------------------------------------------------------------------
// Peer State - key protocol state
// ----------------------------------------------------------------------------------

type PeerState struct {
	ws WireState
	bitfield *BitSet
}

func NewPeerState(bits *BitSet) PeerState {
	return PeerState{
		ws      : initWireState,
		bitfield: bits,
	}
}

// ----------------------------------------------------------------------------------
// BlockOffset - the offset of a particular block in the torrent
// ----------------------------------------------------------------------------------

func blockOffset(index, begin, pieceSize int) int64 {
	return int64(index) * int64(pieceSize) + int64(begin)
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
	blocks 	   set
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
		blocks: make(set),
	}
}

func (p *Peer) Id() *PeerIdentity {
	return p.id
}

func OnMessages(msgs []ProtocolMessage, p *Peer) (error, []ProtocolMessage, []DiskOp, []int64) {

	// State to build during message processing
	var out []ProtocolMessage
	var ops []DiskOp
	var blocks []int64
	var err error
	ws := p.state.ws
	mp := p.pieceMap
	bf := p.state.bitfield

	for _, msg := range msgs {

		var pm ProtocolMessage
		var op DiskOp

		// Handle message
		switch m := msg.(type) {
		case Choke:
			ws, blocks = onChoke(ws, p.blocks)
			p.blocks = make(set)
		case Unchoke: ws = onUnchoke(ws)
		case Interested: ws = onInterested(ws)
		case Uninterested: ws = onUninterested(ws)
		case Bitfield: err, bf, ws, pm = onBitfield([]byte(m), ws, bf.Size(), mp)
		case Have: err, ws, pm = onHave(int(m), ws, bf, mp)
		case Cancel: err = onCancel(m.index, m.begin, m.length)
		case Request: err, op = onRequest(m.index, m.begin, m.length)
		case Block: err, op = onBlock(m.index, m.begin, m.block, p.statistics, p.id)
		case KeepAlive: // Nothing to do...
		default:
			p.logger.Printf("Unknown protocol message: %v", m)
		}

		// Check for error & bail
		if err != nil {
			panic(err)
		}

		// Add outgoing messages
		if pm != nil {
			out = append(out, pm)
		}

		// Add disk ops
		if op != nil {
			ops = append(ops, op)
		}
	}

	// Update peer state
	p.state.ws = ws
	p.state.bitfield = bf
	return nil, out, ops, blocks
}

func onChoke(ws WireState, blocks set) (WireState, []int64) {
	ret := make([]int64, len(blocks))
	for block, _ := range blocks {
		ret = append(ret, block)
	}
	return ws.Choked(), ret
}

func onUnchoke(ws WireState) WireState {
	return ws.NotChoked()
}

func onInterested(ws WireState) WireState {
	return ws.Interested()
}

func onUninterested(ws WireState) WireState {
	return ws.NotInterested()
}

func onHave(index int, ws WireState, bitfield *BitSet, mp *PieceMap) (error, WireState, ProtocolMessage) {

	// Validate
	if !bitfield.IsValid(index) {
		return newError("Invalid index received: %v", index), ws, nil
	}

	var msg ProtocolMessage
	if !bitfield.Have(index) {

		// Update bitfield & update availability
		bitfield.Set(index)
		mp.Inc(index)

		if isNowInteresting(index, ws, mp) {
			ws = ws.Interesting()
			msg = Interested{}
		}
	}
	return nil, ws, msg
}

func onCancel(index, begin, length int) error {
	// TODO: Implement cancel support
	return nil
}

func onRequest(index, begin, length int) (error, DiskOp) {

	// TODO: Check request valid
	//	p.pieceMap.IsValid()

	// Get block message and pass to disk to fill
//	p.disk.Read(index, begin, p.id)
	return nil, nil
}

func onBlock(index, begin int, block []byte, s *Statistics, id *PeerIdentity) (error, DiskOp) {
	// NOTE: Already on the way to disk...
	// p.disk.Write(index, begin, length, p.id)
//	p.disk.Write(index, begin, block)
	s.Download.Add(len(block))
	return nil, ReadOp{id, Block{index, begin, block}}
}

func onBitfield(bits []byte, ws WireState, n int, mp *PieceMap) (error, *BitSet, WireState, ProtocolMessage) {

	// Create & validate bitfield
	bitfield, err := NewBitSetFrom(bits, n)
	if err != nil {
		return err, nil, ws, nil
	}

	// Update availability in global piece map
	mp.IncAll(bitfield)

	// Check if we are interested
	var msg ProtocolMessage
	for i := 0; i < bitfield.Size(); i++ {
		if mp.Piece(i).RequestsRequired() {
			ws = ws.Interesting()
			msg = Interested{}
			break
		}
	}
	return nil, bitfield, ws, msg
}

func (p Peer) Stats() *Statistics {
	return p.statistics
}

func isNowInteresting(index int, ws WireState, mp *PieceMap) bool {
	return !ws.IsInteresting() && mp.Piece(index).RequestsRequired()
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

func (p *Peer) Choke() error {
	p.state.ws = p.state.ws.Choking()
	return p.Add(Choke{})
}

func (p *Peer) UnChoke(optimistic bool) error {
	ws := p.state.ws
	// TODO: This isn't great
	if optimistic {
		ws = ws.Optimistic()
	} else {
		ws = ws.NotOptimistic()
	}
	p.state.ws = ws.NotNew().NotChoking()

	return p.Add(Unchoke{})
}

func (p *Peer) IsInterested() bool {
	return p.state.ws.IsInterested()
}

func (p *Peer) IsChoked() bool {
	return p.state.ws.IsChoked()
}

func (p *Peer) IsChoking() bool {
	return p.state.ws.IsChoking()
}

func (p *Peer) IsOptimistic() bool {
	return p.state.ws.IsOptimistic()
}

func (p *Peer) ClearOptimistic() {
	p.state.ws = p.state.ws.NotOptimistic()
}

func (p *Peer) IsNew() bool {
	return p.state.ws.IsNew()
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
