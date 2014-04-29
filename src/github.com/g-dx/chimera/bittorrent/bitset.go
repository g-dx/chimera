package bittorrent

import (
	"fmt"
	"bytes"
)

var bitMasks = [8]byte{ 1, 2, 4, 8, 16, 32, 64, 128 }

type BitSet struct {
	bits []byte
	size uint32
	complete bool
}

func NewBitSet(size uint32) *BitSet {

	// Ensure we have enough storage
	len := size/8
	if size%8 != 0 {
		len++
	}

	return &BitSet { make([]uint8, len), size, false }
}

func NewFromBytes(bits []byte, size uint32) *BitSet {
	// TODO: Ensure high bits are not set...
	return &BitSet { bits, size, false }
}

func (bs BitSet) Have(i uint32) bool {
	if !bs.IsValid(i) {
		return false
	}

	return (bs.bits[i/8] & bitMasks[i%8]) == bitMasks[i%8]
}

func (bs BitSet) Size() uint32 {
	return bs.size
}

func (bs *BitSet) Set(i uint32) {
	if bs.IsValid(i) && !bs.Have(i) {
		bs.bits[i/8] = bs.bits[i/8] | bitMasks[i%8]
	}
}

func (bs BitSet) IsValid(i uint32) bool {
	return i >= 0 && i < bs.size
}

func (bs *BitSet) IsComplete() bool {

	// Fast path
	if bs.complete {
		return true
	}

	// Check all but last byte
	for i := 0 ; i < len(bs.bits)-2 ; i++ {
		if bs.bits[i] != 0xFF {
			return false
		}
	}

	// Check last byte
	b := uint8(8)
	if bs.size%8 != 0 {
		b = uint8(bs.size%8)
	}
	bs.complete = bs.bits[len(bs.bits)-1] == (0xFF >> (8-b))
	return bs.complete
}

func (bs BitSet) String() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("BitSet, size=%v ", bs.size))
	for i := len(bs.bits)-1 ; i >= 0 ; i-- {
		buf.WriteString(fmt.Sprintf("[%v]%08b", 8*(i+1), bs.bits[i]))
	}
	return buf.String()
}
