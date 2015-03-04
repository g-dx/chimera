package bittorrent

const (
	_16KB  = int(16 * 1024)
	_128KB = int(128 * 1024)
)

type PieceMap struct {
	pieces []*Piece
	pieceSize int
}

func NewPieceMap(n, pieceLen int, len uint64) *PieceMap {

	pieces := make([]*Piece, n)
	for i := 0; i < n-1; i++ {
		pieces[i] = NewPiece(i, pieceLen)
	}

	// Build last piece
	lastPieceLen := int(len % uint64(pieceLen))
	if lastPieceLen == 0 {
		lastPieceLen = pieceLen
	}
	pieces[n-1] = NewPiece(n-1, lastPieceLen)
	return &PieceMap{pieces, pieceLen}
}

func (pm PieceMap) IsComplete() bool {
	for _, p := range pm.pieces {
		if !p.IsComplete() {
			return false
		}
	}
	return true
}

func (pm PieceMap) Get(i int) *Piece {
	return pm.pieces[i]
}

func (pm *PieceMap) Inc(i int) {
	pm.pieces[i].availability++
}

func (pm *PieceMap) IncAll(bits *BitSet) {
	for i := 0; i < bits.Size(); i++ {
		if bits.Have(i) {
			pm.Inc(i)
		}
	}
}

func (pm *PieceMap) Dec(i int) {
	pm.pieces[i].availability--
}

func (pm *PieceMap) DecAll(bits *BitSet) {
	for i := 0; i < bits.Size(); i++ {
		if bits.Have(i) {
			pm.Dec(i)
		}
	}
}

func (pm *PieceMap) IsValid(index, begin, length int) bool {
	// 1. index valid
	if index >= len(pm.pieces) {
		return false
	}

	// 2. begin + length < size
	piece := pm.pieces[index]
	if begin+length >= piece.Length() {
		return false
	}

	// 3. length < 2^18 (maximum piece size)
	return length < _128KB
}

func (pm *PieceMap) Piece(i int) *Piece {
	return pm.pieces[i]
}

func (p *PieceMap) ReturnOffsets(offs set) {
    for off, _ := range offs {
        index := off/int64(p.pieceSize)
        begin := int(off%int64(p.pieceSize))
        p.pieces[index].ReturnBlock2(begin)
    }
}

func (p *PieceMap) ReturnBlocks(reqs []Request) {
	for _, req := range reqs {
		// Reset block state to needed and ensure overall piece state is blocks needed
		p.pieces[req.index].ReturnBlock(req)
	}
}

func (p *PieceMap) SetBlock(index, begin int, state byte) {
	p.pieces[index].blocks[begin/_16KB] = state
}

func (p *PieceMap) ReturnBlock(index, begin int) {
	// TODO: Check index & begin valid!
	pi := p.pieces[index]
	pi.blocks[begin/_16KB] = NEEDED

	// Ensure we set overall piece state
	pi.state = NOT_STARTED
	for _, s := range pi.blocks {
		if s == REQUESTED {
			pi.state = BLOCKS_NEEDED
			return
		}
	}
}

const (
	NOT_STARTED = iota
	BLOCKS_NEEDED
	FULLY_REQUESTED
	COMPLETE
)

const (
	NEEDED = iota
	REQUESTED
	DOWNLOADED
	DONE
)

type Piece struct {
	index        int
	len          int
	blocks       []uint8
	lastBlockLen int
	availability int
	state        int
}

func NewPiece(i, len int) *Piece {

	// Calculate number of blocks & size of last block
	n := len / _16KB
	lastBlockLen := _16KB
	if len%_16KB != 0 {
		n++
		lastBlockLen = len % _16KB
	}

	// Init all block state to required
	blocks := make([]uint8, n)
	for index := range blocks {
		blocks[index] = NEEDED
	}

	return &Piece{i, len, blocks, lastBlockLen, 0, NOT_STARTED}
}

func (p *Piece) BlockLen(block int) int {
	length := _16KB
	if block == len(p.blocks)-1 {
		length = p.lastBlockLen
	}
	return length
}

func (p *Piece) Priority() int {
	if p.availability == 0 || p.state == FULLY_REQUESTED || p.state == COMPLETE {
		return 0
	}

	// Calculate inverse relationship with availability
	// & give preference to partial downloads
	pri := (1 / float32(p.availability)) * 1000
	if p.state == BLOCKS_NEEDED {
		pri++
	}
	return int(pri)
}

// Are there blocks still to be requested?
func (p *Piece) RequestsRequired() bool {
	return p.state == NOT_STARTED || p.state == BLOCKS_NEEDED
}

// Have all blocks been requested?
func (p *Piece) FullyRequested() bool {
	return p.state == FULLY_REQUESTED
}

func (p *Piece) Complete() {
	p.state = COMPLETE
}

func (p *Piece) Reset() {
    for i, _ := range p.blocks {
        p.blocks[i] = NEEDED
    }
	p.state = NOT_STARTED
}

func (p Piece) Length() int {
	return p.len
}

func (p *Piece) TakeBlocks(n int) []Request {

	reqs := make([]Request, 0, 5)
	state := FULLY_REQUESTED
	for i, s := range p.blocks {

		// Take this block if we still need blocks
		if s == NEEDED && len(reqs) != n {
			p.blocks[i] = REQUESTED

			// Check if this is last block
			length := _16KB
			if i == len(p.blocks)-1 {
				length = p.lastBlockLen
			}
			reqs = append(reqs, Request{p.index, i*_16KB, length})
		}

		// Keep track of the overall piece a
		if p.blocks[i] == NEEDED {
			state = BLOCKS_NEEDED
		}
	}

	// Set overall state & return
	p.state = state
	return reqs
}

func (p *Piece) ReturnBlock(req Request) {
	p.ReturnBlock2(req.begin)
}

func (p *Piece) ReturnBlock2(begin int) {
    p.blocks[begin/_16KB] = NEEDED

    // Ensure we set
    p.state = NOT_STARTED
    for _, s := range p.blocks {
        if s == REQUESTED {
            p.state = BLOCKS_NEEDED
            return
        }
    }
}

func (p *Piece) IsComplete() bool {
	return p.state == COMPLETE
}
