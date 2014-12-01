package bittorrent

import "testing"

func TestChokePeersGivenNoPeers(t *testing.T) {
	ChokePeers(false, make([]*Peer, 0), false)
	ChokePeers(false, make([]*Peer, 0), true)
	ChokePeers(true, make([]*Peer, 0), false)
	ChokePeers(false, make([]*Peer, 0), true)
	ChokePeers(true, make([]*Peer, 0), true)
}

func TestChokePeersGivenOnePeer(t *testing.T) {

	p1 := pr()
	peers := append([]*Peer(nil), p1)

	ChokePeers(false, peers, false)
	boolEquals(t, false, p1.IsChoked())
	boolEquals(t, false, p1.IsOptimistic())

	// Should be unchoked & optimistic
	ChokePeers(false, peers, true)
	boolEquals(t, false, p1.IsChoked())
	boolEquals(t, true, p1.IsOptimistic())

	ChokePeers(true, peers, false)
	boolEquals(t, false, p1.IsChoked())
	boolEquals(t, false, p1.IsOptimistic())

	// Should be unchoked & optimistic
	ChokePeers(false, peers, true)
	boolEquals(t, false, p1.IsChoked())
	boolEquals(t, true, p1.IsOptimistic())

	// Should be unchoked & optimistic
	ChokePeers(true, peers, true)
	boolEquals(t, false, p1.IsChoked())
	boolEquals(t, true, p1.IsOptimistic())
}

func TestChokePeersGiven8PeersWhenNotOptimisticAndNotSeed(t *testing.T) {

	p1 := pr().dl(1)
	p2 := pr().dl(2).interested()
	p3 := pr().dl(3)
	p4 := pr().dl(4).interested()
	p5 := pr().dl(5).interested()
	p6 := pr().dl(6).interested()
	p7 := pr().dl(7).interested()
	p8 := pr().dl(8)
	peers := append([]*Peer(nil), p1, p2, p3, p4, p5, p6, p7, p8)
	ChokePeers(false, peers, false)
	// Choked
	boolEquals(t, true, p1.IsChoked())
	boolEquals(t, true, p2.IsChoked())
	boolEquals(t, true, p3.IsChoked())
	// Unchoked
	boolEquals(t, false, p4.IsChoked())
	boolEquals(t, false, p5.IsChoked())
	boolEquals(t, false, p6.IsChoked())
	boolEquals(t, false, p7.IsChoked())
	boolEquals(t, false, p8.IsChoked())

	// Alter download speeds & run choker
	p3.dl(9)
	p2.dl(10)
	ChokePeers(false, peers, false)
	// Choked
	boolEquals(t, true, p1.IsChoked())
	boolEquals(t, true, p4.IsChoked())
	// Unchoked
	boolEquals(t, false, p5.IsChoked())
	boolEquals(t, false, p6.IsChoked())
	boolEquals(t, false, p7.IsChoked())
	boolEquals(t, false, p8.IsChoked())
	boolEquals(t, false, p3.IsChoked())
	boolEquals(t, false, p2.IsChoked())

	// Alter interest & run choker
	p4.uninterested()
	p5.uninterested()
	ChokePeers(false, peers, false)
	// Unchoked
	boolEquals(t, false, p1.IsChoked())
	boolEquals(t, false, p4.IsChoked())
	boolEquals(t, false, p5.IsChoked())
	boolEquals(t, false, p6.IsChoked())
	boolEquals(t, false, p7.IsChoked())
	boolEquals(t, false, p8.IsChoked())
	boolEquals(t, false, p3.IsChoked())
	boolEquals(t, false, p2.IsChoked())

	// Alter interest & run choker
	p4.interested()
	p5.interested()
	p8.interested()
	ChokePeers(false, peers, false)
	// Choked
	boolEquals(t, true, p1.IsChoked())
	boolEquals(t, true, p4.IsChoked())
	boolEquals(t, true, p5.IsChoked())
	boolEquals(t, true, p6.IsChoked())
	// Unchoked
	boolEquals(t, false, p7.IsChoked())
	boolEquals(t, false, p8.IsChoked())
	boolEquals(t, false, p3.IsChoked())
	boolEquals(t, false, p2.IsChoked())
}

func TestChokePeersGiven8PeersWhenOptimisticAndNotSeed(t *testing.T) {

	p1 := pr().dl(1)
	p2 := pr().dl(2)
	p3 := pr().dl(3)
	p4 := pr().dl(4)
	p5 := pr().dl(5)
	p6 := pr().dl(6)
	p7 := pr().dl(7)
	p8 := pr().dl(8)
	peers := append([]*Peer(nil), p1, p2, p3, p4, p5, p6, p7, p8)
	ChokePeers(false, peers, true)

}

type TestPeer struct {
	*Peer
}

func (tp *TestPeer) interested() *TestPeer {
	tp.state.remoteInterest = true
	return tp
}

func (tp *TestPeer) uninterested() *TestPeer {
	tp.state.remoteInterest = false
	return tp
}

func (tp *TestPeer) dl(rate uint) *TestPeer {
	tp.Stats().bytesDownloadedPerUpdate = rate
	return tp
}

func (tp *TestPeer) ul(rate uint) *TestPeer {
	tp.Stats().bytesUploadedPerUpdate = rate
	return tp
}

func pr() *TestPeer {
	return nil
}
