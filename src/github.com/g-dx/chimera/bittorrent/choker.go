package bittorrent

import (
	"math/rand"
	"sort"
)

const (
	uploadSlots = 4
)

var random = rand.New(rand.NewSource(1))

////////////////////////////////////////////////////////////////////////////////////////////////
// ByTransferSpeed compares
////////////////////////////////////////////////////////////////////////////////////////////////

type Peers []*Peer

func (p Peers) Len() int      { return len(p) }
func (p Peers) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

type ByTransferSpeed struct {
	Peers
	isSeed bool
}

func (b ByTransferSpeed) Less(i, j int) bool {
	irate := b.Peers[i].Stats().Download.Rate()
	jrate := b.Peers[j].Stats().Download.Rate()
	if b.isSeed {
		irate = b.Peers[i].Stats().Upload.Rate()
		jrate = b.Peers[j].Stats().Upload.Rate()
	}

	// Check speeds then interest state
	if irate > jrate {
		return true
	}
	if irate < jrate {
		return false
	}
	if b.Peers[i].IsInterested() {
		return true
	}
	return false
}

////////////////////////////////////////////////////////////////////////////////////////////////

func ChokePeers(isSeed bool, peers []*Peer, changeOptimistic bool) {
	if len(peers) == 0 {
		return
	}

	// Attempt to set a new optimistic peer. If it's interested it counts as a downloader
	n := 0
	if changeOptimistic {
		p := maybeChangeOptimistic(peers)
		if p != nil && p.IsInterested() {
			n++
		}
	}

	// Order all peers by download or uploaded rate if we are a seed. Iterate from fastest to
	// slowest counting the interested ones until we have filled our upload slots.
	sort.Sort(ByTransferSpeed{peers, isSeed})
	pos := -1
	for i, p := range peers {
		if p.IsOptimistic() {
			continue
		}
		if p.IsInterested() {
			n++
			pos = i
		}
		if n == uploadSlots {
			break
		}
	}

	// Iterate over peers and perform chokes & unchokes based on the position of the slowest
	// peer to unchoke. Take care to not change the optimistic unchoke.
	for i, p := range peers {
		if p.IsOptimistic() {
			continue
		}
		if i <= pos && p.IsChoked() {
			p.UnChoke(false)
		}
		if i > pos && !p.IsChoked() {
			p.Choke()
		}
	}
}

func maybeChangeOptimistic(peers []*Peer) *Peer {

	c, cur := buildCandidates(peers)
	if len(c) > 0 {
		if cur != nil {
			cur.ClearOptimistic()
		}
		cur := c[random.Intn(len(c))]
		cur.UnChoke(true)
	}
	return cur
}

// Create the list of candidate optimistic unchoke peers. Also return
func buildCandidates(peers []*Peer) ([]*Peer, *Peer) {

	var cur *Peer
	c := make([]*Peer, 0, len(peers))
	for _, p := range peers {
		if p.IsOptimistic() {
			cur = p
		}

		// Newly connected peers 3x more likely to start
		if p.IsChoked() {
			c = append(c, p)
			if p.IsNew() {
				c = append(c, p, p)
			}
		}
	}
	return c, cur
}
