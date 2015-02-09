package bittorrent

import (
	"io"
	"log"
	"os"
	"crypto/sha1"
	"bytes"
	"errors"
	"fmt"
	"math"
)

const (
	BufLen = uint32(16 * 1024)
)

var errPieceHashIncorrect = errors.New("Piece hash was incorrect.")
var empty = make([]DiskMessage, 0)


////////////////////////////////////////////////////////////////////////////////////////////////
// Disk Op
////////////////////////////////////////////////////////////////////////////////////////////////

type DiskMessage interface {
}

type WriteMessage Block
type ReadMessage struct {
	id    *PeerIdentity
	block Block
}
// TODO: Implement Cancel

////////////////////////////////////////////////////////////////////////////////////////////////
// Disk Op Results
////////////////////////////////////////////////////////////////////////////////////////////////

type DiskMessageResult interface {
}

type ErrorResult struct {
	op string
	err error
}

type WriteOk int
type PieceOk int
type HashFailed int

type ReadOk struct {
	id    *PeerIdentity
	block Block
}


////////////////////////////////////////////////////////////////////////////////////////////////
// Disk Layout
////////////////////////////////////////////////////////////////////////////////////////////////

type DiskLayout struct {
	files []File
	pieceSize int
	lastPieceSize int
	size int64
}

func NewDiskLayout(files []File, pieceSize uint32, size uint64) *DiskLayout {
	// Calculate details
	lastPieceSize := int(size % uint64(pieceSize))
	if lastPieceSize == 0 {
		lastPieceSize = int(pieceSize)
	}
	return &DiskLayout{files, int(pieceSize), lastPieceSize, int64(size) }
}


////////////////////////////////////////////////////////////////////////////////////////////////
// Disk
////////////////////////////////////////////////////////////////////////////////////////////////

type Disk struct {
	in chan DiskMessage
	out chan DiskMessageResult
	done chan struct{}
	io IO
}

func NewDisk(io IO, out chan DiskMessageResult) *Disk {

	// Create, start & return
	disk := &Disk{make(chan DiskMessage, 50), out, make(chan struct{}), io}
	go disk.loop()
	return disk
}

func (d * Disk) loop() {
	ops := empty
	for {

		// Execute any outstanding ops
		for len(ops) != 0 {
			ops = d.execute(ops)
		}

		// Read new ops
		select {
		case op := <- d.in:
			ops = append(ops, op)

		case <- d.done:
			err := d.io.Close()
			if err != nil {
				// TODO: need channel to send err or nil
			}
		}
	}
}

func (d * Disk) Read(id *PeerIdentity, block Block) {
	d.in <- ReadMessage{id, block}
}

func (d * Disk) Write( block Block) {
	d.in <- WriteMessage(block)
}

func (d * Disk) execute(msgs []DiskMessage) []DiskMessage {

	msg := msgs[0]
	msgs = msgs[1:]

 	switch m := msg.(type) {
	case ReadMessage:
		err := d.io.ReadAt(m.block.block, int(m.block.index), int(m.block.begin))
		if err != nil {
			d.out <- ErrorResult{"read", err}
		} else {
			d.out <- ReadOk{m.id, m.block}
		}
	case WriteMessage:
		err := d.io.WriteAt(m.block, int(m.index), int(m.begin))
		if err == errPieceHashIncorrect {
			d.out <- HashFailed(int(m.index))
		} else if err != nil {
			d.out <- ErrorResult{"write", err}
		} else {
			d.out <- WriteOk(int(m.index))
		}

	default:
		panic(fmt.Sprintf("unknown op: %v", m))
	}
	return msgs
}

func (d * Disk) Close() error {
	close(d.done)
	// TODO: Should send channel for errors to come back on
	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////////
// IO Access
////////////////////////////////////////////////////////////////////////////////////////////////

type IO interface {
	WriteAt(p []byte, index, off int) error
	ReadAt(p []byte, index, off int) error
	io.Closer
}

////////////////////////////////////////////////////////////////////////////////////////////////
// IO Caching
////////////////////////////////////////////////////////////////////////////////////////////////

type WriteBuffer struct {
	buf []byte
	*BitSet
}

type CacheIO struct {
	io IO
	blockSize int
	layout *DiskLayout
	out chan DiskMessageResult
	hashes [][]byte
	w map[int]*WriteBuffer
}

func NewCacheIO(layout *DiskLayout, hashes[][]byte, blockSize int, out chan DiskMessageResult, io IO) *CacheIO {
	return &CacheIO{io, blockSize, layout, out, hashes, make(map[int]*WriteBuffer)}
}

func (ci * CacheIO) Close() error {
	// TODO: Should we flush buffers?
	return ci.io.Close()
}

func (ci * CacheIO) ReadAt(p []byte, index, off int) error {
	// TODO: Implement read caching
	return ci.io.ReadAt(p, index, off)
}

func (ci * CacheIO) WriteAt(p []byte, index, off int) error {
	wb, ok := ci.w[index]
	if !ok {

		// Calculate size
		size := ci.layout.pieceSize
		if index == len(ci.hashes) - 1 {
			size = ci.layout.lastPieceSize
		}

		// Calculate no of blocks
		blocks := int(math.Ceil(float64(size) / float64(ci.blockSize)))

		// Create new write buffer
		wb = &WriteBuffer{make([]byte, size), NewBitSet(blocks)}
		ci.w[index] = wb
	}

	// Check if we need this block
	block := off / ci.blockSize
	if !wb.Have(block) {

		copy(wb.buf[off:off+len(p)], p)
		wb.Set(block)

		// Are we done?
		if wb.IsComplete() {

			// Check hash
			sha := sha1.New()
			sha.Write(wb.buf)
			if !bytes.Equal(ci.hashes[index], sha.Sum(nil)) {
				return errPieceHashIncorrect
			}

			// Write piece
			err := ci.io.WriteAt(wb.buf, index, 0)
			if err != nil {
				return err
			}

			// Send success & remove buffer
			ci.out <- PieceOk(index)
			delete(ci.w, index) // TODO: should be reused for cached reads
		}
	}

	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Disk File
////////////////////////////////////////////////////////////////////////////////////////////////

type File struct {
	io.ReaderAt
	io.WriterAt
	io.Closer
	len int64
}

////////////////////////////////////////////////////////////////////////////////////////////////
// Disk IO
////////////////////////////////////////////////////////////////////////////////////////////////

type DiskIO struct {
	layout *DiskLayout
	lens  []int64
	log   *log.Logger
}

func NewDiskIO(layout *DiskLayout, log *log.Logger) *DiskIO {

	var l int64
	lens := make([]int64, 0, len(files))

	// Calculate continuous length
	for _, f := range layout.files {
		l += f.len
		lens = append(lens, l)
	}

	// Create & start
	da := &DiskIO{
		layout: layout,
		lens:  lens,
		log:   log,
	}
	return da
}

func (da *DiskIO) WriteAt(p []byte, index, off int) error {
	return da.onIO(p, index, off, onWriteBlock)
}

func (da *DiskIO) ReadAt(p []byte, index, off int) error {
	return da.onIO(p, index, off, onReadBlock)
}

func (da *DiskIO) Close() error {
	var err error
	for _, f := range da.layout.files {
		e := f.Close()
		if err == nil && e != nil {
			err = e
		}
	}
	return err
}

func (da DiskIO) onIO(buf []byte, index, begin int,
	ioFn func(File, []byte, int64) (int, error)) error {

	// Calculate starting positions
	i, prev, off := da.initIO(index, begin)
	pos := 0
	for ; i < len(da.layout.files) ; i++ {

		// Perform I/O
		n, err := ioFn(da.layout.files[i], buf[pos:], off-prev)

//		da.log.Printf("I/O: \nfile: %v\noff: %v\nbuf: % X", da.layout.files[i], off-prev, buf[pos:pos+n])

		if err != io.EOF {
			return err
		}

		// Should we move to the next file?
		pos += n
		off += int64(n)
		if err == io.EOF && pos != len(buf) {
//			da.log.Printf("I/O: Skipping to next file, %v bytes remaining", len(buf) - pos)
			prev = da.lens[i]
			continue
		}

		// Done
		return nil
	}

	// Ran out of data
	return io.ErrUnexpectedEOF
}

func (da * DiskIO) initIO(index, begin int) (int, int64, int64) {

	// Calc offset into files
	off := int64(index * da.layout.pieceSize) + int64(begin)

	// Calc len to previous file end
	for i, len := range da.lens {
		if off < len {
			prev := int64(0)
			if i > 0 {
				prev = da.lens[i-1]
			}
			return i, prev, off
		}
	}
	panic("Offset greater than length!")
}

func onReadBlock(f File, buf []byte, off int64) (int, error) {
	return f.ReadAt(buf, off)
}

func onWriteBlock(f File, buf []byte, off int64) (int, error) {
	return f.WriteAt(buf, off)
}

func CreateOrRead(mif []MetaInfoFile, dir string) ([]File, error) {

	files := make([]File, 0, len(mif))

	// Attempt to read files - if not present, create them
	for _, f := range mif {

		var file *os.File
		fileDir := dir + "/" + f.Path
		filePath := fileDir + "/" + f.Name

		// Stat file to see if it exists
		if _, err := os.Stat(filePath); os.IsExist(err) {

			// Open existing file
			file, err = os.Open(filePath)
			if err != nil {
				return nil, err
			}

		} else {

			// Create all dirs
			err = os.MkdirAll(fileDir, os.ModeDir|os.ModePerm)
			if err != nil {
				return nil, err
			}

			// Create new file
			file, err = os.Create(filePath)
			if err != nil {
				return nil, err
			}

			// Preallocate space
			err = file.Truncate(int64(f.Length))
			if err != nil {
				return nil, err
			}
		}

		files = append(files, File{file, file, file, int64(f.Length)})
	}

	return files, nil
}
