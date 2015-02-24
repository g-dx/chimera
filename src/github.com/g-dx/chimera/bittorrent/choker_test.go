package bittorrent

import (
	"fmt"
	"strconv"
	"testing"
	"strings"
)

type ids []PeerIdentity
type ps []*Peer
var none = "<none>"

func BenchmarkChokePeers10(b *testing.B) {

	// Build a collection of peers in different states
	ws := initWireState
	peers := ps{
		pr1(1, ws.NotChoking()),
		pr2(2, ws.Interested()),
		pr3(0, ws),
		pr4(4, ws.Interested()),
		pr5(5, ws),
		pr6(0, ws.NotChoking()),
		pr7(7, ws.Interested()),
		pr8(8, ws.NotChoking()),
	}

	for i := 0; i < b.N; i++ {
		ChokePeers(false, peers, true)
	}
}

func TestBuildCandidates(t *testing.T) {

	ws := initWireState
	tests := []struct {
		ps ps
		curOpt string
		candidates ids
	}{
		{ ps{ pr1(1, ws), pr2(1, ws) }, none, ids{ "p1", "p1", "p1", "p2", "p2", "p2"}},

		{ ps{ pr1(1, ws.NotNew()), pr2(1, ws) }, none, ids{ "p1", "p2", "p2", "p2"}},

		{ ps{ pr1(1, ws.NotChoking().Optimistic()), pr2(1, ws) }, "p1", ids{ "p2", "p2", "p2"}},
	}

	for i, tt := range tests {
		can, c := buildCandidates(tt.ps)

		// Check current optimistic
		var errMsgs []string
		errMsgs = checkPeer(errMsgs, "Current Optimistic", tt.curOpt, c)

		// Check candidates
		notFound, notExpected := containsPeers(tt.candidates, toPeerIdentities(can))
		errMsgs = checkPeers(errMsgs, "Optimistic Candidates", notFound, notExpected)
		if len(errMsgs) > 0 {
			t.Errorf("Run: %v\n%v", i, strings.Join(errMsgs, "\n"))
		}
	}
}

func TestChokePeers(t *testing.T) {

	// Init download & upload rate & wire state
	ws := initWireState
	tests := []struct {
		ps ps
		changeOpt bool
		oldOpt string
		newOpt string
		chokes ids
		unchokes ids
	}{

		//------------------------------------------------------------------------------------------
		// 0.
		// Check empty
		{ ps{}, false, none, none, ids{}, ids{} },

		//------------------------------------------------------------------------------------------
		// 1.
		// Check empty
		{ ps{}, true, none, none, ids{}, ids{} },

		//------------------------------------------------------------------------------------------
		// 2.
		// Check p1 unchoked + p2 as optimistic
		// NOTE: p2 is chosen as optimistic "randomly"
		{ ps{
			pr1(10, ws.Interested()),
			pr2(10, ws),
		  }, true, none, "p2", ids{}, ids{ "p1", "p2"},
		},

		//------------------------------------------------------------------------------------------
		// 3.
		// Check both choked
		{ ps{
			pr1(10, ws.NotChoking()),
			pr2(10, ws.NotChoking()),
		  }, false, none, none, ids{ "p1", "p2"}, ids{},
		},

		//------------------------------------------------------------------------------------------
		// 4.
		// Check optimistic does not change & only unchoke gets choked
		{ ps{
			pr1(10, ws.Optimistic().NotChoking()),
			pr2(10, ws.NotChoking()),
		  }, true, "p1", "p1", ids{ "p2" }, ids{},
		},

		//------------------------------------------------------------------------------------------
		// 5.
		// Check no unchokes
		{ ps{
			pr1(10, ws),
			pr2(10, ws),
			pr3(10, ws),
		  }, false, none, none, ids{}, ids{},
		},

		//------------------------------------------------------------------------------------------
		// 6.
		// Check interested gets unchoked with all faster peers & unchoked, uninterested gets choked
		{ ps{
			pr1(10, ws.NotChoking()),
			pr2(20, ws.Interested()),
			pr3(30, ws),
		  }, false, none, none, ids{ "p1"}, ids{ "p2", "p3" },
		},

		//------------------------------------------------------------------------------------------
		// 7.
		// Check uninterested unchokes get choked
		{ ps{
			pr1(10, ws),
			pr2(20, ws.NotChoking()),
			pr3(30, ws.Interested().NotChoking()),
		  }, false, none, none, ids{ "p2" }, ids{},
		},

		//------------------------------------------------------------------------------------------
		// 8.
		// Ensure slow, uninterested, unchoke peer gets choked
		{ ps{
			pr1(30, ws.Interested().NotChoking()),
			pr2(20, ws.Interested().NotChoking()),
			pr3(10, ws.NotChoking()),
		  }, false, none, none, ids{ "p3" }, ids{},
		},

		//------------------------------------------------------------------------------------------
		// 9.
		// Check at most 4 interested unchokes even when more interested & faster uninterested
		{ ps{
			pr1(10, ws.Interested()),
			pr2(20, ws.Interested()),
			pr3(30, ws.Interested()),
			pr4(40, ws.Interested()),
			pr5(50, ws.Interested()),
			pr6(60, ws.Interested()),
			pr7(70, ws),
		  }, false, none, none, ids{}, ids{ "p3", "p4", "p5", "p6", "p7" },
		},

		//------------------------------------------------------------------------------------------
		// 10.
		// Check slower interested unchoked peers get choked
		{ ps{
			pr1(10, ws.Interested().NotChoking()),
			pr2(20, ws.Interested().NotChoking()),
			pr3(30, ws.Interested().NotChoking()),
			pr4(40, ws.Interested().NotChoking()),
			pr5(50, ws.Interested().NotChoking()),
			pr6(60, ws.Interested().NotChoking()),
			pr7(70, ws.NotChoking()),
          }, false, none, none, ids{ "p1", "p2"}, ids{},
		},
	}

	for i, tt := range tests {
		// Perform choking
		old, new, chokes, unchokes := ChokePeers(false, tt.ps, tt.changeOpt)

		// Check old optimistic
		var errMsgs []string
		errMsgs = checkPeer(errMsgs, "Old Opt ", tt.oldOpt, old)

		// Check new optimistic
		errMsgs = checkPeer(errMsgs, "New Opt ", tt.newOpt, new)

		// Check all chokes present
		notFound, notExpected := containsPeers(tt.chokes, toPeerIdentities(chokes))
		errMsgs = checkPeers(errMsgs, "Chokes  ", notFound, notExpected)

		// Check all unchokes present
		notFound, notExpected = containsPeers(tt.unchokes, toPeerIdentities(unchokes))
		errMsgs = checkPeers(errMsgs, "Unchokes", notFound, notExpected)

		// Print error
		if len(errMsgs) > 0 {
			t.Errorf("Run: %v\n%v", i, strings.Join(errMsgs, "\n"))
		}
	}
}

func checkPeer(errMsgs []string, op string, expected string, actual *Peer) []string {
	if actual != nil && string(actual.Id()) != expected {
		errMsgs = append(errMsgs, fmt.Sprintf("%v: Expected: %v, Actual: %v", op, expected, actual.Id()))
	}
	if actual == nil && expected != none {
		errMsgs = append(errMsgs, fmt.Sprintf("%v: Expected: %v, Actual: %v", op, expected, none))
	}
	return errMsgs
}

func checkPeers(errMsgs []string, op string, notFound []string, notExpected []string) []string {
	if len(notFound) > 0 {
		errMsgs = append(errMsgs, fmt.Sprintf("%v:   Expected (%v)", op, strings.Join(notFound, ",")))
	}
	if len(notExpected) > 0 {
		errMsgs = append(errMsgs, fmt.Sprintf("%v: Unexpected (%v)", op, strings.Join(notExpected, ",")))
	}
	return errMsgs
}

func toPeerIdentities(ps []*Peer) (ids []PeerIdentity) {
	for _, p := range ps {
		ids = append(ids, p.Id())
	}
	return ids
}

func containsPeerIdentity(b PeerIdentity, ids []PeerIdentity) bool {
	for _, a := range ids {
		if a == b {
			return true
		}
	}
	return false
}

func containsPeers(expected []PeerIdentity, actual []PeerIdentity) (notFound []string, notExpected []string) {

	// Check all actual present in expected
	for _, id := range actual {
		if !containsPeerIdentity(id, expected) {
			notExpected = append(notExpected, string(id))
		}
	}

	// Check all expected present in actual
	for _, id := range expected {
		if !containsPeerIdentity(id, actual) {
			notFound = append(notFound, string(id))
		}
	}
	return notFound, notExpected
}

type TestPeer struct {
	*Peer
	out chan ProtocolMessage
}

func (tp *TestPeer) notChoking() *TestPeer {
	tp.ws = tp.ws.NotChoking()
	return tp
}

func (tp *TestPeer) choking() *TestPeer {
	tp.ws = tp.ws.Choking()
	return tp
}

func (tp *TestPeer) interesting() *TestPeer {
	tp.ws = tp.ws.Interesting()
	return tp
}

func (tp *TestPeer) notInteresting() *TestPeer {
	tp.ws = tp.ws.NotInteresting()
	return tp
}

func (tp *TestPeer) interested() *TestPeer {
	tp.ws = tp.ws.Interested().NotNew()
	return tp
}

func (tp *TestPeer) uninterested() *TestPeer {
	tp.ws = tp.ws.NotInterested()
	return tp
}

func (tp *TestPeer) with(mp *PieceMap, msgs ...ProtocolMessage) *TestPeer {
	err, _, _ := OnReceiveMessages(msgs, tp.Peer, mp)
	if err != nil {
		panic(err)
	}
	return tp
}

func (tp *TestPeer) dl(rate int) *TestPeer {
	tp.Stats().Download.rate = rate
	tp.ws = tp.ws.NotNew()
	return tp
}

func (tp *TestPeer) ul(rate int) *TestPeer {
	tp.Stats().Upload.rate = rate
	tp.ws = tp.ws.NotNew()
	return tp
}

func (tp *TestPeer) asPeer() *Peer {
	return tp.Peer
}

func per(i int, pm *PieceMap) *TestPeer {
	out := make(chan ProtocolMessage)
	p := NewPeer(PeerIdentity(strconv.Itoa(i)), len(pm.pieces))
	return &TestPeer{p, out}
}

func asList(tps ...*TestPeer) []*Peer {
	ps := make([]*Peer, 0, len(tps))
	for _, p := range tps {
		ps = append(ps, p.asPeer())
	}
	return ps
}

func pr1(rate int, ws WireState) *Peer { return p("p1", rate, ws) }
func pr2(rate int, ws WireState) *Peer { return p("p2", rate, ws) }
func pr3(rate int, ws WireState) *Peer { return p("p3", rate, ws) }
func pr4(rate int, ws WireState) *Peer { return p("p4", rate, ws) }
func pr5(rate int, ws WireState) *Peer { return p("p5", rate, ws) }
func pr6(rate int, ws WireState) *Peer { return p("p6", rate, ws) }
func pr7(rate int, ws WireState) *Peer { return p("p7", rate, ws) }
func pr8(rate int, ws WireState) *Peer { return p("p8", rate, ws) }

func p(id string, rate int, ws WireState) *Peer {
	p := NewPeer(PeerIdentity(id), 0) // No of pieces not important
	p.ws = ws
	p.Stats().Download.rate = rate
	p.Stats().Upload.rate = rate
	return p
}