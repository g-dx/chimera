package bittorrent

import (
    "testing"
    "reflect"
)

func TestOnTickWithChoke(t *testing.T) {

    mp := NewPieceMap(8, _16KB, uint64(_16KB*8))
    p1 := p1(10, ws.Interested())
    p2 := p2(20, ws.Interested())

    // Expected data
    wanted := map[PeerIdentity]ProtocolMessages {
        p1.Id() : ProtocolMessages { Unchoke{} },
        p2.Id() : ProtocolMessages { Unchoke{} },
    }

    got := make(map[PeerIdentity]ProtocolMessages)
    recv := func(p *Peer, msgs []ProtocolMessage) {
        got[p.Id()] = ProtocolMessages(msgs)
    }

    onTick(10, []*Peer{p1, p2}, recv, false, mp)

    for id, msgs := range got {
        if !reflect.DeepEqual(msgs, wanted[id]) {
            t.Errorf("\nPeer: %v, \nWanted: %v\nGot   : %v", id, wanted[id], msgs)
        }
    }
}


func TestOnTickWithPick(t *testing.T) {

    mp := NewPieceMap(8, _16KB, uint64(_16KB*8))
    p1 := withMsgs(p1(10, ws.NotChoked()), mp, Bitfield([]byte{0xF0}))
    p2 := withMsgs(p2(20, ws.NotChoked()), mp, Bitfield([]byte{0x0F}))

    // Expected data
    wanted := map[PeerIdentity]ProtocolMessages {
        p1.Id() : ProtocolMessages {
            Request{0, 0, _16KB}, Request{1, 0, _16KB}, Request{2, 0, _16KB}, Request{3, 0, _16KB} },
        p2.Id() : ProtocolMessages {
            Request{4, 0, _16KB}, Request{5, 0, _16KB}, Request{6, 0, _16KB}, Request{7, 0, _16KB}},
    }

    got := make(map[PeerIdentity]ProtocolMessages)
    recv := func(p *Peer, msgs []ProtocolMessage) {
        got[p.Id()] = ProtocolMessages(msgs)
    }

    onTick(10, []*Peer{p1, p2}, recv, false, mp)

    for id, msgs := range got {
        if !reflect.DeepEqual(msgs, wanted[id]) {
            t.Errorf("\nPeer: %v, \nWanted: %v\nGot   : %v", id, wanted[id], msgs)
        }
    }
}