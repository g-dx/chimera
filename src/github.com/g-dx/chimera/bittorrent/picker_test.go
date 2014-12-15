package bittorrent

import "testing"

func TestPickPieces(t *testing.T) {

	// Build a piecemap (nopieces=3, noblocks=10)
	pm := NewPieceMap(3, 10*_16KB, uint64(30*_16KB))

	// Build a collection of peers in different states
	// Peers 2, 4 & 10 are eligible for piece picking
	p1 := per(1, pm).dl(1)
	p2 := per(2, pm).dl(2).with(Bitfield(nil, []byte{0xE0}), Unchoke(nil))
	p3 := per(3, pm)
	p4 := per(4, pm).dl(4).with(Bitfield(nil, []byte{0xE0}), Unchoke(nil))
	p5 := per(5, pm).dl(5)
	p6 := per(6, pm)
	p7 := per(7, pm).dl(7).with(Unchoke(nil))
	p8 := per(8, pm).dl(8)
	p9 := per(9, pm).dl(9).with(Interested(nil))
	p10 := per(10, pm).dl(10).with(Bitfield(nil, []byte{0xE0}), Unchoke(nil))
	peers := asList(p1, p2, p3, p4, p5, p6, p7, p8, p9, p10)
	defer teardown(peers)

	// Build a request timer
	pt := &ProtocolRequestTimer{timers : make(map[PeerIdentity]*RequestTimer),
		                        onReturn : func(int, int) {}}
	for _, p := range peers {
		pt.CreateTimer(*p.Id())
	}

	PickPieces(peers, pm, pt)

	// Build expected output
	var testdata = []struct {
		peer *TestPeer
		msgs MessageList
	}{
		{ p1, asMessageList()},
		{ p3, asMessageList()},
		{ p5, asMessageList()},
		{ p6, asMessageList()},
		{ p7, asMessageList()},
		{ p8, asMessageList()},
		{ p9, asMessageList()},
		{ p10, asMessageList(
			Interested(p10.Id()),
			Request(p10.Id(), 0, 0, _16KB),
			Request(p10.Id(), 0, 1*_16KB, _16KB),
			Request(p10.Id(), 0, 2*_16KB, _16KB),
			Request(p10.Id(), 0, 3*_16KB, _16KB),
			Request(p10.Id(), 0, 4*_16KB, _16KB),
			Request(p10.Id(), 0, 5*_16KB, _16KB),
			Request(p10.Id(), 0, 6*_16KB, _16KB),
			Request(p10.Id(), 0, 7*_16KB, _16KB),
			Request(p10.Id(), 0, 8*_16KB, _16KB),
			Request(p10.Id(), 0, 9*_16KB, _16KB),
		)},
		{ p4, asMessageList(
			Interested(p4.Id()),
			Request(p4.Id(), 1, 0, _16KB),
			Request(p4.Id(), 1, 1*_16KB, _16KB),
			Request(p4.Id(), 1, 2*_16KB, _16KB),
			Request(p4.Id(), 1, 3*_16KB, _16KB),
			Request(p4.Id(), 1, 4*_16KB, _16KB),
			Request(p4.Id(), 1, 5*_16KB, _16KB),
			Request(p4.Id(), 1, 6*_16KB, _16KB),
			Request(p4.Id(), 1, 7*_16KB, _16KB),
			Request(p4.Id(), 1, 8*_16KB, _16KB),
			Request(p4.Id(), 1, 9*_16KB, _16KB),
		)},
		{ p2, asMessageList(
			Interested(p2.Id()),
			Request(p2.Id(), 2, 0, _16KB),
			Request(p2.Id(), 2, 1*_16KB, _16KB),
			Request(p2.Id(), 2, 2*_16KB, _16KB),
			Request(p2.Id(), 2, 3*_16KB, _16KB),
			Request(p2.Id(), 2, 4*_16KB, _16KB),
			Request(p2.Id(), 2, 5*_16KB, _16KB),
			Request(p2.Id(), 2, 6*_16KB, _16KB),
			Request(p2.Id(), 2, 7*_16KB, _16KB),
			Request(p2.Id(), 2, 8*_16KB, _16KB),
			Request(p2.Id(), 2, 9*_16KB, _16KB),
		)},
	}

	// Check outgoing message of all peers
	for _, tt := range testdata {
		for _, msg := range tt.msgs.allMsgs {
			// TODO: Should implement an eqivalence() method
			expected := ToString(msg)
			actual := ToString(<- tt.peer.out)
			if actual != expected {
				t.Errorf("\nPeer: %v\nExpected: %v\nActual  : %v",
					tt.peer.Id(), expected, actual)
			}
		}
	}
}

