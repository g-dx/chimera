package bittorrent

import (
	"errors"
	"io"
	"os"
)

type Disk struct {
	out        chan<- DiskResult
	in         <-chan DiskOp
	cancel     <-chan CancelOp
	unfinished []UnfinishedPiece
	complete   []Piece
}

var errNotFinished = errors.New("Not finished - no reads allowed.")
var errFinished = errors.New("Finished - no writes allowed.")

type DiskPiece interface {
	io.ReaderAt
	io.WriterAt
	IsComplete() bool // false = no reads, true == no writes
}

type diskPiece struct {
	buf        []byte
	hash       []byte
	blocks     *BitSet
	isComplete bool
}

func (dp diskPiece) WriteAt(p []byte, off int64) (n int, err error) {
	if dp.isComplete {
		return 0, errFinished
	}

	copy(dp.buf[off:len(p)], p)
	dp.blocks.Set(off / _16KB)
	return len(p), nil
}

func (dp diskPiece) ReadAt(p []byte, off int64) (n int, err error) {

	if dp.isComplete {
		return 0, errFinished
	}

	copy(p, i.buf[off:len(p)])
	return len(p), nil
}

type onDisk struct {
	f os.File
}

func (o onDisk) ReadAt(p []byte, off int64) (n int, err error) {
	n, err = o.f.ReadAt(p)
}

type DiskResult interface {
}

type PeiceWriteResult struct {
	index uint32
	err   error
}

type BlockReadResult struct {
	id    *PeerIdentity
	block *BlockMessage
	err   error
}

type DiskOp interface {
}

type WriteOp *BlockMessage
type ReadOp struct {
	id    *PeerIdentity
	block *BlockMessage
}

type CancelOp struct {
	index, begin uint32
}

func (disk *Disk) Read(index, begin uint32, id PeerIdentity) {
	disk.in <- &ReadOp{id, EmptyBlock(index, begin)}
}

func (disk *Disk) Write(index, begin uint32, block []byte) {
	disk.in <- WriteOp(Block(index, begin, block))
}

func (disk *Disk) Cancel(index, begin uint32) {
	disk.cancel <- CancelOp{index, begin}
}

type DiskProgress struct {
	pieces map[int]DiskPart
}

type DiskPart struct {
	buf []byte
	blocks *BitSet
}

