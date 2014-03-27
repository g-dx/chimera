package bittorrent

import (
	"runtime"
	"errors"
	"fmt"
)

const (
	SHA1_LENGTH = 20
)

type MetaInfo struct {
	Announce string
	CreationDate int64
	Comment string
	CreatedBy string
	Encoding string
	PieceLength int64
	Hashes [][]byte
	Private bool
	Files []MetaInfoFile
	InfoHash []byte
}

type MetaInfoFile struct {
	Path string
	Name string
	Length int64
	CheckSum []byte
}

func NewMetaInfo(entries map[string] interface {}) (mi *MetaInfo, err error) {

	// Recover from any decoding panics & return error
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				panic(r)
			}
			err = r.(error)
		}
	}()

	// Build meta info
	mi = &MetaInfo {
		Announce:	  bs(entries, "announce"),
		CreationDate: optI(entries, "creation date"),
		Comment:	  optBs(entries, "comment"),
		CreatedBy:	  optBs(entries, "created by"),
		Encoding:	  optBs(entries, "encoding"),
		PieceLength:  i(d(entries, "info"), "piece length"),
		Hashes:       toSha1Hashes(bs(d(entries, "info"), "pieces")),
		Private:	  optI(d(entries, "info"), "private") != 0,
		Files:		  toMetaInfoFiles(d(entries, "info")),
		InfoHash:	  []byte(bs(entries, "info_hash")),
	}

	return mi, nil
}

func toMetaInfoFiles(info map[string]interface{}) []MetaInfoFile {

	name := bs(info, "name")
	files := optL(info, "files")

	// Single-file mode
	if files == nil {
		return []MetaInfoFile{
			MetaInfoFile {
				Path:	  "/",
				Name:	  name,
				Length:	  i(info, "length"),
				CheckSum: []byte(optBs(info, "md5sum")),
			}}
	}

	// Multi-file mode
	miFiles := make([]MetaInfoFile, 0, len(files))
	for _, entry := range files {

		// All entries in files list are dictionarys which describe each file
		miFile := entry.(map[string]interface{})
		path := l(miFile, "path")
		miFiles = append(miFiles,
			MetaInfoFile {
				Path:	  name + "/" + joinStrings(path[:len(path)-1], "/"),
				Name:	  path[len(path)-1].(string),
				Length:	  i(miFile, "length"),
				CheckSum: []byte(optBs(miFile, "md5sum")),
			})
	}

	return miFiles
}

func toSha1Hashes(pieces string) [][]byte {

	// Check format/length
	if len(pieces) % SHA1_LENGTH != 0 {
		panic(errors.New(fmt.Sprintf("pieces value is malformed.")))
	}

	hashes := make([][]byte, 0, len(pieces)/SHA1_LENGTH)
	buf := []byte(pieces)
	for len(buf) != 0 {
		hashes = append(hashes, buf[:SHA1_LENGTH])
		buf = buf[SHA1_LENGTH:]
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