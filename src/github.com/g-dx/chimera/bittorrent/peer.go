package bittorrent

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
	ws         WireState
	bitfield   *BitSet
	id         PeerIdentity
	statistics *Statistics
	blocks 	   set
}

func NewPeer(id PeerIdentity, noOfPieces int) *Peer {

	return &Peer{
		id:         id,
		ws:         initWireState,
		bitfield:   NewBitSet(noOfPieces),
		statistics: NewStatistics(),
		blocks: make(set),
	}
}

func (p *Peer) State() WireState {
	return p.ws
}

func (p *Peer) Id() PeerIdentity {
	return p.id
}

func (p Peer) Stats() *Statistics {
	return p.statistics
}

func (p *Peer) QueuedRequests() int {
	return len(p.blocks)
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
