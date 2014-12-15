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
