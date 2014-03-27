package bittorrent

import (
	"net"
	"time"
	"strings"
	"net/url"
	"github.com/g-dx/chimera/bencode"
)

type TrackerParameters struct {
	Params map[string] string
}

func (t *TrackerParameters) InfoHash(hash []byte) {
	t.Params["info_hash"] = string(hash)
}

func (t *TrackerParameters) PeerId(id string) {
	t.Params["peerid"] = id
}
func (t *TrackerParameters) NumWanted(num int) {
	t.Params["numwanted"] = string(num)
}

type Response struct {
	Interval int64
	MinInterval int64
}

func Tracker() {

	go run()
}

func run() {

}

func Request(requestUrl string) (*Response, error) {

	// Connect
	conn, err := net.Dial("tcp", requestUrl)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Read response & parse contents
	conn.SetReadDeadline(time.Now().Add(time.Second * 10)) // timeout
	data, err := bencode.Decode(conn)
	if err != nil {
		return nil, err
	}

	// Check for failure
	failure := optBs(data.(map[string] interface {}), "failure")
	if len(failure) > 0 {
		return nil, err
	}

	return &Response{
		Interval : i(data.(map[string] interface {}), "interval"),
		MinInterval : optI(data.(map[string] interface {}), "min interval"),
	}, nil
}

func BuildRequestUrl(uri string, tp TrackerParameters) string {

	keysAndValues := make([]string, 0, len(tp.Params))
	for k, v := range tp.Params {
		keysAndValues = append(keysAndValues, k + "=" + url.QueryEscape(v))
	}

	return uri + "?" + strings.Join(keysAndValues, "&")
}
