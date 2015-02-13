package bittorrent

import (
	"log"
)

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
	ws         WireState
	bitfield   *BitSet
	id         *PeerIdentity
	statistics *Statistics
	logger     *log.Logger
	blocks 	   set
}

func NewPeer(id *PeerIdentity,
	queue *PeerQueue,
	noOfPieces int,
	logger *log.Logger) *Peer {

	return &Peer{
		id:         id,
		ws:         initWireState,
		bitfield:   NewBitSet(noOfPieces),
		logger:     logger,
		statistics: NewStatistics(),
		queue:      queue,
		blocks: make(set),
	}
}

func (p *Peer) State() WireState {
	return p.ws
}

func (p *Peer) Id() *PeerIdentity {
	return p.id
}

func (p Peer) Stats() *Statistics {
	return p.statistics
}

func (p *Peer) QueuedRequests() int {
	return len(p.blocks)
}

func (p *Peer) Choke() {
	p.ws = p.ws.Choking()
}

func (p *Peer) UnChoke(optimistic bool) {
	ws := p.ws
	// TODO: This isn't great
	if optimistic {
		ws = ws.Optimistic()
	} else {
		ws = ws.NotOptimistic()
	}
	p.ws = ws.NotNew().NotChoking()
}

func (p *Peer) ClearOptimistic() {
	p.ws = p.ws.NotOptimistic()
}

func (p *Peer) Add(pm ProtocolMessage) {
	p.queue.Add(pm)
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
