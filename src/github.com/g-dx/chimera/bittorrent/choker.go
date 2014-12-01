package bittorrent

import (
	"math/rand"
	"sort"
)

const (
	downloadersSize = 4
)

func ChokePeers(peers []*Peer, optimistic bool) {

	// 1. Unchoke 4 peers I get best download rate from & who are interested in me (downloaders)
	// 2. Unchoke peers who have *better* upload rates but *aren't* interested in me
	// 3. If file is complete use my upload rate to them to decide
	// 4. Optimistic peer rotates every 30 secs. If they are interested they count as 1
	//    of the 4 downloaders

	downloaders := 0
	if optimistic {

		// Get optimistic candidates, choose one and
		can, old := candidateOptimistics(peers)
		new := can[rand.Intn(len(can))]
		if old != nil {
			old.Choke(false)
		}
		new.UnChoke(true)

		// If optimistic is interested counts as a downloader
		if new.IsInterested() {
			downloaders++
		}
	}

	// Order all peers by download rate
	sort.Sort(ByDownloadSpeed(peers))

	// Unchoke all choked peers with best download rate to me who are
	// interested (up to 4) or uninterested
	remaining := 0
	for i, p := range peers {
		if p.IsChoked() {
			p.UnChoke(false)
		}
		if p.IsInterested() {
			downloaders++
			if downloaders == downloadersSize {
				remaining = i
				break
			}
		}
	}

	// All remaining unchoked peers get choked
	for i := remaining; i < len(peers); i++ {
		if !peers[i].IsChoked() {
			peers[i].Choke(false)
		}
	}
}

func candidateOptimistics(peers []*Peer) ([]*Peer, *Peer) {

	var p *Peer
	c := make([]*Peer, len(peers))

	for _, peer := range peers {

		// Mark current optimistic
		if peer.IsOptimistic() {
			p = peer
		}

		// Newly connected peers 3x more likely to start
		if p.IsChoked() {
			c = append(c, peer)
			if peer.IsNew() {
				c = append(c, peer, peer)
			}
		}
	}
	return c, p
}
