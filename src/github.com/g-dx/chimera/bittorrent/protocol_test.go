package bittorrent
import "testing"

func TestOnTick(t *testing.T) {

    ws := initWireState

    tick := 10 // Choke & Pick
    p1 := p1(10, ws.Interested())
    p2 := p2(20, ws.Interested())
    buffers := map[PeerIdentity]chan BufferMessage {
        p1.Id() : make(chan BufferMessage, 11),
        p2.Id() : make(chan BufferMessage, 11),
    }

    isSeed := false
    mp := NewPieceMap(8, _16KB, uint64(_16KB*8))

    // Update peers to be seeds
    _, msgs, _ := OnReceiveMessages([]ProtocolMessage{ Bitfield([]byte{0xFF}) }, p1, mp)
    for _, msg := range msgs {
        t.Logf("p1: %v", ToString(msg))
    }
    _, msgs, _ = OnReceiveMessages([]ProtocolMessage{ Bitfield([]byte{0xFF}) }, p2, mp)
    for _, msg := range msgs {
        t.Logf("p2: %v", ToString(msg))
    }

    // Tick
    OnTick(tick, []*Peer{p1, p2}, buffers, isSeed, mp)

    // Check for unchokes

    drain(buffers[p1.Id()], t)
    drain(buffers[p2.Id()], t)

}

func drain(c chan BufferMessage, t *testing.T) {
    for {
        select {
        case msg := <-c:
            t.Logf("%v", msg)
        default:
            return
        }
    }
}