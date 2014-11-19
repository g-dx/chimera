package bittorrent

import (
	"path/filepath"
	"io/ioutil"
	"testing"
	"bytes"
)

var torrentData []byte

func init() {

	// Assert file exists
	f, err := filepath.Abs("./testdata/CentOS-6.5-x86_64-bin-DVD1to2.torrent")
	if err != nil {
		panic(err)
	}

	torrentData, err = ioutil.ReadFile(f)
	if err != nil {
		panic(err)
	}
}

func TestNewMetaInfo(t *testing.T) {

	// Load file
	metaInfo, err := NewMetaInfo(bytes.NewReader(torrentData))
	if err != nil {
		t.Fatalf("Failed to load metainfo: %v", err)
	}

	// Double check contents
	stringEquals(t, "http://torrent.centos.org:6969/announce", metaInfo.Announce)
	stringEquals(t, "CentOS-6.5-x86_64-bin-DVD1to2", metaInfo.Comment)
	stringEquals(t, "mktorrent 1.0", metaInfo.CreatedBy)
	intEquals(t, 1385853584, int64(metaInfo.CreationDate))
	intEquals(t, 52428, int64(metaInfo.PieceLength))
	byteEquals(t, []byte("Fill me in!"), metaInfo.InfoHash)
}

func unequalValue(t *testing.T, a, b interface {}) {
	t.Errorf("Expected: (%v), Actual: (%v)", a, b)
}

func byteEquals(t *testing.T, a []byte, b []byte) {
	if ok := bytes.Equal(a, b); !ok {
		unequalValue(t, a, b)
	}
}

func stringEquals(t *testing.T, a string, b string) {
	if a != b {
		unequalValue(t, a, b)
	}
}

func intEquals(t *testing.T, a int64, b int64) {
	if a != b {
		unequalValue(t, a, b)
	}
}