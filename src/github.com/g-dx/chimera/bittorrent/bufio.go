package bittorrent

import (
	"sync"
)

////////////////////////////////////////////////////////////////////////////////////////////////

type Protocol2Message interface {
	Id() byte
	Len() uint32
	PeerId() *PeerIdentity
	Recycle()
}

type basicMessage struct {
	id  byte
	len uint32
	peerId *PeerIdentity
	pool sync.Pool
}

func (m *basicMessage) Id() byte {
	return m.id
}

func (m *basicMessage) Len() uint32 {
	return m.len
}

func (m *basicMessage) PeerId() *PeerIdentity {
	return m.peerId
}

func (m *basicMessage) Recycle() {
	if m.pool != nil {
		m.peerId = nil
		m.pool.Put(m)
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Choke        <len=0001><id=0>
// Unchoke      <len=0001><id=1>
// Interested   <len=0001><id=2>
// Uninterested <len=0001><id=3>
////////////////////////////////////////////////////////////////////////////////////////////////

type KeepAliveMessage2 basicMessage
type ChokeMessage2 basicMessage
type UnChokeMessage2 basicMessage
type InterestedMessage2 basicMessage
type UnInterestedMessage2 basicMessage

const (
	keepAliveLen = 4
)

var KeepAlive2 = &KeepAliveMessage2{id: 0, len: keepAliveLen, pool: nil}
var Choke2 = &ChokeMessage2{id: chokeId, len: chokeLength, pool: nil}
var UnChoke2 = &UnChokeMessage2{id: unchokeId, len: unchokeLength, pool: nil}
var Interested2 = &InterestedMessage2{id: chokeId, len: chokeLength, pool: nil}
var UnInterested2 = &UnInterestedMessage2{id: unchokeId, len: unchokeLength, pool: nil}

////////////////////////////////////////////////////////////////////////////////////////////////
// Handshake: <pstrlen><pstr><reserved><info_hash><peer_id>
////////////////////////////////////////////////////////////////////////////////////////////////

type HandshakeMessage2 struct {
	protocol string
	reserved [8]byte
	infoHash []byte
	peerId   string
}

// Incoming handshake
func handshake2(infoHash []byte, peerId []byte) *HandshakeMessage2 {
	return &HandshakeMessage2{protocolName,
		[8]byte{},
		infoHash,
		string(peerId),
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Have <len=0005><id=4><piece index>
////////////////////////////////////////////////////////////////////////////////////////////////

type Have2Message struct {
	basicMessage
	index uint32
}

func (m *Have2Message) Index() uint32 {
	return m.index
}

func Have2(peerId *PeerIdentity, index uint32) *Have2Message {
	have := havePool.Get().(*Have2Message)
	have.peerId = peerId
	have.index = index
	return have
}

////////////////////////////////////////////////////////////////////////////////////////////////

type blockDetailsMessage struct {
	basicMessage
	index, begin, length uint32
}

func (m blockDetailsMessage) Index() uint32 {
	return m.index
}

func (m blockDetailsMessage) Begin() uint32 {
	return m.begin
}

func (m blockDetailsMessage) Length() uint32 {
	return m.length
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Request <len=0013><id=6><index><begin><length>
// Cancel  <len=0013><id=8><index><begin><length>
////////////////////////////////////////////////////////////////////////////////////////////////
type Request2Message blockDetailsMessage
type Cancel2Message blockDetailsMessage

func Request2(peerId *PeerIdentity, index, begin, length uint32) *Request2Message {
	req := requestPool.Get().(*Request2Message)
	req.peerId = peerId
	req.index = index
	req.begin = begin
	req.length = length
	return req
}
