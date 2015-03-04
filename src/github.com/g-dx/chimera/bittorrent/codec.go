package bittorrent

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"crypto/sha1"
	"net"
	"time"
    "strings"
)

const MaxInt = ^uint(0) >> 1
const msgLen = 4

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
	keepAliveLength    int = 0
	chokeLength        int = 1
	unchokeLength      int = 1
	interestedLength   int = 1
	uninterestedLength int = 1
	haveLength         int = 5
	cancelLength       int = 13
	requestLength      int = 13
	handshakeLength    int = 68
)

////////////////////////////////////////////////////////////////////////////////////////////////
// Message list
////////////////////////////////////////////////////////////////////////////////////////////////

type MessageList struct {
	msgs []ProtocolMessage
	id   PeerIdentity
}

func NewMessageList(id PeerIdentity, msgs ...ProtocolMessage) *MessageList {
	return &MessageList{msgs, id}
}

// TODO - Decide whether to keep me
type MsgList []ProtocolMessage

func Msgs(msgs ...ProtocolMessage) MsgList {
	return MsgList(msgs)
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Basic message & wrapper
////////////////////////////////////////////////////////////////////////////////////////////////

type ProtocolMessage interface {
}

type ProtocolMessages []ProtocolMessage

func (pm ProtocolMessages) String() string {
    var s []string
    for _, m := range pm {
        s = append(s, ToString(m))
    }
    return "[" + strings.Join(s, ", ") + "]"
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

func WriteHandshake(c net.Conn, h *HandshakeMessage) {
	// Create buffer
	buf := make([]byte, 0, handshakeLength)
	buf = append(buf, byte(len(protocolName)))
	buf = append(buf, []byte(protocolName)...)
	buf = append(buf, make([]byte, 8)...)
	buf = append(buf, h.infoHash...)
	buf = append(buf, h.peerId...)

	// Set timeout & write
	c.SetWriteDeadline(time.Now().Add(handshakeTimeout))
	_, err := c.Write(buf)
	if err != nil {
		panic(err) // TODO: Check failure to write everything is an error
	}
}

func ReadHandshake(buf []byte) *HandshakeMessage {
	// TODO: Assert the protocol & reserved bytes?

	return handshake(buf[28:48], buf[48:handshakeLength])
}

////////////////////////////////////////////////////////////////////////////////////////////////
// KeepAlive <len=0000>
////////////////////////////////////////////////////////////////////////////////////////////////

type KeepAlive struct{}

////////////////////////////////////////////////////////////////////////////////////////////////
// Choke        <len=0001><id=0>
// Unchoke      <len=0001><id=1>
// Interested   <len=0001><id=2>
// Uninterested <len=0001><id=3>
////////////////////////////////////////////////////////////////////////////////////////////////

type Choke struct{}
type Unchoke struct{}
type Interested struct{}
type Uninterested struct{}

////////////////////////////////////////////////////////////////////////////////////////////////
// Have <len=0005><id=4><piece index>
////////////////////////////////////////////////////////////////////////////////////////////////

type Have int

////////////////////////////////////////////////////////////////////////////////////////////////
// Bitfield: <len=0001+X><id=5><bitfield>
////////////////////////////////////////////////////////////////////////////////////////////////

type Bitfield []byte

////////////////////////////////////////////////////////////////////////////////////////////////
// Request <len=0013><id=6><index><begin><length>
// Cancel <len=0013><id=8><index><begin><length>
////////////////////////////////////////////////////////////////////////////////////////////////

type Cancel struct{ index, begin, length int }
type Request struct{ index, begin, length int }

////////////////////////////////////////////////////////////////////////////////////////////////
// Piece <len=0009+X><id=7><index><begin><block>
////////////////////////////////////////////////////////////////////////////////////////////////

type Block struct {
	index, begin int
	block []byte
}

func Marshal(pm ProtocolMessage, buf []byte) int {

	// NOTE: buf is always guaranteed to be able to hold the message

	// Add len & id
	l := Len(pm)
	PutUint32(buf[0:4], l)
	id := Id(pm)
	if id != 0 {
		buf[4] = id
	}

	switch msg := pm.(type) {
	case Have:
		PutUint32(buf[5:9], int(msg))

	case Block:
		PutUint32(buf[5:9], msg.index)
		PutUint32(buf[9:13], msg.begin)
		copy(buf[13:13+len(msg.block)], msg.block)

	case Request:
		PutUint32(buf[5:9], msg.index)
		PutUint32(buf[9:13], msg.begin)
		PutUint32(buf[13:17], msg.length)

	case Cancel:
		PutUint32(buf[5:9], msg.index)
		PutUint32(buf[9:13], msg.begin)
		PutUint32(buf[13:17], msg.length)

	case Bitfield:
		copy(buf[5:len(msg)+5], msg)
	}
	return l+4
}

func Unmarshal(r *bytes.Buffer) []ProtocolMessage {

	var msgs []ProtocolMessage
	for {
		// Do we have enough to calculate the length?
		if r.Len() < msgLen {
			return msgs
		}

		// Check: Keepalive
		payloadLen := Int(r.Bytes()[:msgLen])
		if payloadLen == 0 {
			r.Next(msgLen)
			msgs = append(msgs, KeepAlive{})
			continue
		}

		// Do we have enough to unmarshal a message?
		if r.Len() < msgLen+payloadLen {
			return msgs
		}

		// Discard header & read message
		r.Next(msgLen)
		bytes := r.Next(payloadLen)

		// Build a message
		messageId := bytes[0]
		data := bytes[1:]
		var msg ProtocolMessage
		switch messageId {
		case chokeId: msg = Choke{}
		case unchokeId: msg = Unchoke{}
		case interestedId: msg = Interested{}
		case uninterestedId: msg = Uninterested{}
		case haveId: msg = Have(Int(data))
		case bitfieldId:
			bits := make([]byte, len(data))
			copy(bits, data)
			msg = Bitfield(bits)
		case requestId:
			// TODO: If length is > 16Kb close connection?
			msg = Request{Int(data[:4]), Int(data[4:8]), Int(data[8:12])}
		case blockId:
			block := make([]byte, len(data)-8)
			copy(block, data[8:])
			msg = Block{Int(data[:4]), Int(data[4:8]), block}
		case cancelId: msg = Cancel{Int(data[:4]), Int(data[4:8]), Int(data[8:12])}
		default:
			continue // Unknown message - skip - TODO: should log this.
		}
		msgs = append(msgs, msg)
	}
}

func Len(pm ProtocolMessage) int {
	switch m := pm.(type) {
	case KeepAlive: return keepAliveLength
	case Choke: return chokeLength
	case Unchoke: return unchokeLength
	case Interested: return interestedLength
	case Uninterested: return uninterestedLength
	case Have: return haveLength
	case Block: return 9 + len(m.block)
	case Cancel: return cancelLength
	case Request: return requestLength
	case Bitfield: return 1 + len(m)
	default:
		panic(fmt.Sprintf("Unknown message: %v", pm))
	}
}

func Id(pm ProtocolMessage) byte {
	switch m := pm.(type) {
	case KeepAlive: return 0 // TODO: Special marker
	case Choke: return chokeId
	case Unchoke: return unchokeId
	case Interested: return interestedId
	case Uninterested: return uninterestedId
	case Have: return haveId
	case Block: return blockId
	case Cancel: return cancelId
	case Request: return requestId
	case Bitfield: return bitfieldId
	default:
		panic(fmt.Sprintf("Unknown message: %v", m))
	}
}

func ToString(pm ProtocolMessage) string {
	switch m := pm.(type) {
	case KeepAlive: return "KeepAlive"
	case Choke: return "Choke"
	case Unchoke: return "Unchoke"
	case Interested: return "Interested"
	case Uninterested: return "Uninterested"
	case Have: return fmt.Sprintf("Have [%v]", m)
	case Block: return fmt.Sprintf("Block [index:%v, begin:%v, %x...]", m.index, m.begin, m.block[0:int(math.Min(10, float64(len(m.block))))])
	case Cancel: return fmt.Sprintf("Cancel [index:%v, begin:%v, length:%v]", m.index, m.begin, m.length)
	case Request: return fmt.Sprintf("Request [index:%v, begin:%v, length:%v]", m.index, m.begin, m.length)
	case Bitfield: return fmt.Sprintf("Bitfield [%x]", m)
	default: return fmt.Sprintf("Unknown Message: %v", m)
	}
}

// Private function to read byte slice to int
func Int(b []byte) int {
	i := binary.BigEndian.Uint32(b)
	if uint(i) > MaxInt {
		panic("Cannot decode int")
	}
	return int(i)
}

// Private function to write int into byte slice
func PutUint32(b []byte, i int) {
	binary.BigEndian.PutUint32(b, uint32(i))
}
