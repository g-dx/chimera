package bittorrent

import (
	"testing"
)

const (
	lenSize = 4
)

func TestEncode(t *testing.T) {

	byteEquals(t, []byte{0, 0, 0, 0}, encodeTo(KeepAlive))
	byteEquals(t, []byte{0, 0, 0, 1, 0}, encodeTo(Choke()))
	byteEquals(t, []byte{0, 0, 0, 1, 1}, encodeTo(Unchoke()))
	byteEquals(t, []byte{0, 0, 0, 1, 2}, encodeTo(Interested()))
	byteEquals(t, []byte{0, 0, 0, 1, 3}, encodeTo(Uninterested()))
	byteEquals(t, []byte{0, 0, 0, 5, 4, 0, 0, 0, 1}, encodeTo(Have(1)))
	byteEquals(t, []byte{0, 0, 0, 4, 5, 1, 2, 3}, encodeTo(Bitfield([]byte{1, 2, 3})))
	byteEquals(t, []byte{0, 0, 0, 13, 6, 0, 0, 0, 12, 0, 0, 0, 128, 0, 0, 32, 0},
		encodeTo(Request(12, 128, 8192)))
	byteEquals(t, []byte{0, 0, 0, 19, 7, 0, 0, 1, 0, 0, 0, 0, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		encodeTo(Block(256, 10, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})))
	byteEquals(t, []byte{0, 0, 0, 13, 8, 0, 0, 0, 3, 0, 0, 3, 0, 0, 0, 181, 253},
		encodeTo(Cancel(3, 768, 46589)))

}

func encodeTo(pm ProtocolMessage) []byte {
	buffer := make([]byte, int(lenSize+pm.Len()))
	Marshal(pm, buffer)
	return buffer
}

func TestDecode(t *testing.T) {

	buf := []byte{0, 0, 0, 0, 0, 0, 0, 1, 0}

	remainingBuf, pm := Unmarshal(buf)
	msgEquals(t, KeepAlive, pm)
	remainingBuf, pm = Unmarshal(remainingBuf)
	msgEquals(t, Choke(), pm)

	// Add more bytes
	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 1}...)
	remainingBuf, pm = Unmarshal(remainingBuf)
	isNil(t, pm)

	// Complete message
	remainingBuf = append(remainingBuf, 1)
	remainingBuf, pm = Unmarshal(remainingBuf)
	msgEquals(t, Unchoke(), pm)

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 1, 2}...)
	remainingBuf, pm = Unmarshal(remainingBuf)
	msgEquals(t, Interested(), pm)

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 1, 3}...)
	remainingBuf, pm = Unmarshal(remainingBuf)
	msgEquals(t, Uninterested(), pm)

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 5, 4, 0, 0, 1, 1}...)
	remainingBuf, pm = Unmarshal(remainingBuf)
	msgEquals(t, Have(257), pm)

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 4, 5, 1, 2, 3}...)
	remainingBuf, pm = Unmarshal(remainingBuf)
	msgEquals(t, Bitfield([]byte{1, 2, 3}), pm)

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 13, 6, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 3}...)
	remainingBuf, pm = Unmarshal(remainingBuf)
	msgEquals(t, Request(1, 2, 3), pm)

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 14, 7, 0, 0, 0, 1, 0, 0, 0, 2, 255, 255, 255, 255, 255, 10}...)
	remainingBuf, pm = Unmarshal(remainingBuf)
	msgEquals(t, Block(1, 2, []byte{255, 255, 255, 255, 255}), pm)
	intEquals(t, 1, len(remainingBuf))
	intEquals(t, 10, int(remainingBuf[0]))
	remainingBuf = []byte{}

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 13, 8, 0, 0, 0, 4, 0, 0, 0, 5, 0, 0, 0, 6}...)
	remainingBuf, pm = Unmarshal(remainingBuf)
	msgEquals(t, Cancel(4, 5, 6), pm)

}
