package bittorrent

import (
	"testing"
)

const (
	lenSize = 4
)

func TestEncode(t *testing.T) {

	byteEquals(t, []byte{0, 0, 0, 0}, encodeTo(KeepAlive))
	byteEquals(t, []byte{0, 0, 0, 1, 0}, encodeTo(Choke(nil)))
	byteEquals(t, []byte{0, 0, 0, 1, 1}, encodeTo(Unchoke(nil)))
	byteEquals(t, []byte{0, 0, 0, 1, 2}, encodeTo(Interested(nil)))
	byteEquals(t, []byte{0, 0, 0, 1, 3}, encodeTo(Uninterested(nil)))
	byteEquals(t, []byte{0, 0, 0, 5, 4, 0, 0, 0, 1}, encodeTo(Have(nil, 1)))
	byteEquals(t, []byte{0, 0, 0, 4, 5, 1, 2, 3}, encodeTo(Bitfield(nil, []byte{1, 2, 3})))
	byteEquals(t, []byte{0, 0, 0, 13, 6, 0, 0, 0, 12, 0, 0, 0, 128, 0, 0, 32, 0},
		encodeTo(Request(nil, 12, 128, 8192)))
	byteEquals(t, []byte{0, 0, 0, 19, 7, 0, 0, 1, 0, 0, 0, 0, 10, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
		encodeTo(Block(nil, 256, 10, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})))
	byteEquals(t, []byte{0, 0, 0, 13, 8, 0, 0, 0, 3, 0, 0, 3, 0, 0, 0, 181, 253},
		encodeTo(Cancel(nil, 3, 768, 46589)))

}

func encodeTo(pm ProtocolMessage) []byte {
	buffer := make([]byte, int(lenSize+pm.Len()))
	Marshal(pm, buffer)
	return buffer
}

func TestDecode(t *testing.T) {

	buf := []byte{0, 0, 0, 0, 0, 0, 0, 1, 0}
	id := &PeerIdentity{[]byte("unknown"), "addr"}

	remainingBuf, pm := Unmarshal(id, buf)
	msgEquals(t, KeepAlive, pm)
	remainingBuf, pm = Unmarshal(id, remainingBuf)
	msgEquals(t, Choke(id), pm)

	// Add more bytes
	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 1}...)
	remainingBuf, pm = Unmarshal(id, remainingBuf)
	isNil(t, pm)

	// Complete message
	remainingBuf = append(remainingBuf, 1)
	remainingBuf, pm = Unmarshal(id, remainingBuf)
	msgEquals(t, Unchoke(id), pm)

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 1, 2}...)
	remainingBuf, pm = Unmarshal(id, remainingBuf)
	msgEquals(t, Interested(id), pm)

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 1, 3}...)
	remainingBuf, pm = Unmarshal(id, remainingBuf)
	msgEquals(t, Uninterested(id), pm)

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 5, 4, 0, 0, 1, 1}...)
	remainingBuf, pm = Unmarshal(id, remainingBuf)
	msgEquals(t, Have(id, 257), pm)

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 4, 5, 1, 2, 3}...)
	remainingBuf, pm = Unmarshal(id, remainingBuf)
	msgEquals(t, Bitfield(id, []byte{1, 2, 3}), pm)

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 13, 6, 0, 0, 0, 1, 0, 0, 0, 2, 0, 0, 0, 3}...)
	remainingBuf, pm = Unmarshal(id, remainingBuf)
	msgEquals(t, Request(id, 1, 2, 3), pm)

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 14, 7, 0, 0, 0, 1, 0, 0, 0, 2, 255, 255, 255, 255, 255, 10}...)
	remainingBuf, pm = Unmarshal(id, remainingBuf)
	msgEquals(t, Block(id, 1, 2, []byte{255, 255, 255, 255, 255}), pm)
	intEquals(t, int64(len(remainingBuf)), 1)
	intEquals(t, int64(remainingBuf[0]), 10)
	remainingBuf = []byte{}

	remainingBuf = append(remainingBuf, []byte{0, 0, 0, 13, 8, 0, 0, 0, 4, 0, 0, 0, 5, 0, 0, 0, 6}...)
	remainingBuf, pm = Unmarshal(id, remainingBuf)
	msgEquals(t, Cancel(id, 4, 5, 6), pm)

}
