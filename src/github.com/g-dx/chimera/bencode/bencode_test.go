package bencode

import (
	"bytes"
	"io/ioutil"
	"path/filepath"
	"testing"
)

var (
	torrentData []byte
)

func init() {

	// Assert file exists
	f, err := filepath.Abs("../bittorrent/testdata/CentOS-6.5-x86_64-bin-DVD1to2.torrent")
	if err != nil {
		panic(err)
	}

	torrentData, err = ioutil.ReadFile(f)
	if err != nil {
		panic(err)
	}
}

func BenchmarkDecodeBigTorrent(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Decode(bytes.NewReader(torrentData))
	}
}
