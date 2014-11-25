package bittorrent

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	intEquals(t, 524288, int64(metaInfo.PieceLength))
	byteEquals(t,
		[]byte{77, 15, 92, 159, 158, 96, 107, 203, 24, 8, 187, 51, 227, 103, 148, 219, 158, 132, 7, 227},
		metaInfo.InfoHash)
}

func byteEquals(t *testing.T, a, b []byte) {
	if ok := bytes.Equal(a, b); !ok {
		unequalValue(t, a, b)
	}
}

func stringEquals(t *testing.T, a, b string) {
	if a != b {
		unequalValue(t, a, b)
	}
}

func intEquals(t *testing.T, a, b int64) {
	if a != b {
		unequalValue(t, a, b)
	}
}

func msgEquals(t *testing.T, a, b ProtocolMessage) {

	if a.PeerId() != b.PeerId() {
		unequalValue(t, a.PeerId(), b.PeerId())
	}

	sa := ToString(a)
	sb := ToString(b)
	if sa != sb {
		unequalValue(t, sa, sb)
	}
}

func errEquals(t *testing.T, a, b error) {
	if a.Error() != b.Error() {
		unequalValue(t, a, b)
	}
}

func isNil(t *testing.T, a interface{}) {
	if a != nil {
		t.Fatalf(buildUnequalMessage(nil, a))
	}
}

// Do not call this from outside this package!
func unequalValue(t *testing.T, a, b interface{}) {
	t.Errorf(buildUnequalMessage(a, b))
}

// Do not call this from outside this package!
func buildUnequalMessage(a, b interface{}) string {
	_, file, line, _ := runtime.Caller(3)
	return fmt.Sprintf("\nFile    : %v:%v\nExpected: %v\nActual  : %v",
		file[strings.LastIndex(file, "/")+1:len(file)], line, a, b)
}
