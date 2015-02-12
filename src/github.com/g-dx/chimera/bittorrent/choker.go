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
	if b.Peers[i].State().IsInterested() {
		return true
	}
	return false
}

////////////////////////////////////////////////////////////////////////////////////////////////

func ChokePeers(isSeed bool, peers []*Peer, changeOptimistic bool) (*Peer, *Peer, []*Peer, []*Peer) {

	// State to return
	var old, new *Peer
	var chokes, unchokes []*Peer

	// Sanity check
	if len(peers) == 0 {
		return old, new, chokes, unchokes
	}

	// Attempt to calculate a new optimistic peer. If it's interested it counts as a downloader
	n := 0
	if changeOptimistic {
		old, new = chooseOptimistic(peers)
		if new != nil {
			if new.State().IsInterested() {
				n++
			}
			if old != new {
				append(unchokes, new)
			}
		}
	}

	// Order all peers by download or uploaded rate if we are a seed. Iterate from fastest to
	// slowest counting the interested ones until we have filled our upload slots.
	sort.Sort(ByTransferSpeed{peers, isSeed})
	pos := -1
	for i, p := range peers {
		if p == new {
			continue
		}
		if p.State().IsInterested() {
			n++
			pos = i
		}
		if n == uploadSlots {
			break
		}
	}

	// Iterate over peers and calculate chokes & unchokes based on the position of the slowest
	// peer to unchoke. Take care to not change the optimistic unchoke.
	for i, p := range peers {
		if p == new {
			continue
		}
		if i <= pos && p.State().IsChoking() {
			unchokes = append(unchokes, p)
		}
		if i > pos && !p.State().IsChoking() {
			chokes = append(chokes, p)
		}
	}
	return old, new, chokes, unchokes
}

// Select a random peer to be the optimistic
func chooseOptimistic(peers []*Peer) (*Peer, *Peer) {

	var new *Peer
	can, old := buildCandidates(peers)
	new = old
	if len(can) > 0 {
		new = can[random.Intn(len(can))]
	}
	return old, new
}

// Create the list of candidate optimistic unchoke peers.
func buildCandidates(peers []*Peer) ([]*Peer, *Peer) {

	var cur *Peer
	c := make([]*Peer, 0, len(peers))
	for _, p := range peers {
		if p.State().IsOptimistic() {
			cur = p
		}

		// Newly connected peers 3x more likely to start
		if p.State().IsChoking() {
			c = append(c, p)
			if p.State().IsNew() {
				c = append(c, p, p)
			}
		}
	}
	return c, cur
}
