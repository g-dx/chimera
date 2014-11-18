package bittorrent

import (
	"io"
)

type Connection struct {
}

func (c *Connection) Write(p []byte) (n int, err error) {
	return 0, nil
}

////////////////////////////////////////////////////////////////////////////////////////////////

type GenericMessage interface {
	io.WriterAt
	io.ReaderFrom
	Id() byte
	Len() uint32
}

type Message struct {
	raw []byte
	id  byte
	len uint32
}

func (m *Message) Id() byte {
	return m.id
}

func (m *Message) Len() uint32 {
	return m.len
}

func (m *Message) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(m.raw)
	return int64(n), err
}

type ChokeMessage2 Message
type UnChokeMessage2 Message
type InterestedMessage2 Message
type UnInterestedMessage2 Message

var Choke2 = &ChokeMessage2{raw: build(chokeLength, chokeId), id: chokeId, len: chokeLength}
var UnChoke2 = &UnChokeMessage2{raw: build(unchokeLength, unchokeId), id: unchokeId, len: unchokeLength}

////////////////////////////////////////////////////////////////////////////////////////////////

type BlockDetailsMessage struct {
	Message
	index, begin, length uint32
}

func (m BlockDetailsMessage) Index() uint32 {
	return m.index
}

func (m BlockDetailsMessage) Begin() uint32 {
	return m.begin
}

func (m BlockDetailsMessage) Length() uint32 {
	return m.length
}

func (m *BlockDetailsMessage) SetIndex(index uint32) {
	m.Index = index
	copy(m.raw[6:10], toUint32())
}

type CanMessage BlockDetailsMessage
type ReqMessage BlockDetailsMessage

func QuitMessage(index, begin, offset uint32) *QuitMessage {
	// Get from pool

	// Set fields

}

func build(len uint32, id byte) []byte {
	m := make([]byte, 5)
	return m
}
