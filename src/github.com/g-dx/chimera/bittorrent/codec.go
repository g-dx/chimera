package bittorrent

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sync"
	"crypto/sha1"
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
	keepAliveLength    uint32 = 0
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
		return &HaveMessage{GenericMessage{haveLength, haveId, nil}, 0}
	},
}

var requestPool = &sync.Pool{
	New: func() interface{} {
		return &RequestMessage{BlockDetailsMessage{GenericMessage{requestLength, requestId, nil}, 0, 0, 0}}
	},
}

// Used for clearing slices on reuse
var emptyBlock = make([]byte, 0, _16KB)

var blockPool = &sync.Pool{
	New: func() interface{} {
		// TODO: Fix this length calculation. Are we enforcing supporting requests of only 16Kb?
		return &BlockMessage{GenericMessage{_16KB + 9, blockId, nil}, 0, 0, make([]byte, _16KB)}
	},
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Message list
////////////////////////////////////////////////////////////////////////////////////////////////

type MessageList struct {
	msgs []ProtocolMessage
	id   *PeerIdentity
}

func NewMessageList(id *PeerIdentity, msgs ...ProtocolMessage) *MessageList {
	return &MessageList{msgs, id}
}

func (ml *MessageList) Add(pm ProtocolMessage) {
	ml.msgs = append(ml.msgs, pm)
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Basic message
////////////////////////////////////////////////////////////////////////////////////////////////

type ProtocolMessage interface {
	Id() byte
	Len() uint32
	Recycle()
}

type GenericMessage struct {
	len  uint32
	id   byte
	pool *sync.Pool
}

func (m *GenericMessage) Id() byte {
	return m.id
}

func (m *GenericMessage) Len() uint32 {
	return m.len
}

func (m *GenericMessage) Recycle() {
	if m.pool != nil {
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
func handshake(infoHash []byte, peerId []byte) *HandshakeMessage {
	return &HandshakeMessage{
		protocolName,
		[8]byte{},
		infoHash,
		string(peerId),
	}
}

// Outgoing handshake
func Handshake(infoHash []byte) *HandshakeMessage {
	if len(infoHash) != sha1.Size {
		panic(errors.New("Invalid info_hash length."))
	}

	return handshake(infoHash, PeerId)
}

func WriteHandshake(h *HandshakeMessage) []byte {
	buf := make([]byte, 0, handshakeLength)
	buf = append(buf, byte(len(protocolName)))
	buf = append(buf, []byte(protocolName)...)
	buf = append(buf, make([]byte, 8)...)
	buf = append(buf, h.infoHash...)
	buf = append(buf, h.peerId...)
	return buf
}

func ReadHandshake(buf []byte) *HandshakeMessage {
	// TODO: Assert the protocol & reserved bytes?

	return handshake(buf[28:48], buf[48:handshakeLength])
}

////////////////////////////////////////////////////////////////////////////////////////////////
// KeepAlive <len=0000>
////////////////////////////////////////////////////////////////////////////////////////////////

type KeepAliveMessage struct{ GenericMessage }

var KeepAlive = &KeepAliveMessage{GenericMessage{keepAliveLength, 0, nil}}

////////////////////////////////////////////////////////////////////////////////////////////////
// Choke        <len=0001><id=0>
// Unchoke      <len=0001><id=1>
// Interested   <len=0001><id=2>
// Uninterested <len=0001><id=3>
////////////////////////////////////////////////////////////////////////////////////////////////

type ChokeMessage struct{ GenericMessage }
type UnchokeMessage struct{ GenericMessage }
type InterestedMessage struct{ GenericMessage }
type UninterestedMessage struct{ GenericMessage }

func Choke() ProtocolMessage {
	return &ChokeMessage{GenericMessage{chokeLength, chokeId, nil}}
}

func Unchoke() ProtocolMessage {
	return &UnchokeMessage{GenericMessage{unchokeLength, unchokeId, nil}}
}

func Interested() ProtocolMessage {
	return &InterestedMessage{GenericMessage{interestedLength, interestedId, nil}}
}

func Uninterested() ProtocolMessage {
	return &UninterestedMessage{GenericMessage{uninterestedLength, uninterestedId, nil}}
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

func Have(i uint32) *HaveMessage {
	have := havePool.Get().(*HaveMessage)
	have.index = i
	have.pool = havePool
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

func Bitfield(bits []byte) *BitfieldMessage {
	return &BitfieldMessage{GenericMessage{uint32(1 + len(bits)), bitfieldId, nil}, bits}
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

type CancelMessage struct{ BlockDetailsMessage }
type RequestMessage struct{ BlockDetailsMessage }

func Request(index, begin, length uint32) *RequestMessage {
	req := requestPool.Get().(*RequestMessage)
	req.index = index
	req.begin = begin
	req.length = length
	req.pool = requestPool
	return req
}

func Cancel(index, begin, length uint32) *CancelMessage {
	return &CancelMessage{
		BlockDetailsMessage{
			GenericMessage{cancelLength, cancelId, nil},
			index,
			begin,
			length,
		},
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

func (m *BlockMessage) Index() uint32 {
	return m.index
}

func (m *BlockMessage) Begin() uint32 {
	return m.begin
}

func (m *BlockMessage) Block() []byte {
	return m.block
}

func Block(index, begin uint32, block []byte) *BlockMessage {
	b := blockPool.Get().(*BlockMessage)
	b.index = index
	b.begin = begin
	b.block = b.block[:len(block)] // Set slice to block len
	copy(b.block, block)
	return b
}

// TODO: possibly remove me..
func EmptyBlock(index, begin uint32) *BlockMessage {
	b := blockPool.Get().(*BlockMessage)
	b.index = index
	b.begin = begin
	copy(b.block, emptyBlock)
	return b
}

func Marshal(pm ProtocolMessage, buf []byte) {

	// NOTE: buf is always guaranteed to be able to hold the message

	// Add len & id
	PutUint32(buf[0:4], pm.Len())
	if pm.Id() != 0 {
		buf[4] = pm.Id()
	}

	switch msg := pm.(type) {
	case *HaveMessage:
		PutUint32(buf[5:9], msg.index)

	case *BlockMessage:
		PutUint32(buf[5:9], msg.index)
		PutUint32(buf[9:13], msg.begin)
		copy(buf[13:13+len(msg.block)], msg.block)

	case *RequestMessage:
		PutUint32(buf[5:9], msg.index)
		PutUint32(buf[9:13], msg.begin)
		PutUint32(buf[13:17], msg.length)

	case *CancelMessage:
		PutUint32(buf[5:9], msg.index)
		PutUint32(buf[9:13], msg.begin)
		PutUint32(buf[13:17], msg.length)

	case *BitfieldMessage:
		copy(buf[5:len(msg.bits)+5], msg.bits)
	}
}

func Unmarshal(r *bytes.Buffer) ProtocolMessage {

	// Do we have enough to calculate the length?
	if r.Len() < 4 {
		return nil
	}

	// Check: Keepalive
	msgLen := Uint32(r.Bytes()[0:4])
	if msgLen == 0 {
		r.Next(4)
		return KeepAlive
	}

	// Do we have enough to unmarshal a message?
	if r.Len() < int(msgLen)+4 {
		return nil
	}

	// Discard header & read message
	r.Next(4)
	bytes := r.Next(int(msgLen))

	// Build a message
	messageId := bytes[0]
	data := bytes[1:]
	switch messageId {
	case chokeId:
		return Choke()
	case unchokeId:
		return Unchoke()
	case interestedId:
		return Interested()
	case uninterestedId:
		return Uninterested()
	case haveId:
		index := Uint32(data)
		return Have(index)
	case bitfieldId:
		return Bitfield(data)
	case requestId:
		index := Uint32(data[0:4])
		begin := Uint32(data[4:8])
		length := Uint32(data[8:12])
		// TODO: If length is > 16Kb close connection?
		return Request(index, begin, length)
	case blockId:
		index := Uint32(data[0:4])
		begin := Uint32(data[4:8])
		return Block(index, begin, data[8:])
	case cancelId:
		index := Uint32(data[0:4])
		begin := Uint32(data[4:8])
		length := Uint32(data[8:12])
		return Cancel(index, begin, length)
	default:
		// Unknown message
		return &GenericMessage{msgLen, messageId, nil}
	}
}

// Private function to read byte slice to uint32
func Uint32(bytes []byte) uint32 {
	return binary.BigEndian.Uint32(bytes)
}

// Private function to write uint32 into byte slice
func PutUint32(b []byte, i uint32) {
	binary.BigEndian.PutUint32(b, i)
}

func ToString(pm ProtocolMessage) string {

	switch m := pm.(type) {
	case *KeepAliveMessage:
		return "KeepAlive"
	case *ChokeMessage:
		return "Choke"
	case *UnchokeMessage:
		return "Unchoke"
	case *InterestedMessage:
		return "Interested"
	case *UninterestedMessage:
		return "Uninterested"
	case *HaveMessage:
		return fmt.Sprintf("Have [%v]", m.index)
	case *BlockMessage:
		return fmt.Sprintf("Block [index:%v, begin:%v, %x...]", m.index, m.begin, m.block[0:int(math.Min(10, float64(len(m.block))))])
	case *CancelMessage:
		return fmt.Sprintf("Cancel [index:%v, begin:%v, length:%v]", m.index, m.begin, m.length)
	case *RequestMessage:
		return fmt.Sprintf("Request [index:%v, begin:%v, length:%v]", m.index, m.begin, m.length)
	case *BitfieldMessage:
		return fmt.Sprintf("Bitfield [%x]", m.bits)
	case *GenericMessage:
		return fmt.Sprintf("Generic Message [Len: %v, Id: %v]", m.len, m.id)
	default:
		return fmt.Sprintf("Unknown Message: %v", m)
	}
}

func OnRequest(pm ProtocolMessage, f func(req *RequestMessage)) {
	if req, ok := pm.(*RequestMessage); ok {
		f(req)
	}
}
