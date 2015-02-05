package bittorrent

import "testing"

func TestPickPieces(t *testing.T) {

	// Build a piecemap (nopieces=3, noblocks=10)
	pm := NewPieceMap(3, 10*_16KB, uint64(30*_16KB))

	// Build a collection of peers in different states
	// Peers 2, 4 & 10 are eligible for piece picking
	p1 := per(1, pm).dl(1)
	p2 := per(2, pm).dl(2).with(Bitfield([]byte{0xE0}), Unchoke{})
	p3 := per(3, pm)
	p4 := per(4, pm).dl(4).with(Bitfield([]byte{0xE0}), Unchoke{})
	p5 := per(5, pm).dl(5)
	p6 := per(6, pm)
	p7 := per(7, pm).dl(7).with(Unchoke{})
	p8 := per(8, pm).dl(8)
	p9 := per(9, pm).dl(9).with(Interested{})
	p10 := per(10, pm).dl(10).with(Bitfield([]byte{0xE0}), Unchoke{})
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
		list *MessageList
	}{
		{ p1, NewMessageList(p1.Id())},
		{ p3, NewMessageList(p3.Id())},
		{ p5, NewMessageList(p5.Id())},
		{ p6, NewMessageList(p6.Id())},
		{ p7, NewMessageList(p7.Id())},
		{ p8, NewMessageList(p8.Id())},
		{ p9, NewMessageList(p9.Id())},
		{ p10, NewMessageList(p10.Id(),
			Interested{},
			Request{0, 0, _16KB},
			Request{0, 1*_16KB, _16KB},
			Request{0, 2*_16KB, _16KB},
			Request{0, 3*_16KB, _16KB},
			Request{0, 4*_16KB, _16KB},
			Request{0, 5*_16KB, _16KB},
			Request{0, 6*_16KB, _16KB},
			Request{0, 7*_16KB, _16KB},
			Request{0, 8*_16KB, _16KB},
			Request{0, 9*_16KB, _16KB},
		)},
		{ p4, NewMessageList(p4.Id(),
			Interested{},
			Request{1, 0, _16KB},
			Request{1, 1*_16KB, _16KB},
			Request{1, 2*_16KB, _16KB},
			Request{1, 3*_16KB, _16KB},
			Request{1, 4*_16KB, _16KB},
			Request{1, 5*_16KB, _16KB},
			Request{1, 6*_16KB, _16KB},
			Request{1, 7*_16KB, _16KB},
			Request{1, 8*_16KB, _16KB},
			Request{1, 9*_16KB, _16KB},
		)},
		{ p2, NewMessageList(p2.Id(),
			Interested{},
			Request{2, 0, _16KB},
			Request{2, 1*_16KB, _16KB},
			Request{2, 2*_16KB, _16KB},
			Request{2, 3*_16KB, _16KB},
			Request{2, 4*_16KB, _16KB},
			Request{2, 5*_16KB, _16KB},
			Request{2, 6*_16KB, _16KB},
			Request{2, 7*_16KB, _16KB},
			Request{2, 8*_16KB, _16KB},
			Request{2, 9*_16KB, _16KB},
		)},
	}

	// Check outgoing message of all peers
	for _, tt := range testdata {
		for _, msg := range tt.list.msgs {
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

