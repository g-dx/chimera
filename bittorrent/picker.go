package bittorrent

import (
	"sort"
	"fmt"
)

/**
 * Picker algorithm :
 * 1. Sort peers into fastest downloaders.
 * 2. For each peer calculate a list of the 20 rarest pieces this peer is able to download
 *    that are not already being downloaded.
 * 3. Randomly attempt to choose blocks from those pieces until the desired number is achieved.
 */

type ByDownloadSpeed []*Peer

func (b ByDownloadSpeed) Len() int { return len(b) }
func (b ByDownloadSpeed) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b ByDownloadSpeed) Less(i, j int) bool {
	return b[i].Statistics().bytesDownloadedPerUpdate < b[j].Statistics().bytesDownloadedPerUpdate
}

type ByAvailability []*Piece
func (b ByAvailability) Len() int { return len(b) }
func (b ByAvailability) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b ByAvailability) Less(i, j int) bool { return b[i].Availability() < b[j].Availability() }

func PickPieces(peers []*Peer, pieceMap *PieceMap) {

	fmt.Println("Running piece picker...")
	// Sort by availability
	sort.Sort(ByAvailability(pieceMap.pieces))

	// Update counters
	for _, p := range peers {
		p.Statistics().Update()
	}

	// Sort into fastest downloaders
	sort.Sort(ByDownloadSpeed(peers))

	// For each unchoked & interesting peer calculate the rarest 20 pieces they have
	for _, peer := range peers {
		if peer.CanDownload() {

			fmt.Printf("Finding pieces for peer: %v\n", peer.id)

			// Find 10 rarest available pieces with blocks still required
			pieces := make([]*Piece, 0, 10)
			for _, piece := range pieceMap.pieces {
				if piece.state == BLOCKS_NEEDED && peer.state.bitfield.Have(piece.index) {
					pieces = append(pieces, piece)
					if len(pieces) == 10 {
						break
					}
				}
			}

			fmt.Printf("Peer: %v rarest pieces: %v\n", peer.id, pieces)

			// Attempt to pick the number of required blocks
			TakeBlocks(pieces, peer.BlocksRequired(), peer)
		}
	}
}

func TakeBlocks(pieces []*Piece, numRequired uint, p *Peer) {
	n := numRequired
	for _, piece := range pieces {
		reqs := piece.TakeBlocks(n)
		n -= uint(len(reqs))
		for _, req := range reqs {
			fmt.Printf("Picker: %v, Peer: %v\n", req, p.id)
			p.localQ.Add(req)
		}
	}
}
