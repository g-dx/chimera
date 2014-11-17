package bittorrent

const (
	_16KB = uint32(16 * 1024)
	_128KB = uint32(128 * 1024)
)

type PieceMap struct {
	pieces []*Piece
}

func NewPieceMap(n, pieceLen uint32, mapLen uint64) *PieceMap {

	pieces := make([]*Piece, n)
	for i := uint32(0); i < n-1; i++ {
		pieces[i] = NewPiece(i, pieceLen)
	}

	// Build last piece
	lastPieceLen := uint32(mapLen % uint64(pieceLen))
	if lastPieceLen == 0 {
		lastPieceLen = pieceLen
	}
	pieces[n-1] = NewPiece(n-1, lastPieceLen)
	return &PieceMap { pieces }
}

func (pm PieceMap) Get(i uint32) *Piece {
	return pm.pieces[i]
}

func (pm * PieceMap) Inc(i uint32) {
	pm.pieces[i].availability++
}

func (pm * PieceMap) IncAll(bits *BitSet) {
	for i := uint32(0); i < bits.Size(); i++ {
		if bits.Have(i) {
			pm.Inc(i)
		}
	}
}

func (pm * PieceMap) Dec(i uint32) {
	pm.pieces[i].availability--
}

func (pm * PieceMap) DecAll(bits *BitSet) {
	for i := uint32(0); i < bits.Size(); i++ {
		if bits.Have(i) {
			pm.Dec(i)
		}
	}
}

func (pm * PieceMap) IsValid(index, begin, length uint32) bool {
	// 1. index valid
	if index >= uint32(len(pm.pieces)) {
		return false
	}

	// 2. begin + length < size
	piece := pm.pieces[index]
	if begin + length >= piece.Length() {
		return false
	}

	// 3. length < 2^18 (minimum piece size)
	if length >= _128KB {
		return false
	}
	return true
}

func (pm * PieceMap) Piece(i uint32) *Piece {
	return pm.pieces[i]
}

func (p * PieceMap) ReturnBlocks(reqs []*RequestMessage) {
	for _, req := range reqs {
		// Reset block state to needed and ensure overall piece state is blocks needed
		piece := p.pieces[req.Index()]
		piece.blocks[req.Begin()%_16KB] = NEEDED
		piece.state = BLOCKS_NEEDED
	}
}

const (
	BLOCKS_NEEDED = iota
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
	index uint32
	len uint32
	blocks []uint8
	lastBlockLen uint32
	availability uint32
	state int
}

func NewPiece(i, len uint32) *Piece {

	// Calculate number of blocks & size of last block
	n := len / _16KB
	lastBlockLen := _16KB
	if len % _16KB != 0 {
		n++
		lastBlockLen = len % _16KB
	}

	// Init all block state to required
	blocks := make([]uint8, n)
	for index := range blocks {
		blocks[index] = NEEDED
	}

	return &Piece { i, len, blocks, lastBlockLen, 0, BLOCKS_NEEDED }
}

func (p Piece) Availability() uint32 {
	return p.availability
}

func (p Piece) BlocksNeeded() bool {
	return p.state == BLOCKS_NEEDED
}

func (p Piece) Length() uint32 {
	return p.len
}

func (p * Piece) TakeBlocks(n int) []*RequestMessage {

	blocks := make([]*RequestMessage, 0, 5)
	state := FULLY_REQUESTED
	for i, s := range p.blocks {

		// Take this block if we still need blocks
		if s == NEEDED && len(blocks) != n {
			p.blocks[i] = REQUESTED
			blocks = append(blocks, Request(p.index, uint32(i) * _16KB, _16KB))
		}

		// Keep track of the overall piece a
		if p.blocks[i] == NEEDED {
			state = BLOCKS_NEEDED
		}
	}

	// Set overall state & return
	p.state = state
	return blocks
}

func (p * Piece) BlockDone(begin uint32) {

	// Set state
	p.blocks[begin/_16KB] = DONE

	// Check overall piece
	isComplete := true
	for _, block := range p.blocks {
		if block != DONE {
			isComplete = false
			break
		}
	}

	if isComplete {
		p.state = COMPLETE
	}
}

func (p * Piece) IsComplete() bool {
	return p.state == COMPLETE
}

func (p Piece) BlockLen(i uint32) uint32 {
	blockLen := _16KB
	if uint32(len(p.blocks)-1) == i {
		blockLen = p.lastBlockLen
	}
	return blockLen
}
