package bittorrent

import (
	"io"
	"log"
	"os"
)

const (
	BufLen = uint32(16 * 1024)
)

type DiskOp2 interface {
	Id() *PeerIdentity
}

type WriteOp2 struct {
	id           *PeerIdentity
	index, begin uint32
	block        []byte
}

type ReadOp2 struct {
	id                   *PeerIdentity
	index, begin, length uint32
}

func (dw WriteOp2) Id() *PeerIdentity {
	return dw.id
}

func DiskWrite(b *BlockMessage, id *PeerIdentity) *WriteOp2 {
	return &WriteOp2{
		id:    id,
		index: b.Index(),
		begin: b.Begin(),
		block: b.Block(),
	}
}

func (dm ReadOp2) Id() *PeerIdentity {
	return dm.id
}

func DiskRead(r *RequestMessage, id *PeerIdentity) *ReadOp2 {
	return &ReadOp2{
		id:     id,
		index:  r.Index(),
		begin:  r.Begin(),
		length: r.Length(),
	}
}

type DiskOp2Result interface {
	Id() *PeerIdentity
}

type DiskReadResult struct {
	id *PeerIdentity
	b  *BlockMessage
}

func (drr DiskReadResult) Id() *PeerIdentity {
	return drr.id
}

type DiskWriteResult struct {
	id                   *PeerIdentity
	index, begin, length uint32
}

func (dwr DiskWriteResult) Id() *PeerIdentity {
	return dwr.id
}

func mockDisk(reader <-chan DiskOp2, logger *log.Logger) {
	for {
		select {
		case r := <-reader:
			logger.Printf("Disk Read: %v\n", r)
			//		case w := <- writer:
			//			logger.Printf("Disk Write: %v\n", w)
		}
	}
}

type DiskAccess struct {
	files []*os.File
	lens  []uint64
	mi    *MetaInfo
	in    <-chan DiskOp2
	out   chan<- DiskOp2Result
	log   *log.Logger
}

func NewDiskAccess(mi *MetaInfo,
	in <-chan DiskOp2,
	out chan<- DiskOp2Result,
	dir string,
	log *log.Logger) (*DiskAccess, error) {

	// Read or create files
	files, lens, err := initialise(mi.Files, dir)
	if err != nil {
		return nil, err
	}

	// Create & start
	da := &DiskAccess{
		files: files,
		lens:  lens,
		mi:    mi,
		in:    in,
		out:   out,
		log:   log,
	}
	go da.loop()
	return da, nil
}

func initialise(mif []MetaInfoFile, dir string) ([]*os.File, []uint64, error) {

	var l uint64
	lens := make([]uint64, 0, len(mif))
	files := make([]*os.File, 0, len(mif))

	// Attempt to read files - if not present, create them
	for _, f := range mif {

		var file *os.File
		fileDir := dir + "/" + f.Path
		filePath := fileDir + f.Name

		// Stat file to see if it exists
		if _, err := os.Stat(filePath); os.IsExist(err) {

			// Open existing file
			file, err = os.Open(filePath)
			if err != nil {
				return nil, nil, err
			}

		} else {

			// Create all dirs
			err = os.MkdirAll(fileDir, os.ModeDir|os.ModePerm)
			if err != nil {
				return nil, nil, err
			}

			// Create new file
			file, err = os.Create(filePath)
			if err != nil {
				return nil, nil, err
			}

			// Preallocate space
			err = file.Truncate(int64(f.Length))
			if err != nil {
				return nil, nil, err
			}
		}

		// Calculate continuous length
		l += f.Length
		files = append(files, file)
		lens = append(lens, l)
	}

	return files, lens, nil
}

func (da DiskAccess) loop() {
	//	defer da.onExit

	for {
		select {
		case ioOp := <-da.in:

			var res DiskOp2Result
			var err error

			// Perform io op
			switch msg := ioOp.(type) {
			case *ReadOp2:
				res, err = da.onReadMessage(msg)
			case *WriteOp2:
				res, err = da.onWriteMessage(msg)
			}

			// Check for error
			if err != nil {

				// TODO: panic? inform coordinator?
				log.Printf("Disk Error: %v\n", err)
				break
			}
			da.out <- res
		}
	}
}

func (da DiskAccess) onReadMessage(drm *ReadOp2) (DiskOp2Result, error) {
	buf := make([]byte, 0, drm.length)
	err := da.onIO(buf, drm.index, drm.begin, onReadBlock)
	if err != nil {
		return nil, err
	}
	return &DiskReadResult{drm.Id(), Block(drm.index, drm.begin, buf)}, nil
}

func (da DiskAccess) onWriteMessage(drm *WriteOp2) (DiskOp2Result, error) {
	err := da.onIO(drm.block, drm.index, drm.begin, onWriteBlock)
	if err != nil {
		return nil, err
	}
	return &DiskWriteResult{drm.Id(), drm.index, drm.begin, uint32(len(drm.block))}, nil
}

func (da DiskAccess) onIO(buf []byte,
	index, begin uint32,
	ioFn func(*os.File, []byte, uint64) (int, error)) error {

	start := (uint64(index) * uint64(da.mi.PieceLength)) + uint64(begin)
	var bufOff uint64 = 0

	for i, len := range da.lens {
		if start <= len {

			// Calculate offset & perform I/O
			off := (start + bufOff) - da.mi.Files[i].Length
			n, err := ioFn(da.files[i], buf[bufOff:], off)

			// Reached end of file - move to next & continue
			if err == io.EOF {
				bufOff += uint64(n)
				continue
			}

			// I/O error
			if err != io.EOF {
				return err
			}

			// No error - finished
			return nil
		}
	}

	return nil
}

func (da DiskAccess) onExit() {
	for _, f := range da.files {
		err := f.Close()
		if err != nil {
			da.log.Printf("Disk File Close Err: %v\n", err)
		}
	}
}

func onReadBlock(f *os.File, buf []byte, off uint64) (int, error) {
	return f.ReadAt(buf, int64(off))
}

func onWriteBlock(f *os.File, buf []byte, off uint64) (int, error) {
	return f.WriteAt(buf, int64(off))
}
