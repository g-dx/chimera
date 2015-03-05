package bittorrent

import (
    "testing"
    "reflect"
    "time"
    "io/ioutil"
    "log"
)

var devNull = log.New(ioutil.Discard, "", log.LstdFlags)

func TestOnTickWithChoke(t *testing.T) {

    mp := NewPieceMap(8, _16KB, uint64(_16KB*8))
    p1 := p1(10, ws.Interested())
    p2 := p2(20, ws.Interested())

    // Expected data
    wanted := map[PeerIdentity]ProtocolMessages {
        p1.Id() : ProtocolMessages { Unchoke{} },
        p2.Id() : ProtocolMessages { Unchoke{} },
    }

    recv, got := testSendAndReceiver()

    onTick(10, []*Peer{p1, p2}, recv, false, mp)

    assertReceivedMessages(t, got, wanted)
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

    recv, got := testSendAndReceiver()

    onTick(10, []*Peer{p1, p2}, recv, false, mp)

    assertReceivedMessages(t, got, wanted)
}

func TestOnPieceOk(t *testing.T) {

    // Mark all but first piece complete
    mp := NewPieceMap(8, _16KB, uint64(_16KB*8))
    for i := 1; i < len(mp.pieces); i++ {
        mp.Piece(i).Complete()
    }

    p1 := p1(10, ws)
    p2 := p2(20, ws)
    c := make(chan struct{})

    wanted := map[PeerIdentity]ProtocolMessages {
        p1.Id() : ProtocolMessages { Have(0) },
        p2.Id() : ProtocolMessages { Have(0) },
    }

    recv, got := testBroadcastAndReceiver()

    onPieceOk(0, mp, recv, c, devNull)

    // Check for haves
    assertReceivedMessages(t, got, wanted)

    // Check marked as complete
    if !mp.IsComplete() {
        t.Errorf("PieceMap not complete!")
    }
    select {
        case <- time.Tick(oneSecond):
            t.Errorf("Download not complete!")
        case <- c:
    }
}

func TestOnReadOk(t *testing.T) {

    p1 := p1(10, ws)
    recv, got := testSendAndReceiver()

    onReadOk(Block{0, 0, []byte{1, 2, 3, 4}}, p1, recv)

    assertReceivedMessages(t, got, map[PeerIdentity]ProtocolMessages {
        p1.Id() : ProtocolMessages { Block{0, 0, []byte{1, 2, 3, 4}} },
    })
}

func testSendAndReceiver() (func(p *Peer, msgs []ProtocolMessage), map[PeerIdentity]ProtocolMessages) {
    got := make(map[PeerIdentity]ProtocolMessages)
    f := func(p *Peer, msgs []ProtocolMessage) {
        got[p.Id()] = ProtocolMessages(msgs)
    }
    return f, got
}

func testBroadcastAndReceiver() (func(msg ProtocolMessage), ProtocolMessages) {
    var msgs ProtocolMessages
    f := func(msg ProtocolMessage) {
        msgs = append(msgs, msg)
    }
    return f, msgs
}


func assertReceivedMessages(t *testing.T, got map[PeerIdentity]ProtocolMessages, wanted map[PeerIdentity]ProtocolMessages) {
    for id, msgs := range got {
        if !reflect.DeepEqual(wanted[id], msgs) {
            t.Errorf("\nPeer: %v, \nWanted: %v\nGot   : %v", id, wanted[id], msgs)
        }
    }
}