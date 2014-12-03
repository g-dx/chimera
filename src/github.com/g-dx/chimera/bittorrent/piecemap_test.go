package bittorrent

import (
	"testing"
)

func TestNewPieceMap(t *testing.T) {

	// Piece = 512Kb
	// Total Len = 13.2M
	pm := NewPieceMap(26, 524288, 13207200)
	intEquals(t, 26, len(pm.pieces))

	for i := uint32(0); i < 25; i++ {
		p := pm.Get(i)
		intEquals(t, 32, len(p.blocks))
	}

	p := pm.Get(25)
	intEquals(t, 7, len(p.blocks))
}

func TestNewRegularPiece(t *testing.T) {

	// 1 - 10 = 16Kb
	i := uint32(0)
	l := uint32(163840)
	p := NewPiece(i, l)
	intEquals(t, 10, len(p.blocks))
	intEquals(t, 0, p.Priority())
	boolEquals(t, true, p.RequestsRequired())
	boolEquals(t, false, p.FullyRequested())
	boolEquals(t, false, p.IsComplete())
	uint32Equals(t, l, p.len)
	uint32Equals(t, _16KB, p.lastBlockLen)

	// Check all requests
	r := p.TakeBlocks(nil, 10)
	intEquals(t, 10, len(r))

	// Requests 1 - 10
	for j, req := range r {
		t.Logf("%v", *req)
		uint32Equals(t, i, req.Index())
		uint32Equals(t, _16KB*uint32(j), req.Begin())
		uint32Equals(t, _16KB, req.Length())
	}
}

func TestNewIrregularPiece(t *testing.T) {

	// 1 - 10 = 16Kb, 11 = 124b
	i := uint32(0)
	l := uint32(163964)
	p := NewPiece(i, l)
	intEquals(t, 11, len(p.blocks))
	uint32Equals(t, 124, p.lastBlockLen)

	// Check all requests
	r := p.TakeBlocks(nil, 11)
	intEquals(t, 11, len(r))

	// Requests 1 - 10
	for j, req := range r {
		if j != 10 {
			uint32Equals(t, i, req.Index())
			uint32Equals(t, _16KB*uint32(j), req.Begin())
			uint32Equals(t, _16KB, req.Length())
		}
	}

	// Request 11
	req := r[10]
	uint32Equals(t, i, req.Index())
	uint32Equals(t, _16KB*10, req.Begin())
	uint32Equals(t, 124, req.Length())
}

func TestPieceStateAndPriority(t *testing.T) {

	// Create piece, set availability
	blocks := 10
	p := NewPiece(0, _16KB*uint32(blocks))
	p.availability = 5
	notStartedPriority := p.Priority()

	// Take some blocks
	reqs := p.TakeBlocks(nil, 2)
	blocksNeedPriority := p.Priority()

	// Check state & increased priority
	intEquals(t, BLOCKS_NEEDED, p.state)
	boolEquals(t, true, p.RequestsRequired())
	if notStartedPriority >= blocksNeedPriority {
		t.Errorf("Expected: %v > %v", p.Priority(), p)
	}

	// Return blocks
	p.ReturnBlock(reqs[0])
	p.ReturnBlock(reqs[1])

	// Ensure not started & reduced priority
	intEquals(t, NOT_STARTED, p.state)
	boolEquals(t, true, p.RequestsRequired())
	intEquals(t, notStartedPriority, p.Priority())

	// Take all the blocks
	reqs = p.TakeBlocks(nil, blocks)
	intEquals(t, blocks, len(reqs))

	// Ensure fully requested and zero priority
	intEquals(t, FULLY_REQUESTED, p.state)
	boolEquals(t, true, p.FullyRequested())
	boolEquals(t, false, p.RequestsRequired())
	intEquals(t, 0, p.Priority())

	// Return one block
	p.ReturnBlock(reqs[9])

	// Check blocks needed and increased priority
	intEquals(t, BLOCKS_NEEDED, p.state)
	intEquals(t, blocksNeedPriority, p.Priority())

	// Check complete state zero priority
	// TODO: Decide on API to set complete state
	p.state = COMPLETE
	intEquals(t, 0, p.Priority())
}