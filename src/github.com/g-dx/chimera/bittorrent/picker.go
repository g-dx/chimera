package bittorrent

import (
	"fmt"
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
	return b[i].Stats().bytesDownloadedPerUpdate < b[j].Stats().bytesDownloadedPerUpdate
}

type ByPriority []*Piece

func (b ByPriority) Len() int           { return len(b) }
func (b ByPriority) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByPriority) Less(i, j int) bool { return b[i].Priority() < b[j].Priority() }

func PickPieces(peers []*Peer, pieceMap *PieceMap) bool {

	fmt.Println("Running piece picker...")

	// Update counters
	for _, p := range peers {
		p.Stats().Update()
	}

	// Sort into fastest downloaders
	sort.Sort(ByDownloadSpeed(peers))

	// For each unchoked & interesting peer calculate blocks to pick
	pieces := make([]*Piece, 0, len(pieceMap.pieces))
	for _, peer := range peers {
		if peer.CanDownload() {

			fmt.Printf("Finding pieces for peer: %v\n", peer.id)

			// Find all pieces which this peer has which still require blocks
			for _, piece := range pieceMap.pieces {
				if !piece.FullyRequested() && peer.state.bitfield.Have(piece.index) {
					pieces = append(pieces, piece)
				}
			}

			// Sort peer-specific pieces by priority
			sort.Sort(ByPriority(pieces))

			fmt.Printf("Peer: %v rarest pieces: %v\n", peer.id, pieces)

			// Attempt to pick the number of required blocks
			TakeBlocks(pieces, peer.BlocksRequired(), peer)
		}

		// Clear peer list
		pieces = pieces[:0]
	}

	return false
}

func TakeBlocks(pieces []*Piece, numRequired int, p *Peer) {
	n := numRequired
	for _, piece := range pieces {
		reqs := piece.TakeBlocks(p.id, n)
		n -= len(reqs)
		for _, req := range reqs {
			fmt.Printf("Picker: %v, Peer: %v\n", req, p.id)
			p.queue.Add(req)
		}
	}
}
