package bittorrent

import (
	"testing"
	"bytes"
)

func TestEncode(t *testing.T) {

	byteEquals(t, []byte{0, 0, 0, 0}, encodeTo(KeepAlive{}))
	byteEquals(t, []byte{0, 0, 0, 1, 0}, encodeTo(Choke{}))
	byteEquals(t, []byte{0, 0, 0, 1, 1}, encodeTo(Unchoke{}))
	byteEquals(t, []byte{0, 0, 0, 1, 2}, encodeTo(Interested{}))
	byteEquals(t, []byte{0, 0, 0, 1, 3}, encodeTo(Uninterested{}))
	byteEquals(t, []byte{0, 0, 0, 5, 4, 0, 0, 0, 1}, encodeTo(Have(1)))
	byteEquals(t, []byte{0, 0, 0, 4, 5, 1, 2, 3}, encodeTo(Bitfield([]byte{1, 2, 3})))
	byteEquals(t, []byte{0, 0, 0, 13, 6, 0, 0, 0, 12, 0, 0, 0, 128, 0, 0, 32, 0},
		encodeTo(Request{12, 128, 8192}))
	byteEquals(t, []byte{0, 0, 0, 19, 7, 0, 0, 1, 0, 0, 0, 0, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		encodeTo(Block{256, 10, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}))
	byteEquals(t, []byte{0, 0, 0, 13, 8, 0, 0, 0, 3, 0, 0, 3, 0, 0, 0, 181, 253},
		encodeTo(Cancel{3, 768, 46589}))

}

func encodeTo(pm ProtocolMessage) []byte {
	buffer := make([]byte, int(msgLen+Len(pm)))
	Marshal(pm, buffer)
	return buffer
}

func TestDecode(t *testing.T) {

	buf := bytes.NewBuffer([]byte{0, 0, 0, 0, 0, 0, 0, 1, 0})

	pm := Unmarshal(buf)
	msgEquals(t, KeepAlive{}, pm)
	pm = Unmarshal(buf)
	msgEquals(t, Choke{}, pm)

	// Add more bytes
	buf.Write([]byte{0, 0, 0, 1})
	pm = Unmarshal(buf)
	isNil(t, pm)

	// Complete message
	buf.WriteByte(1)
	pm = Unmarshal(buf)
	msgEquals(t, Unchoke{}, pm)

	buf.Write([]byte{0, 0, 0, 1, 2})
	pm = Unmarshal(buf)
	msgEquals(t, Interested{}, pm)

	buf.Write([]byte{0, 0, 0, 1, 3})
	pm = Unmarshal(buf)
	msgEquals(t, Uninterested{}, pm)

	buf.Write([]byte{0, 0, 0, 5, 4, 0, 0, 1, 1})
	pm = Unmarshal(buf)
	msgEquals(t, Have(257), pm)

	buf.Write([]byte{0, 0, 0, 4, 5, 1, 2, 3})
	pm = Unmarshal(buf)
	msgEquals(t, Bitfield([]byte{1, 2, 3}), pm)

	buf.Write([]byte{0, 0, 0, 13, 6, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 3})
	pm = Unmarshal(buf)
	msgEquals(t, Request{1, 2, 3}, pm)

	buf.Write([]byte{0, 0, 0, 14, 7, 0, 0, 0, 1, 0, 0, 0, 2, 255, 255, 255, 255, 255})
	pm = Unmarshal(buf)
	msgEquals(t, Block{1, 2, []byte{255, 255, 255, 255, 255}}, pm)

	buf.Write([]byte{0, 0, 0, 13, 8, 0, 0, 0, 4, 0, 0, 0, 5, 0, 0, 0, 6})
	pm = Unmarshal(buf)
	msgEquals(t, Cancel{4, 5, 6}, pm)

}
