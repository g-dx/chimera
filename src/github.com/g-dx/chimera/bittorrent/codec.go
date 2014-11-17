package bittorrent

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
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
// Basic message
////////////////////////////////////////////////////////////////////////////////////////////////

type ProtocolMessageWithId struct {
	msg    ProtocolMessage
	peerid *PeerIdentity
}

func (m ProtocolMessageWithId) PeerId() *PeerIdentity {
	return m.peerid
}

func (m ProtocolMessageWithId) Msg() ProtocolMessage {
	return m.msg
}

type ProtocolMessage interface {
	Id() byte
	Len() uint32
	String() string
}

type msg struct {
	len uint32
	id  byte
}

func (m msg) Id() byte {
	return m.id
}

func (m msg) Len() uint32 {
	return m.len
}

func (m msg) String() string {
	return "KeepAlive" // TODO: again this isn't great!
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

// TODO: This isn't great!
type HandshakeMessage struct {
	msg
	protocol string
	reserved [8]byte
	infoHash []byte
	peerId   string
}

func (m HandshakeMessage) String() string {
	return fmt.Sprintf("Handshake [%x, %v]", m.infoHash, m.peerId)
}

// Incoming handshake
func handshake(infoHash []byte, peerId []byte) *HandshakeMessage {
	return &HandshakeMessage{
		msg{uint32(handshakeLength - 4), 0}, // What a hack!...(sigh)
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

	return handshake(infoHash, PeerId)
}

////////////////////////////////////////////////////////////////////////////////////////////////
// KeepAlive <len=0000>
////////////////////////////////////////////////////////////////////////////////////////////////

// TODO: This isn't great!
var KeepAliveMessage = &msg{0, 0}

////////////////////////////////////////////////////////////////////////////////////////////////
// Choke        <len=0001><id=0>
// Unchoke      <len=0001><id=1>
// Interested   <len=0001><id=2>
// Uninterested <len=0001><id=3>
////////////////////////////////////////////////////////////////////////////////////////////////

type ChokeMessage struct {
	msg
}

func (m ChokeMessage) String() string {
	return "Choke"
}

type UnchokeMessage struct {
	msg
}

func (m UnchokeMessage) String() string {
	return "Unchoke"
}

type InterestedMessage struct {
	msg
}

func (m InterestedMessage) String() string {
	return "Interested"
}

type UninterestedMessage struct {
	msg
}

func (m UninterestedMessage) String() string {
	return "Uninterested"
}

var (
	Choke        = &ChokeMessage{msg{len: chokeLength, id: chokeId}}
	Unchoke      = &UnchokeMessage{msg{len: unchokeLength, id: unchokeId}}
	Interested   = &InterestedMessage{msg{len: interestedLength, id: interestedId}}
	Uninterested = &UninterestedMessage{msg{len: uninterestedLength, id: uninterestedId}}
)

////////////////////////////////////////////////////////////////////////////////////////////////
// Have <len=0005><id=4><piece index>
////////////////////////////////////////////////////////////////////////////////////////////////

type HaveMessage struct {
	msg
	index uint32
}

func (m HaveMessage) Index() uint32 {
	return m.index
}

func (m HaveMessage) String() string {
	return fmt.Sprintf("Have [%v]", m.index)
}

func Have(i uint32) *HaveMessage {
	return &HaveMessage{msg{len: haveLength, id: haveId}, i}
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Bitfield: <len=0001+X><id=5><bitfield>
////////////////////////////////////////////////////////////////////////////////////////////////

type BitfieldMessage struct {
	msg
	bits []byte
}

func (m BitfieldMessage) Bits() []uint8 {
	return m.bits
}

func (m BitfieldMessage) String() string {
	return fmt.Sprintf("Bitfield [%x]", m.bits)
}

func Bitfield(bits []byte) *BitfieldMessage {
	return &BitfieldMessage{msg{len: uint32(1 + len(bits)), id: bitfieldId}, bits}
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Request <len=0013><id=6><index><begin><length>
////////////////////////////////////////////////////////////////////////////////////////////////

type RequestMessage struct {
	msg
	index, begin, length uint32
}

func (m RequestMessage) Index() uint32 {
	return m.index
}

func (m RequestMessage) Begin() uint32 {
	return m.begin
}

func (m RequestMessage) Length() uint32 {
	return m.length
}

func (m RequestMessage) String() string {
	return fmt.Sprintf("Request [index:%v, begin:%v, length:%v]", m.index, m.begin, m.length)
}

func Request(index, begin, length uint32) *RequestMessage {
	return &RequestMessage{
		msg{len: requestLength, id: requestId},
		index,
		begin,
		length,
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Piece <len=0009+X><id=7><index><begin><block>
////////////////////////////////////////////////////////////////////////////////////////////////

type BlockMessage struct {
	msg
	index, begin uint32
	block        []byte
}

func (m BlockMessage) Index() uint32 {
	return m.index
}

func (m BlockMessage) Begin() uint32 {
	return m.begin
}

func (m BlockMessage) Block() []uint8 {
	return m.block
}

func (m BlockMessage) String() string {
	return fmt.Sprintf("Block [index:%v, begin:%v, %x...]", m.index, m.begin, m.block[0:10])
}

func Block(index, begin uint32, block []byte) *BlockMessage {
	return &BlockMessage{
		msg{len: uint32(9 + len(block)), id: blockId},
		index,
		begin,
		block,
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Cancel <len=0013><id=8><index><begin><length>
////////////////////////////////////////////////////////////////////////////////////////////////

type CancelMessage struct {
	msg
	index, begin, length uint32
}

func (m CancelMessage) Index() uint32 {
	return m.index
}

func (m CancelMessage) Begin() uint32 {
	return m.begin
}

func (m CancelMessage) Length() uint32 {
	return m.length
}

func (m CancelMessage) String() string {
	return fmt.Sprintf("Request [index:%v, begin:%v, length:%v]", m.index, m.begin, m.length)
}

func Cancel(index, begin, length uint32) *CancelMessage {
	return &CancelMessage{
		msg{len: cancelLength, id: cancelId},
		index,
		begin,
		length,
	}
}

func ReadHandshake(buf []byte, id *PeerIdentity) ([]byte, *ProtocolMessageWithId) {

	// Do we have enough data for handshake?
	if len(buf) < int(handshakeLength) {
		return nil, nil
	}

	// Calculate handshake data & any remaining
	data := buf[0:handshakeLength]
	remainingBuf := buf[handshakeLength:]

	// TODO: Assert the protocol & reserved bytes?

	return remainingBuf, &ProtocolMessageWithId{msg: handshake(data[28:48], data[48:handshakeLength]), peerid: id}
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

func UnmarshalWithId(buf []byte, id *PeerIdentity) ([]byte, *ProtocolMessageWithId) {
	remaining, msg := Unmarshal(buf)
	return remaining, &ProtocolMessageWithId{msg: msg, peerid: id}
}

func Unmarshal(buf []byte) ([]byte, ProtocolMessage) {

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
		return remainingBuf, Choke
	case unchokeId:
		return remainingBuf, Unchoke
	case interestedId:
		return remainingBuf, Interested
	case uninterestedId:
		return remainingBuf, Uninterested
	case haveId:
		index := toUint32(data)
		return remainingBuf, Have(index)
	case bitfieldId:
		return remainingBuf, Bitfield(data)
	case requestId:
		index := toUint32(data[0:4])
		begin := toUint32(data[4:8])
		length := toUint32(data[8:12])
		return remainingBuf, Request(index, begin, length)
	case blockId:
		index := toUint32(data[0:4])
		begin := toUint32(data[4:8])
		return remainingBuf, Block(index, begin, data[8:])
	case cancelId:
		index := toUint32(data[0:4])
		begin := toUint32(data[4:8])
		length := toUint32(data[8:12])
		return remainingBuf, Cancel(index, begin, length)
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
