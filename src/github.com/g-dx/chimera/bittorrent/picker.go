package bittorrent

import (
	"sort"
)

/**
 * Picker algorithm :
 * 1. Sort peers into fastest downloaders.
 * 2. For each peer calculate a list of the 20 rarest pieces this peer is able to download
 *    that are not already being downloaded.
 * 3. Randomly attempt to choose blocks from those pieces until the desired number is achieved.
 */

type ByDownloadSpeed []*Peer

func (b ByDownloadSpeed) Len() int      { return len(b) }
func (b ByDownloadSpeed) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b ByDownloadSpeed) Less(i, j int) bool {
	return b[i].Stats().Download.Rate() > b[j].Stats().Download.Rate()
}

type ByPriority []*Piece

func (b ByPriority) Len() int           { return len(b) }
func (b ByPriority) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByPriority) Less(i, j int) bool { return b[i].Priority() < b[j].Priority() }

func PickPieces(peers []*Peer, pieceMap *PieceMap) map[*Peer][]Request {

	// Sort into fastest downloaders
	sortedPeers := make([]*Peer, len(peers))
	copy(sortedPeers, peers)
	sort.Sort(ByDownloadSpeed(sortedPeers))

	// For each unchoked & interesting peer calculate blocks to pick

	taken := make(set)    // The blocks which have already been taken
	complete := make(set) // The pieces whose block have are all requested or taken
	picked := make(map[*Peer][]Request) // The blocks picked for each peer

	for _, peer := range sortedPeers {
		if peer.State().CanDownload() {

			// Find total required
			n := 10 - peer.QueuedRequests() // TODO: Make configurable

			reqs := make([]Request, 0, n)
			for _, p := range availablePieces(complete, pieceMap, peer.bitfield, n) {

				// Pick all the pieces we can
				blocks, wanted, pieceDone := pick(taken, p, n, pieceMap.pieceSize)
				reqs = append(reqs, blocks...)
				n -= len(blocks)

				// Update taken blocks
				for _, block := range blocks {
					taken.Add(toOffset(p.index, int(block.begin/_16KB), pieceMap.pieceSize))
				}

				if pieceDone {
					complete.Add(int64(p.index))
				}

				// If we got all we wanted
				if wanted {
					break
				}
			}

			// Set picked pieces for peer
			picked[peer] = reqs
		}
	}

	return picked
}

func availablePieces(complete set, pieceMap *PieceMap, bitfield *BitSet, n int) []*Piece {

	// Find all pieces which this peer has which still require blocks
	pieces := make([]*Piece, 0, n)
	if n == 0 {
		return pieces
	}

	for _, piece := range pieceMap.pieces {

		// If this piece hasn't been completed already AND it isn't fully requested AND this peer has it
		if !complete.Has(int64(piece.index)) && !piece.FullyRequested() && bitfield.Have(piece.index) {
			pieces = append(pieces, piece)
		}

		// Each piece must have at least one free block so take at most the number of blocks
		if len(pieces) == n {
			break
		}
	}

	// Sort them
	sort.Sort(ByPriority(pieces))
	return pieces
}


func pick(taken set, p *Piece, wanted, pieceSize int) ([]Request, bool, bool) {

	reqs := make([]Request, 0, wanted)
	for i, s := range p.blocks {

		// If not already taken & needed
		if !taken.Has(toOffset(p.index, i, pieceSize)) && s == NEEDED {

			// Add request & check if we are done
			reqs = append(reqs, Request{p.index, i*_16KB, p.BlockLen(i)})
			if len(reqs) == wanted {
				return reqs, true, i == len(p.blocks)-1
			}
		}
	}
	return reqs, false, false
}

// Return the global offset of a block within the entire file
func toOffset(index, block, pieceSize int) int64 {
	return int64(index) * int64(pieceSize) + int64(block * 16 * 1024) // TODO: Should use constant
}

// ============================================================================================================
// Set
// ============================================================================================================

type set map[int64]struct{}

func (b set) Has(i int64) bool {
	_, has := b[i]
	return has
}

func (b set) Add(i int64) {
	b[i] = struct{}{}
}

func (b set) Remove(i int64) {
	delete(b, i)
}