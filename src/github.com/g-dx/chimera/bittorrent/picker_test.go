package bittorrent

import "testing"


func TestPick(t *testing.T) {
	// None taken, piece blocks all free, request all
	taken := make(set)
	n := 5
	p := NewPiece(0, n* _16KB)
	reqs, wanted, pieceDone := pick(taken, p, n, p.len)
	if !wanted {
		t.Errorf("Expected: peer done")
	}
	if !pieceDone{
		t.Errorf("Expected: piece done")
	}
	if len(reqs) != n {
		t.Errorf("Expected: len(reqs)=%v, Actual: %v", n, len(reqs))
	}
	msgs := []ProtocolMessage{Request{0, 0, _16KB},
		           Request{0, 1*_16KB, _16KB},
				   Request{0, 2*_16KB, _16KB},
		           Request{0, 3*_16KB, _16KB},
				   Request{0, 4*_16KB, _16KB}}
	for i, msg := range reqs {
		actual := ToString(msg)
		expected := ToString(msgs[i])
		if actual != expected {
			t.Errorf("Expected: %v\nActual  : %v", expected, actual)
		}
	}
}

func TestPickPieces(t *testing.T) {

	// Build a piecemap (nopieces=3, noblocks=10)
	pm := NewPieceMap(3, 10*_16KB, uint64(30*_16KB))

	// Build a collection of peers in different states
	// Peers 2, 4 & 10 are eligible for piece picking
	p1 := per(1, pm).dl(1)
	p2 := per(2, pm).dl(2).with(pm, Bitfield([]byte{0xE0}), Unchoke{})
	p3 := per(3, pm)
	p4 := per(4, pm).dl(4).with(pm, Bitfield([]byte{0xE0}), Unchoke{})
	p5 := per(5, pm).dl(5)
	p6 := per(6, pm)
	p7 := per(7, pm).dl(7).with(pm, Unchoke{})
	p8 := per(8, pm).dl(8)
	p9 := per(9, pm).dl(9).with(pm, Interested{})
	p10 := per(10, pm).dl(10).with(pm, Bitfield([]byte{0xE0}), Unchoke{})
	peers := asList(p1, p2, p3, p4, p5, p6, p7, p8, p9, p10)
	defer teardown(peers)

	// Build a request timer
	pt := &ProtocolRequestTimer{timers : make(map[PeerIdentity]*RequestTimer),
		                        onReturn : func(int, int) {}}
	for _, p := range peers {
		pt.CreateTimer(*p.Id())
	}

	// Pick pieces
	picked := PickPieces(peers, pm, pt)

	testData := map[*TestPeer][]ProtocolMessage {
		 p1: []ProtocolMessage{},
		 p3: []ProtocolMessage{},
		 p5: []ProtocolMessage{},
		 p6: []ProtocolMessage{},
		 p7: []ProtocolMessage{},
		 p8: []ProtocolMessage{},
		 p9: []ProtocolMessage{},
		 p10: []ProtocolMessage{
			 Interested{},
			 Request{0, 0, _16KB},
			 Request{0, 1 * _16KB, _16KB},
			 Request{0, 2 * _16KB, _16KB},
			 Request{0, 3 * _16KB, _16KB},
			 Request{0, 4 * _16KB, _16KB},
			 Request{0, 5 * _16KB, _16KB},
			 Request{0, 6 * _16KB, _16KB},
			 Request{0, 7 * _16KB, _16KB},
			 Request{0, 8 * _16KB, _16KB},
			 Request{0, 9 * _16KB, _16KB},
		 },
		p4: []ProtocolMessage{
			Interested{},
			Request{1, 0, _16KB},
			Request{1, 1 * _16KB, _16KB},
			Request{1, 2 * _16KB, _16KB},
			Request{1, 3 * _16KB, _16KB},
			Request{1, 4 * _16KB, _16KB},
			Request{1, 5 * _16KB, _16KB},
			Request{1, 6 * _16KB, _16KB},
			Request{1, 7 * _16KB, _16KB},
			Request{1, 8 * _16KB, _16KB},
			Request{1, 9 * _16KB, _16KB},
		},
		p2: []ProtocolMessage{
			Interested{},
			Request{2, 0, _16KB},
			Request{2, 1 * _16KB, _16KB},
			Request{2, 2 * _16KB, _16KB},
			Request{2, 3 * _16KB, _16KB},
			Request{2, 4 * _16KB, _16KB},
			Request{2, 5 * _16KB, _16KB},
			Request{2, 6 * _16KB, _16KB},
			Request{2, 7 * _16KB, _16KB},
			Request{2, 8 * _16KB, _16KB},
			Request{2, 9 * _16KB, _16KB},
		},
	}

	t.Logf("%v", picked)

	for p, actualMsgs := range picked {
		expectedMsgs := getExpected(p, testData)
			for _, expectedMsg := range expectedMsgs {
				for _, actualMsg := range actualMsgs {
					actual := ToString(actualMsg)
					expected := ToString(expectedMsg)
					if actual != expected {
						t.Errorf("\nPeer: %v\nExpected: %v\nActual  : %v",
							p.Id(), expected, actual)
					}
				}
			}
	}
}

func getExpected(p *Peer, testData map[*TestPeer][]ProtocolMessage) []ProtocolMessage {
	for key, value := range testData {
		if key.Peer.Id().Equals(p.Id()) {
			return value
		}
	}
	return nil
}
