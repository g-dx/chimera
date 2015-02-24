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

func PickPieces(peers []*Peer, pieceMap *PieceMap) map[*Peer][]ProtocolMessage {

	// Sort into fastest downloaders
	sortedPeers := make([]*Peer, len(peers))
	copy(sortedPeers, peers)
	sort.Sort(ByDownloadSpeed(sortedPeers))

	// For each unchoked & interesting peer calculate blocks to pick

	takenBlocks := make(set)    // The blocks which have already been taken
	completedPieces := make(set) // The pieces whose block have are all requested or taken
	picked := make(map[*Peer][]ProtocolMessage) // The blocks picked for each peer

	for _, peer := range sortedPeers {
		if peer.State().CanDownload() {

			// Find total required
			n := 10 - peer.QueuedRequests() // TODO: Make configurable

			r := make([]ProtocolMessage, 0, n)
			for _, p := range availablePieces(completedPieces, pieceMap, peer.bitfield, n) {

				// Pick all the pieces we can
				blocks, done := pickBlocks(takenBlocks, p, n, pieceMap.pieceSize)
                if done {
                    completedPieces.Add(int64(p.index))
                }

                // Add to peer picks and update taken set
				for _, block := range blocks {
                    r = append(r, block)
					takenBlocks.Add(toOffset(p.index, int(block.begin/_16KB), pieceMap.pieceSize))
				}

                if len(blocks) == n {
                    break
                }
			}

			// Set picked pieces for peer
			picked[peer] = r
		}
	}

	return picked
}

func availablePieces(complete set, pieceMap *PieceMap, bitfield *BitSet, n int) []*Piece {

	// Find all pieces which this peer has which still require blocks
	pieces := make([]*Piece, 0, n)
	for _, piece := range pieceMap.pieces {

        // Each piece must have at least one free block so take at most the number of blocks
        if len(pieces) == n {
            break
        }

		// If this piece hasn't been completed already AND it isn't fully requested AND this peer has it
		if !complete.Has(int64(piece.index)) && !piece.FullyRequested() && bitfield.Have(piece.index) {
			pieces = append(pieces, piece)
		}
	}

	// Sort them
	sort.Sort(ByPriority(pieces))
	return pieces
}


func pickBlocks(taken set, p *Piece, wanted, pieceSize int) ([]Request, bool) {

	reqs := make([]Request, 0, wanted)
	for i, s := range p.blocks {

		// If not already taken & needed
		if !taken.Has(toOffset(p.index, i, pieceSize)) && s == NEEDED {

			// Add request & check if we are done
			reqs = append(reqs, Request{p.index, i*_16KB, p.BlockLen(i)})
			if len(reqs) == wanted {
				return reqs, i == len(p.blocks)-1
			}
		}
	}
	return reqs, false
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