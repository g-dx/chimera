package bittorrent

import (
	"runtime"
	"errors"
	"io"
	"github.com/g-dx/chimera/bencode"
)

const (
	sha1Length = 20
)

// Meta-info dictionary keys
const (
	announce     = "announce"
	creationDate = "creation date"
	comment      = "comment"
	createdBy    = "created by"
	encoding     = "encoding"
	pieceLength  = "piece length"
	info         = "info"
	pieces       = "pieces"
	private      = "private"
	infoHash     = "info_hash"
	name         = "name"
	files        = "files"
	length       = "length"
	md5sum       = "md5sum"
	path         = "path"
)

// Errors
var (
	errPiecesValueMalformed = errors.New("pieces value is malformed.")
)

type MetaInfo struct {
	Announce     string
	CreationDate int64
	Comment      string
	CreatedBy    string
	Encoding     string
	PieceLength  int64
	Hashes       [][]byte
	Private      bool
	Files        []MetaInfoFile
	InfoHash     []byte
}

type MetaInfoFile struct {
	Path     string
	Name     string
	Length   int64
	CheckSum []byte
}

func (mi *MetaInfo) TotalLength() int64 {
	var totalLength int64
	for i := range mi.Files {
		totalLength += mi.Files[i].Length
	}
	return totalLength
}

func NewMetaInfo(r io.Reader) (mi *MetaInfo, err error) {

	// Recover from any decoding panics & return error
	// TODO: Check if there is way push this into the bencode.go so it
	//       can be usesd by anyone who is doing bencoding
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				panic(r)
			}
			err = r.(error)
		}
	}()

	// Decode
	bdata, err := bencode.DecodeAsDict(r)
	if err != nil {
		return nil, err
	}
	
	// Build meta info
	mi = &MetaInfo {
		Announce:      	bs(bdata, announce),
		CreationDate:	optI(bdata, creationDate),
		Comment:      	optBs(bdata, comment),
		CreatedBy:      optBs(bdata, createdBy),
		Encoding:		optBs(bdata, encoding),
		PieceLength:	i(d(bdata, info), pieceLength),
		Hashes:			toSha1Hashes(bs(d(bdata, info), pieces)),
		Private:		optI(d(bdata, info), private) != 0,
		Files:			toMetaInfoFiles(d(bdata, info)),
		InfoHash:		[]byte(bs(bdata, infoHash)),
	}

	return mi, nil
}

func toMetaInfoFiles(info map[string]interface{}) []MetaInfoFile {

	name := bs(info, name)
	files := optL(info, files)

	// Single-file mode
	if files == nil {
		return []MetaInfoFile{
			MetaInfoFile {
				Path:      "/",
				Name:      name,
				Length:    i(info, length),
				CheckSum: []byte(optBs(info, md5sum)),
			}}
	}

	// Multi-file mode
	miFiles := make([]MetaInfoFile, 0, len(files))
	for _, entry := range files {

		// All bdata in files list are dictionarys which describe each file
		miFile := entry.(map[string]interface{})
		path := l(miFile, path)
		miFiles = append(miFiles,
			MetaInfoFile {
				Path:      name+"/"+joinStrings(path[:len(path)-1], "/"),
				Name:      path[len(path)-1].(string),
				Length:    i(miFile, length),
				CheckSum: []byte(optBs(miFile, md5sum)),
			})
	}

	return miFiles
}

func toSha1Hashes(pieces string) [][]byte {

	// Check format/length
	if len(pieces) % sha1Length != 0 {
		panic(errPiecesValueMalformed)
	}

	hashes := make([][]byte, 0, len(pieces)/sha1Length)
	buf := []byte(pieces)
	for len(buf) != 0 {
		hashes = append(hashes, buf[:sha1Length])
		buf = buf[sha1Length:]
	}
	return hashes
}

func joinStrings(list []interface {}, separator string) string {
	buf := ""
	for _, s := range list {
		buf += s.(string) + separator
	}

	return buf
}
