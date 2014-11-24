package bittorrent

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
)

const (
	// message ids (0 ... 9)
	chokeId byte = iota
	unchokeId
	interestedId
	uninterestedId
	haveId
	bitfieldId
	requestId
	blockId
	cancelId
)

const (
	// Fixed message lengths
	chokeLength        uint32 = 1
	unchokeLength      uint32 = 1
	interestedLength   uint32 = 1
	uninterestedLength uint32 = 1
	haveLength         uint32 = 5
	cancelLength       uint32 = 13
	requestLength      uint32 = 13
	handshakeLength    uint32 = 68
)

////////////////////////////////////////////////////////////////////////////////////////////////
// Message Pools
////////////////////////////////////////////////////////////////////////////////////////////////

var havePool = &sync.Pool{
	New: func() interface{} {
		return &HaveMessage{GenericMessage{haveLength, haveId, nil, havePool}, 0}
	},
}

var requestPool = &sync.Pool{
	New: func() interface{} {
		return &RequestMessage{GenericMessage{requestLength, requestId, nil, requestPool}, 0, 0, 0}
	},
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Basic message
////////////////////////////////////////////////////////////////////////////////////////////////

type ProtocolMessage interface {
	Id() byte
	Len() uint32
	PeerId() *PeerIdentity
	Recycle()
}

type GenericMessage struct {
	len    uint32
	id     byte
	peerId *PeerIdentity
	pool   *sync.Pool
}

func (m *GenericMessage) Id() byte {
	return m.id
}

func (m *GenericMessage) Len() uint32 {
	return m.len
}

func (m *GenericMessage) PeerId() *PeerIdentity {
	return m.peerId
}

func (m *GenericMessage) Recycle() {
	if m.pool != nil {
		m.peerId = nil
		m.pool.Put(m)
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Handshake: <pstrlen><pstr><reserved><info_hash><peer_id>
////////////////////////////////////////////////////////////////////////////////////////////////

const protocolName = "BitTorrent protocol"

var PeerId []byte

func init() {
	// Build unique Peer ID
	clientId := []byte("-CH0001-")
	randBytes := make([]byte, 20-len(clientId))
	rand.Read(randBytes)
	PeerId = append(clientId, randBytes...)
}

type HandshakeMessage struct {
	protocol string
	reserved [8]byte
	infoHash []byte
	peerId   string
}

// Incoming handshake
func handshake(p *PeerIdentity, infoHash []byte, peerId []byte) *HandshakeMessage {
	return &HandshakeMessage{
		protocolName,
		[8]byte{},
		infoHash,
		string(peerId),
	}
}

// Outgoing handshake
func Handshake(infoHash []byte) *HandshakeMessage {
	if len(infoHash) != 20 {
		panic(errors.New("Invalid info_hash length."))
	}

	return handshake(nil, infoHash, PeerId)
}

////////////////////////////////////////////////////////////////////////////////////////////////
// KeepAlive <len=0000>
////////////////////////////////////////////////////////////////////////////////////////////////

var KeepAliveMessage = &GenericMessage{0, 0, nil, nil}

////////////////////////////////////////////////////////////////////////////////////////////////
// Choke        <len=0001><id=0>
// Unchoke      <len=0001><id=1>
// Interested   <len=0001><id=2>
// Uninterested <len=0001><id=3>
////////////////////////////////////////////////////////////////////////////////////////////////

type ChokeMessage GenericMessage
type UnchokeMessage GenericMessage
type InterestedMessage GenericMessage
type UninterestedMessage GenericMessage

func Choke(p *PeerIdentity) *ChokeMessage {
	return &ChokeMessage{chokeLength, chokeId, p, nil}
}

func Unchoke(p *PeerIdentity) *UnchokeMessage {
	return &UnchokeMessage{unchokeLength, unchokeId, p, nil}
}

func Interested(p *PeerIdentity) *InterestedMessage {
	return &InterestedMessage{interestedLength, interestedId, p, nil}
}

func Uninterested(p *PeerIdentity) *UninterestedMessage {
	return &UninterestedMessage{uninterestedLength, uninterestedId, p, nil}
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Have <len=0005><id=4><piece index>
////////////////////////////////////////////////////////////////////////////////////////////////

type HaveMessage struct {
	GenericMessage
	index uint32
}

func (m *HaveMessage) Index() uint32 {
	return m.index
}

func Have(p *PeerIdentity, i uint32) *HaveMessage {
	have := havePool.Get().(*HaveMessage)
	have.peerId = p
	have.index = i
	return have
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Bitfield: <len=0001+X><id=5><bitfield>
////////////////////////////////////////////////////////////////////////////////////////////////

type BitfieldMessage struct {
	GenericMessage
	bits []byte
}

func (m *BitfieldMessage) Bits() []byte {
	return m.bits
}

func Bitfield(p *PeerIdentity, bits []byte) *BitfieldMessage {
	return &BitfieldMessage{GenericMessage{uint32(1 + len(bits)), bitfieldId, p, nil}, bits}
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Request <len=0013><id=6><index><begin><length>
// Cancel <len=0013><id=8><index><begin><length>
////////////////////////////////////////////////////////////////////////////////////////////////

type BlockDetailsMessage struct {
	GenericMessage
	index, begin, length uint32
}

func (m *BlockDetailsMessage) Index() uint32 {
	return m.index
}

func (m *BlockDetailsMessage) Begin() uint32 {
	return m.begin
}

func (m *BlockDetailsMessage) Length() uint32 {
	return m.length
}

type CancelMessage BlockDetailsMessage
type RequestMessage BlockDetailsMessage

func Request(p *PeerIdentity, index, begin, length uint32) *RequestMessage {
	req := requestPool.Get().(*RequestMessage)
	req.index = index
	req.begin = begin
	req.length = length
	req.peerId = p
	return req
}

func Cancel(p *PeerIdentity, index, begin, length uint32) *CancelMessage {
	return &CancelMessage{
		GenericMessage{cancelLength, cancelId, p, nil},
		index,
		begin,
		length,
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Piece <len=0009+X><id=7><index><begin><block>
////////////////////////////////////////////////////////////////////////////////////////////////

type BlockMessage struct {
	GenericMessage
	index, begin uint32
	block        []byte
}

func (m BlockMessage) Index() uint32 {
	return m.index
}

func (m BlockMessage) Begin() uint32 {
	return m.begin
}

func (m BlockMessage) Block() []byte {
	return m.block
}

func Block(p *PeerIdentity, index, begin uint32, block []byte) *BlockMessage {
	return &BlockMessage{
		GenericMessage{uint32(9 + len(block)), blockId, p, nil},
		index,
		begin,
		block,
	}
}

func ReadHandshake(buf []byte, id *PeerIdentity) ([]byte, ProtocolMessage) {

	// Do we have enough data for handshake?
	if len(buf) < int(handshakeLength) {
		return nil, nil
	}

	// Calculate handshake data & any remaining
	data := buf[0:handshakeLength]
	remainingBuf := buf[handshakeLength:]

	// TODO: Assert the protocol & reserved bytes?

	return remainingBuf, handshake(id, data[28:48], data[48:handshakeLength])
}

func Marshal(pm ProtocolMessage) []byte {

	// TODO: This isn't great
	if pm == KeepAliveMessage {
		return make([]byte, 4)
	}

	// Encode struct
	w := bytes.NewBuffer(make([]byte, 0, pm.Len()+4))
	switch msg := pm.(type) {
	case *BlockMessage:
		marshal(w, binary.BigEndian, msg.len)
		marshal(w, binary.BigEndian, msg.id)
		marshal(w, binary.BigEndian, msg.index)
		marshal(w, binary.BigEndian, msg.begin)
		marshal(w, binary.BigEndian, msg.block)

	case *BitfieldMessage:
		marshal(w, binary.BigEndian, msg.len)
		marshal(w, binary.BigEndian, msg.id)
		marshal(w, binary.BigEndian, msg.bits)

	case *HandshakeMessage:
		marshal(w, binary.BigEndian, uint8(len(msg.protocol)))
		marshal(w, binary.BigEndian, []byte(msg.protocol))
		marshal(w, binary.BigEndian, msg.reserved)
		marshal(w, binary.BigEndian, msg.infoHash)
		marshal(w, binary.BigEndian, []byte(PeerId))

	default:
		marshal(w, binary.BigEndian, pm)
	}

	return w.Bytes()
}

func Unmarshal(p *PeerIdentity, buf []byte) ([]byte, ProtocolMessage) {

	// Do we have enough to calculate the length?
	if len(buf) < 4 {
		return buf, nil
	}

	// Check: Keepalive
	msgLen := toUint32(buf[0:4])
	remainingBuf := buf[4:]
	if msgLen == 0 {
		return remainingBuf, KeepAliveMessage
	}

	// Do we have to unmarshal a message?
	if len(remainingBuf) < int(msgLen) {
		return buf, nil
	}

	// Calculate data & any remaining
	data := remainingBuf[:msgLen]
	remainingBuf = remainingBuf[msgLen:]

	// Build a message
	messageId := data[0]
	data = data[1:]
	switch messageId {
	case chokeId:
		return remainingBuf, Choke(p)
	case unchokeId:
		return remainingBuf, Unchoke(p)
	case interestedId:
		return remainingBuf, Interested(p)
	case uninterestedId:
		return remainingBuf, Uninterested(p)
	case haveId:
		index := toUint32(data)
		return remainingBuf, Have(p, index)
	case bitfieldId:
		return remainingBuf, Bitfield(p, data)
	case requestId:
		index := toUint32(data[0:4])
		begin := toUint32(data[4:8])
		length := toUint32(data[8:12])
		return remainingBuf, Request(p, index, begin, length)
	case blockId:
		index := toUint32(data[0:4])
		begin := toUint32(data[4:8])
		return remainingBuf, Block(p, index, begin, data[8:])
	case cancelId:
		index := toUint32(data[0:4])
		begin := toUint32(data[4:8])
		length := toUint32(data[8:12])
		return remainingBuf, Cancel(p, index, begin, length)
	default:
		fmt.Printf("Unknown message: %v", data)
		return remainingBuf, nil
	}
}

func toUint32(bytes []byte) uint32 {

	var a uint32
	l := len(bytes)
	for i, b := range bytes {
		shift := uint32((l - i - 1) * 8)
		a |= uint32(b) << shift
	}
	return a
}

// Private function to panic on write problems
func marshal(w io.Writer, order binary.ByteOrder, data interface{}) {
	err := binary.Write(w, order, data)
	if err != nil {
		panic(err)
	}
}
