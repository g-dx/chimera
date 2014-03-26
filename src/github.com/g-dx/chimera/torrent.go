package main

import (
	"runtime"
	"errors"
	"fmt"
	"strings"
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
		Files:		  toFiles(d(entries, "info")),
		InfoHash:	  []byte(bs(entries, "info_hash")),
	}

	return mi, nil
}

func toFiles(info map[string]interface{}) []MetaInfoFile {

	name := bs(info, "name")

	// Which mode are we in?
	files := optL(info, "files")
	if files == nil {

		// Single-file mode
		return []MetaInfoFile{
			MetaInfoFile {
				Path:	  name,
				Length:	  i(info, "length"),
				CheckSum: []byte(optBs(info, "md5sum")),
			}}
	}

	// Multi-file mode
	infoFiles := make([]MetaInfoFile, 0, len(files))
	for _, f := range files {
		fs := f.(map[string]interface{})
		infoFiles = append(infoFiles,
			MetaInfoFile {
				Path:	  name + strings.Join(l(fs, "path"), "/"),
				Length:	  i(fs, "length"),
				CheckSum: []byte(optBs(fs, "md5sum")),
			})
	}

	return infoFiles
}

func toSha1Hashes(pieces string) [][]byte {
	hashes := make([][]byte, 0, len(pieces)/20)
	buf := []byte(pieces)
	for len(buf) != 0 {
		hashes = append(hashes, buf[:20])
		buf = buf[20:]
	}
	return hashes
}

func bs(entries map[string] interface{}, key string) (string) {

	v, ok := entries[key]
	if !ok {
		panic(errors.New(fmt.Sprintf("String (%v) not found.", key)))
	}
	fmt.Printf("%v => %v", key, v)
	return v.(string)
}

func optBs(entries map[string] interface {}, key string) (string) {

	v, ok := entries[key]
	if !ok {
		return ""
	}
	return v.(string)
}

func optI(entries map[string] interface {}, key string) (int64) {

	v, ok := entries[key]
	if !ok {
		return 0
	}
	return v.(int64)
}

func optL(entries map[string] interface {}, key string) ([]interface {}) {

	v, ok := entries[key]
	if !ok {
		return nil
	}
	return v.([]interface {})
}

func l(entries map[string] interface {}, key string) ([]interface {}) {

	v, ok := entries[key]
	if !ok {
		panic(errors.New(fmt.Sprintf("List (%v) not found.", key)))
	}
	return v.([]interface {})
}

func i(entries map[string] interface {}, key string) (int64) {

	v, ok := entries[key]
	if !ok {
		panic(errors.New(fmt.Sprintf("Integer (%v) not found.", key)))
	}
	return v.(int64)
}

func d(entries map[string] interface {}, key string) (map[string] interface {}) {

	v, ok := entries[key]
	if !ok {
		panic(errors.New(fmt.Sprintf("Dict (%v) not found.", key)))
	}
	return v.(map[string] interface {})
}
