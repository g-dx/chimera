package bencode

import (
	"testing"
	"bytes"
	"io/ioutil"
)

var(
	torrentFile []byte
)

func TestDecodeBigTorrent(t *testing.T) {
	loadTorrentFile(t)
	raw, err := Decode(bytes.NewReader(torrentFile))
	if err != nil {
		t.Error(err)
		return
	}

	// Check type
	data, ok := raw.(map[string] interface {})
	if !ok {
		t.Errorf("Decoded interface is not a map of correct type")
	}

	// Check contents
	v := checkKey(t, data, "info_hash")
	byteEquals(t, []byte("\xbc+B{w1\x89\xfa\xecD\x9a\xa5lC\x8a\xaaRQ\x89b"), v)

	v = checkKey(t, data, "announce")
	stringEquals(t, "http://legittorrents.info:2710/announce", v)

	v = checkKey(t, data, "url-list")

	_, ok = v.([]interface {})
	if !ok {
		t.Errorf("Decoded interface is not a map of correct type")
	}

}

func BenchmarkDecodeBigTorrent(b *testing.B) {
	b.StopTimer()
	r, err := ioutil.ReadFile("./test/CentOS 6.5 x86_64 bin DVD1to2.torrent")
	if err != nil {
		b.Error(err)
		return
	}
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		Decode(bytes.NewReader(r))
	}
}

func checkKey(t *testing.T, data map[string] interface {}, key string) interface {} {
	v, ok := data[key]
	if !ok {
		t.Errorf("Expected key (%v) not found", key)
	}
	return v
}

func unequalValue(t *testing.T, a, b interface {}) {
	t.Error("Expected: (%v), Actual: (%v)", a, b)
}

func byteEquals(t *testing.T, a []byte, b interface {}) {
	if ok := bytes.Equal(a, b.([]byte)); !ok {
		unequalValue(t, a, b)
	}
}

func stringEquals(t *testing.T, a string, b interface {}) {
	if a != b.(string) {
		unequalValue(t, a, b)
	}
}

func loadTorrentFile(t *testing.T) {
	if torrentFile == nil {
		r, err := ioutil.ReadFile("./test/CentOS 6.5 x86_64 bin DVD1to2.torrent")
		if err != nil {
			t.Error(err)
			return
		}
		torrentFile = r
	}
}
