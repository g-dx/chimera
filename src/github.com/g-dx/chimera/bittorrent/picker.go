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

func PickPieces(peers []*Peer, pieceMap *PieceMap, requestTimer *ProtocolRequestTimer) bool {

	// Sort into fastest downloaders
	sort.Sort(ByDownloadSpeed(peers))

	// For each unchoked & interesting peer calculate blocks to pick
	pieces := make([]*Piece, 0, len(pieceMap.pieces))
	for _, peer := range peers {
		if peer.CanDownload() {
			// Find all pieces which this peer has which still require blocks
//			peer.logger.Println("Picking for peer: ", peer.Id())
			for _, piece := range pieceMap.pieces {
				if !piece.FullyRequested() && peer.state.bitfield.Have(piece.index) {
					pieces = append(pieces, piece)
				}
			}

//			peer.logger.Println("peer: ", peer.Id(), " pieces: ", pieces)

			// Sort peer-specific pieces by priority
			sort.Sort(ByPriority(pieces))

			// Ensure an outstanding queue of 10
			// TODO: Fix me!
			n := peer.QueuedRequests() + requestTimer.BlocksWaiting(*peer.Id())
//			peer.logger.Println("peer: ", peer.Id(), " blocks outstanding: ", n)
			TakeBlocks(pieces, 10 - n, peer)
			/**
			  reqs := requestsForNeededBlocks()
			  for req : range reqs {
			    newPieceMap.Get(req.index).SetBlock(REQUESTED)
			  }
			 */
		}

		// Clear peer list
		pieces = pieces[:0]
	}

	return false
}

func TakeBlocks(pieces []*Piece, numRequired int, p *Peer) {
	n := numRequired
	for _, piece := range pieces {
		reqs := piece.TakeBlocks(n)
//		for _, msg := range reqs {
//			p.logger.Println("peer: ", p.Id(), " req: ", ToString(msg))
//		}

		n -= len(reqs)
		for _, req := range reqs {
			p.Add(req)
		}
	}
}

func PickPieces2(peers []*Peer, pieceMap *PieceMap, requestTimer *ProtocolRequestTimer) map[*Peer][]Request {

	// Sort into fastest downloaders
	sortedPeers := make([]*Peer, len(peers))
	copy(sortedPeers, peers)
	sort.Sort(ByDownloadSpeed(sortedPeers))

	// For each unchoked & interesting peer calculate blocks to pick

	taken := make(set)    // The blocks which have already been taken
	complete := make(set) // The pieces whose block have are all requested or taken
	picked := make(map[*Peer][]Request) // The blocks picked for each peer

	for _, peer := range peers {
		if peer.CanDownload() {

			// Find total required
			n := peer.QueuedRequests() + requestTimer.BlocksWaiting(*peer.Id())

			reqs := make([]Request, 0, n)
			for _, p := range availablePieces(complete, pieceMap, peer, n) {

				// Pick all the pieces we can
				blocks, done := pick(taken, p, n)

				// Update taken blocks
				for _, req := range blocks {
					taken.Add(toOffset(p, int(req.begin/_16KB)))
				}

				// If we didn't complete the piece we must be done
				if !done {
					break
				}

				// Mark piece as done & continue
				complete.Add(int64(p.index))
			}

			// Set picked pieces for peer
			picked[peer] = reqs
		}
	}

	return picked
}

func availablePieces(complete set, pieceMap *PieceMap, peer *Peer, n int) []*Piece {
	// Find all pieces which this peer has which still require blocks
	pieces := make([]*Piece, 0, n)
	for _, piece := range pieceMap.pieces {

		// If this piece hasn't been completed already AND it isn't fully requested AND this peer has it
		if !complete.Has(int64(piece.index)) && !piece.FullyRequested() && peer.state.bitfield.Have(piece.index) {
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


func pick(taken set, p *Piece, wanted int) ([]Request, bool) {

	reqs := make([]Request, 0, wanted)
	for i, s := range p.blocks {

		// If not already taken & needed
		if !taken.Has(toOffset(p, i)) && s == NEEDED {

			// Add request & check if we are done
			reqs = append(reqs, Request{p.index, i*_16KB, p.BlockLen(i)})
			if len(reqs) == wanted {
				return reqs, false
			}
		}
	}
	return reqs, true
}

// Return the global offset of a block within the entire file
func toOffset(p *Piece, block int) int64 {
	return int64(p.index) * int64(p.len) + int64(block * 16 * 1024) // TODO: Should use constant
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
