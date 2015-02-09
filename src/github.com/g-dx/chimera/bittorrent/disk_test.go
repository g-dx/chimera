package bittorrent

import (
	"testing"
	"os"
	"log"
	"crypto/sha1"
	"bytes"
	"math"
)

func TestDiskReadAndWrite(t *testing.T) {

	miFiles := []MetaInfoFile{
		MetaInfoFile{"dir1", "file1.txt", 257, []byte("")},
		MetaInfoFile{"dir2", "file2.txt", 1048, []byte("")},
		MetaInfoFile{"dir2", "file3.txt", 10, []byte("")},
		MetaInfoFile{"dir2", "file4.txt", 89, []byte("")},
		MetaInfoFile{"dir3", "file5.txt", 1, []byte("")},
		MetaInfoFile{"dir3", "file6.txt", 3, []byte("")},
		MetaInfoFile{"", "file7.txt", 2300, []byte("")},
	}

	// Create test files
	files, err := CreateOrRead(miFiles, "/Users/Dakeyras/Desktop/Test/")
	if err != nil {
		t.Fatalf("Failed to initialise files: %v", err)
	}
	defer os.RemoveAll("/Users/Dakeyras/Desktop/Test/")

	logger := log.New(os.Stdout, "", log.LstdFlags)

	// Create layered IO
	blockSize := 32
	pieceSize := int64(256)

	// Create data & hashes
	data, hashes, downloadSize := createTestData(miFiles, int(pieceSize))

	// Write data into files
	prevLen := int64(0)
	for _, f := range files {
		f.WriteAt(data[int(prevLen) : int(prevLen + f.len)], 0)
		prevLen += f.len
	}

	// I/O
	layout := NewDiskLayout(files, uint32(pieceSize), uint64(downloadSize))
	diskio := NewDiskIO(layout, logger)

	// Read back contents and check hash
	for i := 0 ; i < len(hashes) ; i++ {

		// Check for last piece
		bufSize := pieceSize
		if i == len(hashes)-1 && downloadSize%pieceSize != 0 {
			bufSize = downloadSize%pieceSize
		}
		buf := make([]byte, bufSize)

		// Read each block
		for j := 0 ; j < int(math.Ceil(float64(len(buf)) / float64(blockSize))); j++ {
			off := j*blockSize
			n := blockSize

			// Check for last block
			if off + n > len(buf) {
				n = len(buf) - off
			}

			// Read block
			err := diskio.ReadAt(buf[off:off+n], i, off)
			if err != nil {
				log.Fatal(err)
			}
		}

		// Assert hash on read
		hash := sha1.New()
		hash.Write(buf)
		h := hash.Sum(nil)

		if !bytes.Equal(h, hashes[i]) {
			log.Fatalf("Piece [%v]\nLength [%v]\nFailed Hash!\nData %v\n\nExpected Hash % X\nActual Hash   % X", i, len(buf), buf, hashes[i], h)
		}
	}
}

type NullIO struct {}
func (nio * NullIO) WriteAt(p []byte, index, off int) error { return nil }
func (nio * NullIO) ReadAt(p []byte, index, off int) error { return nil }
func (nio * NullIO) Close() error { return nil }

// TODO: Fix me!
func CacheWrite(t *testing.T) {

	miFiles := []MetaInfoFile{
		MetaInfoFile{"dir1", "file1.txt", 257, []byte("")},
		MetaInfoFile{"dir2", "file2.txt", 1048, []byte("")},
		MetaInfoFile{"dir2", "file3.txt", 10, []byte("")},
		MetaInfoFile{"dir2", "file4.txt", 89, []byte("")},
		MetaInfoFile{"dir3", "file5.txt", 1, []byte("")},
		MetaInfoFile{"dir3", "file6.txt", 3, []byte("")},
		MetaInfoFile{"", "file7.txt", 2300, []byte("")},
	}

	// Create test files
	files, err := CreateOrRead(miFiles, "/Users/Dakeyras/Desktop/Test/")
	if err != nil {
		t.Fatalf("Failed to initialise files: %v", err)
	}
	defer os.RemoveAll("/Users/Dakeyras/Desktop/Test/")

	// Create layered IO
	blockSize := 32
	pieceSize := int64(128)
	data, hashes, downloadSize := createTestData(miFiles, int(pieceSize))
	layout := NewDiskLayout(files, uint32(pieceSize), uint64(downloadSize))

	// Create buffered channel for test purposes...
	diskOps := make(chan DiskMessageResult, 1)
	cacheIo := NewCacheIO(layout, hashes, blockSize, diskOps, &NullIO{})

	// Write block
	for i := 0 ; i < len(hashes) ; i++ {

		// Check for last piece
		bufSize := pieceSize
		if i == len(hashes)-1 && downloadSize%pieceSize != 0 {
			bufSize = downloadSize%pieceSize
		}

		// Grab piece data
		buf := data[i*int(pieceSize) : i*int(pieceSize) + int(bufSize)]

		// Read each block
		for j := 0 ; j < int(math.Ceil(float64(len(buf)) / float64(blockSize))); j++ {
			off := j * blockSize
			n := blockSize

			// Check for last block
			if off+n > len(buf) {
				n = len(buf)-off
			}

			// Read block
			err := cacheIo.WriteAt(buf[off:off+n], i, off)
			if err != nil {
				log.Fatal(err)
			}
		}

		// Check we have received an Ok
		r := <- diskOps
		switch result := r.(type) {
		case PieceOk:
			if int(result) != i {
				t.Errorf("Expected: %v, Actual: %v", i, result)
			}
		default:
			t.Fatalf("Unexpected disk result: %v", result)
		}
	}
}

func TestDisk(t *testing.T) {
	// Create buffered channel for test purposes...
	diskOps := make(chan DiskMessageResult, 1)
	disk := NewDisk(&NullIO{}, diskOps)
	defer disk.Close()
	id := &PeerIdentity{[20]byte{}, "id"}

	// Test a read

	disk.Read(id, Block{19, 256, make([]byte, _16KB)})
	r := <- diskOps
	switch result := r.(type) {
	case ReadOk:
		if result.id != id {
			t.Errorf("Expected: %v, Actual: %v", id, result.id)
		}
		if result.block.index != 19 {
			t.Errorf("Expected: %v, Actual: %v", 19, result.block.index)
		}
		if result.block.begin != 256 {
			t.Errorf("Expected: %v, Actual: %v", 256, result.block.begin)
		}
	default:
		t.Fatalf("Unexpected disk result: %v", result)
	}

	// Test a write

	disk.Write(Block{8, 0, make([]byte, _16KB)})
	r = <- diskOps
	switch result := r.(type) {
	case WriteOk:
		if int(result) != 8 {
			t.Errorf("Expected: %v, Actual: %v", 8, result)
		}

	default:
		t.Fatalf("Unexpected disk result: %v", result)
	}
}

func createTestData(miFiles []MetaInfoFile, pieceSize int) (data []byte, hashes [][]byte, downloadSize int64) {

	for _, f := range miFiles {
		downloadSize += int64(f.Length)
	}

	data = make([]byte, downloadSize)
	hashes = make([][]byte, int(math.Ceil(float64(downloadSize) / float64(pieceSize))))

	written := 0
	piece := 0
	for i := 0 ; i <= len(data) ; i++ {
		if i != 0 && (i % pieceSize == 0 || i == len(data)) {

			// Hash piece
			h := sha1.New()
			h.Write(data[piece*pieceSize:(piece*pieceSize)+written])
			hashes[piece] = h.Sum(nil)

			// Are we done?
			if i == len(data) {
				break
			}

			// Move to next piece & reset written count
			piece++
			written = 0
		}
		data[i] = byte(piece % 256)
		written++
	}
	return data, hashes, downloadSize
}
