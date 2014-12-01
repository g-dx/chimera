package bittorrent

import (
	"math/rand"
	"sort"
)

const (
	maxDownloaders = 4
)

var random = rand.New(rand.NewSource(1))

////////////////////////////////////////////////////////////////////////////////////////////////
// ByTransferSpeed compares
////////////////////////////////////////////////////////////////////////////////////////////////

type ByTransferSpeed struct {
	[]*Peer
	isSeed bool
}

func (b ByTransferSpeed) Len() int      { return len(b) }
func (b ByTransferSpeed) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b ByTransferSpeed) Less(i, j int) bool {
	// If seed use upload speed otherwise use download
	if b.isSeed {
		return b[i].Stats().Upload() < b[j].Stats().Upload()
	}
	return b[i].Stats().Download() < b[j].Stats().Download()
}

////////////////////////////////////////////////////////////////////////////////////////////////

func ChokePeers(isSeed bool, peers []*Peer, optimistic bool) {
	if len(peers) == 0 {
		return
	}

    // Build a list of candidate optimistic unchokes plus the current optimistic
    // unchoke. Randomly select one unchoke it. If it's interested it counts as
    // a downloader
	n := 0
	if optimistic {
		candidates, old := optimisticCandidates(peers)
		new := candidates[random.Intn(len(candidates))]
		new.UnChoke(true)
		if new.IsInterested() {
			n++
		}
		
		// There may be no previous optimistic
		if old != nil {
			old.Choke(false)
		}
	}
	
	// Order all peers by download or uploaded rate if we are a seed. Iterate from fastest to
	// slowest unchoking all peers and counting the interested ones as downloaders. When we
	// have 4 downloaders choke the remaining peers who are unchoked
	sort.Sort(&ByTransferSpeed { peers, isSeed })
	chokeRemaining := false
	for _, p := range peers {
		if p.IsOptimistic() {
			continue
		}
	    if !p.IsChoked() && chokeRemaining {
	    	p.Choke(false)
	    	continue
		}
		if p.IsChoked() {
			p.UnChoke(false)
		}
		if p.IsInterested() {
			n++
			if n == maxDownloaders {
				chokeRemaining = true
			}
		}
	}
}

// Create the list of candidate optimistic unchoke peers. Also return
func optimisticCandidates(peers []*Peer) ([]*Peer, *Peer) {

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
