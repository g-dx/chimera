package bittorrent

import (
	"fmt"
	"log"
	"strconv"
	"testing"
	"os"
)

func BenchmarkChokePeers10(b *testing.B) {

	// Build a collection of peers in different states
	p1 := p1().dl(1)
	p2 := p2().dl(2).interested()
	p3 := p3()
	p4 := p4().dl(4).interested()
	p5 := p5().dl(5)
	p6 := p6()
	p7 := p7().dl(7).interested()
	p8 := p8().dl(8)
	p9 := p9().dl(9).interested()
	p10 := p10().dl(10).interested()

	// Unchoke a few
	p1.UnChoke(true)
	p6.UnChoke(false)
	p8.UnChoke(false)

	peers := asList(p1, p2, p3, p4, p5, p6, p7, p8, p9, p10)
	defer teardown(peers)
	for i := 0; i < b.N; i++ {
		ChokePeers(false, peers, true)
	}
}

func TestBuildCandidatesGivenNewPeers(t *testing.T) {

	p1 := p1()
	p2 := p2()
	peers := asList(p1, p2)
	defer teardown(peers)

	c, cur := buildCandidates(peers)
	intEquals(t, 6, len(c))
	// TODO: Check cur == nil

	// Set optimistic
	p1.UnChoke(true)

	c, cur = buildCandidates(peers)
	intEquals(t, 3, len(c))
	stringEquals(t, p1.Id().String(), cur.Id().String())
	stringEquals(t, p2.Id().String(), c[0].Id().String())
	stringEquals(t, p2.Id().String(), c[1].Id().String())
	stringEquals(t, p2.Id().String(), c[2].Id().String())

	// Clear
	// TODO: This isn't great
	p1.Choke()
	p1.ClearOptimistic()
	c, cur = buildCandidates(peers)
	intEquals(t, 4, len(c))
	// TODO: Check cur == nil
}

func TestChokePeersGivenNoPeers(t *testing.T) {
	peers := asList()
	ChokePeers(false, peers, false)
	intEquals(t, 0, len(peers))
}

func TestChokePeersGivenNoOptimisticCandidatesAndExistingOptimistic(t *testing.T) {

	p1 := p1()
	p2 := p2()
	peers := asList(p1, p2)
	defer teardown(peers)

	// Set optimistic and unchoked
	p1.UnChoke(true)
	p2.UnChoke(false)

	// Run choker & check optimistic has *not* changed
	ChokePeers(false, peers, true)
	assertUnchoked(t, p1)
	assertChoked(t, p2)
	boolEquals(t, true, p1.IsOptimistic())
	boolEquals(t, false, p2.IsOptimistic())
}

func TestChokePeersGivenNoOptimisticCandidatesAndNoOptimistic(t *testing.T) {

	p1 := p1()
	p2 := p2()
	peers := asList(p1, p2)
	defer teardown(peers)

	// Set both unchoke
	p1.UnChoke(false)
	p2.UnChoke(false)

	// Run choker & check no optimistic
	ChokePeers(false, peers, true)
	assertChoked(t, p1, p2)
	boolEquals(t, false, p1.IsOptimistic())
	boolEquals(t, false, p2.IsOptimistic())
}

func TestChokePeersGivenNoInterestedPeers(t *testing.T) {

	p1 := p1()
	p2 := p2()
	p3 := p3()
	peers := asList(p1, p2, p3)
	defer teardown(peers)

	// Run choker
	ChokePeers(false, peers, false)
	assertChoked(t, p1, p2, p3)
}

func TestChokePeersGivenSameSpeedPeersWhenInterestChanges(t *testing.T) {

	p1 := p1().dl(1).interested()
	p2 := p2().dl(1)
	p3 := p3().dl(1)
	peers := asList(p1, p2, p3)
	defer teardown(peers)

	// Run choker
	ChokePeers(false, peers, false)
	assertChoked(t, p2, p3)
	assertUnchoked(t, p1)

	// Alter interest and run choker
	p1.uninterested()
	p3.interested()
	ChokePeers(false, peers, false)
	assertChoked(t, p1, p2)
	assertUnchoked(t, p3)

	// Alter interest and run choker
	p2.interested()
	p3.uninterested()
	ChokePeers(false, peers, false)
	assertChoked(t, p1, p3)
	assertUnchoked(t, p2)
}

func TestChokePeersGivenDifferentSpeedsWhenInterestChanges(t *testing.T) {

	p1 := p1().dl(1)
	p2 := p2().dl(2).interested()
	p3 := p3().dl(3)
	p4 := p4().dl(4).interested()
	p5 := p5().dl(5)
	p6 := p6().dl(6)
	p7 := p7().dl(7).interested()
	p8 := p8().dl(8)
	peers := asList(p1, p2, p3, p4, p5, p6, p7, p8)
	defer teardown(peers)

	// Run choker
	ChokePeers(false, peers, false)
	assertChoked(t, p1)
	assertUnchoked(t, p2, p3, p4, p5, p6, p7, p8)

	// Alter interest & run choker
	p2.uninterested()
	ChokePeers(false, peers, false)
	assertChoked(t, p1, p2, p3)
	assertUnchoked(t, p4, p5, p6, p7, p8)

	// Alter interest & run choker
	p4.uninterested()
	ChokePeers(false, peers, false)
	assertChoked(t, p1, p2, p3, p4, p5, p6)
	assertUnchoked(t, p7, p8)

	// Alter interest & run choker
	p7.uninterested()
	ChokePeers(false, peers, false)
	assertChoked(t, p1, p2, p3, p4, p5, p6, p7, p8)
}

func TestChokePeersGivenDifferentSpeedsWhenSpeedChanges(t *testing.T) {

	p1 := p1().dl(1)
	p2 := p2().dl(2)
	p3 := p3().dl(3)
	p4 := p4().dl(4)
	p5 := p5().dl(5).interested()
	p6 := p6().dl(6).interested()
	p7 := p7().dl(7).interested()
	p8 := p8().dl(8)
	peers := asList(p1, p2, p3, p4, p5, p6, p7, p8)
	defer teardown(peers)

	ChokePeers(false, peers, false)
	assertChoked(t, p1, p2, p3, p4)
	assertUnchoked(t, p5, p6, p7, p8)

	// Alter speed & run choker
	p5.dl(9)
	p6.dl(10)
	p7.dl(11)
	ChokePeers(false, peers, false)
	assertChoked(t, p1, p2, p3, p4, p8)
	assertUnchoked(t, p5, p6, p7)
}

func assertChoked(t *testing.T, peers ...*TestPeer) {
	assertChokeStatus(t, true, peers)
}

func assertUnchoked(t *testing.T, peers ...*TestPeer) {
	assertChokeStatus(t, false, peers)
}

func assertChokeStatus(t *testing.T, b bool, peers []*TestPeer) {
	for _, p := range peers {
		if p.IsChoked() != b {
			expected := fmt.Sprintf("p(%v) choke=%v", p.Id(), b)
			actual := fmt.Sprintf("p(%v) choke=%v", p.Id(), p.IsChoked())
			t.Errorf(buildUnequalMessage(3, expected, actual))
		}
	}
}

type TestPeer struct {
	*Peer
	out chan ProtocolMessage
}

func (tp *TestPeer) notChoking() *TestPeer {
	tp.state.localChoke = false
	return tp
}

func (tp *TestPeer) choking() *TestPeer {
	tp.state.localChoke = true
	return tp
}

func (tp *TestPeer) interesting() *TestPeer {
	tp.state.localInterest = true
	return tp
}

func (tp *TestPeer) notInteresting() *TestPeer {
	tp.state.localInterest = true
	return tp
}

func (tp *TestPeer) interested() *TestPeer {
	tp.state.remoteInterest = true
	tp.state.new = false
	return tp
}

func (tp *TestPeer) uninterested() *TestPeer {
	tp.state.remoteInterest = false
	return tp
}

func (tp *TestPeer) with(msgs ...ProtocolMessage) *TestPeer {
	for _, msg := range msgs {
		err := tp.OnMessage(msg)
		if err != nil {
			panic(err)
		}
	}
	return tp
}

func (tp *TestPeer) dl(rate int) *TestPeer {
	tp.Stats().Download.rate = rate
	tp.state.new = false
	return tp
}

func (tp *TestPeer) ul(rate int) *TestPeer {
	tp.Stats().Upload.rate = rate
	tp.state.new = false
	return tp
}

func (tp *TestPeer) asPeer() *Peer {
	return tp.Peer
}

func pr(i int) *TestPeer {
	return per(i, NewPieceMap(1, 1, 1))
}

func per(i int, pm *PieceMap) *TestPeer {
	out := make(chan ProtocolMessage)
	p := NewPeer(
		&PeerIdentity{[20]byte{}, strconv.Itoa(i)},
		NewQueue(out, func(int, int) {}),
		pm,
		log.New(os.Stdout, "", log.LstdFlags))
	return &TestPeer{p, out}
}

func asList(tps ...*TestPeer) []*Peer {
	ps := make([]*Peer, 0, len(tps))
	for _, p := range tps {
		ps = append(ps, p.asPeer())
	}
	return ps
}

func teardown(peers []*Peer) {
	for _, p := range peers {
		p.queue.Close()
	}
}

//----------------------------------------------------
// Already named test peers
//----------------------------------------------------

func p1() *TestPeer  { return pr(1) }
func p2() *TestPeer  { return pr(2) }
func p3() *TestPeer  { return pr(3) }
func p4() *TestPeer  { return pr(4) }
func p5() *TestPeer  { return pr(5) }
func p6() *TestPeer  { return pr(6) }
func p7() *TestPeer  { return pr(7) }
func p8() *TestPeer  { return pr(8) }
func p9() *TestPeer  { return pr(9) }
func p10() *TestPeer { return pr(10) }
