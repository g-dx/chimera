package bittorrent

import (
	"math/rand"
	"sort"
)

const (
	downloadersSize = 4
)

func ChokePeers(peers []*Peer, optimistic bool) {

        // Build a list of candidate optimistic unchokes plus the current optimistic 
        // unchoke. Randomly select one unchoke it. If it's interested it counts as 
        // a downloader
	n := 0
	if optimistic {
		candidates, old := candidateOptimistics(peers)
		new := candidates[rand.Intn(len(candidates))]
		new.UnChoke(true)
		if new.IsInterested() {
			n++
		}
		
		// There may be no previous optimistic
		if old != nil {
			old.Choke(false)
		}
	}
	
	// Order all peers by download rate. Iterate from fastest to slowest unchoking all peers and
	// counting the interested ones as downloaders. When we have 4 downloaders choke the remaining
	// peers who are unchoked
	sort.Sort(ByDownloadSpeed(peers))
	chokeRemaining := false
	for i, p := range peers {
	        if !p.IsChoked() && chokeRemaining {
	        	p.Choke(false)
	        	continue
	        }
		if p.IsChoked() {
			p.UnChoke(false)
		}
		if p.IsInterested() {
			n++
			if n == downloadersSize {
				chokeRemaining = true
			}
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
