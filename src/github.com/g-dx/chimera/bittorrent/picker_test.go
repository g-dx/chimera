package bittorrent

import (
    "testing"
    "reflect"
)


func TestPick(t *testing.T) {
	// None taken, piece blocks all free, request all
	taken := make(set)
	n := 5
	p := NewPiece(0, n* _16KB)
	reqs, wanted := pickBlocks(taken, p, n, p.len)
	if !wanted {
		t.Errorf("Expected: peer done")
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
	p1 := p1(1, ws)
	p2 := withMsgs(p2(2, ws), pm, Bitfield([]byte{0xE0}), Unchoke{})
	p3 := p3(3, ws)
	p4 := withMsgs(p4(4, ws), pm, Bitfield([]byte{0xE0}), Unchoke{})
	p5 := p5(5, ws)
	p6 := p6(6, ws)
	p7 := withMsgs(p7(7, ws), pm, Unchoke{})
	p8 := p8(8, ws)
	p9 := p9(9, ws.Interested())
	p10 := withMsgs(p10(10, ws), pm, Bitfield([]byte{0xE0}), Unchoke{})

	// Pick pieces
	picked := PickPieces([]*Peer { p1, p2, p3, p4, p5, p6, p7, p8, p9, p10 }, pm)

	testData := map[*Peer][]ProtocolMessage {
		 p1: []ProtocolMessage{},
		 p3: []ProtocolMessage{},
		 p5: []ProtocolMessage{},
		 p6: []ProtocolMessage{},
		 p7: []ProtocolMessage{},
		 p8: []ProtocolMessage{},
		 p9: []ProtocolMessage{},
		 p10: []ProtocolMessage{
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

	for p, got := range picked {
        wanted := testData[p]
        if !reflect.DeepEqual(got, wanted) {
            t.Errorf("\nPeer: %v, \nWanted: %v\nGot   : %v", p.Id(), wanted, got)
        }
    }
}
