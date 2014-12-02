package bittorrent

import (
	"errors"
	"fmt"
	"github.com/g-dx/chimera/bencode"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
)

// Request & response dictionary keys
// TODO: Consider consolidating all of these in one file
const (
	//	infoHash    = "info_hash"
	numWanted   = "numwant"
	peerId      = "peer_id"
	minInterval = "min interval"
	interval    = "interval"
	failure     = "failure reason"
	left        = "left"
	peers       = "peers"
	port        = "port"
)

type TrackerRequest struct {
	Url       string
	InfoHash  []byte
	NumWanted uint
	Left      uint64 // Must be 64-bit for large files
	Port      uint
}

type TrackerResponse struct {
	Interval, MinInterval uint
	PeerAddresses         []PeerAddress
}

type PeerAddress struct {
	Id, Ip string
	Port   uint
}

func QueryTracker(req *TrackerRequest) (trp *TrackerResponse, err error) {

	// Recover from any decoding panics & return error
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				panic(r)
			}
			err = r.(error)
		}
	}()

	// Build url & GET
	resp, err := http.Get(buildUrl(req))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Parse response
	bdata, err := bencode.DecodeAsDict(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check for failure
	failure := optBs(bdata, failure)
	if len(failure) > 0 {
		return nil, errors.New(failure)
	}

	// Parse response
	return &TrackerResponse{
		Interval:      uint(i(bdata, interval)),
		MinInterval:   uint(optI(bdata, minInterval)),
		PeerAddresses: toPeerAddresses(bdata[peers]),
	}, err

}

type urlBuilder map[string]string

func (b urlBuilder) Add(key string, val string) urlBuilder {
	b[key] = val
	return b
}

func (b urlBuilder) Build(base string) string {

	// Escape & join params
	pairs := make([]string, 0, len(b))
	for k, v := range b {
		pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(v))
	}

	// Trim leading slash if present
	if strings.HasSuffix(base, "/") {
		base = base[0 : len(base)-1]
	}

	return base + "?" + strings.Join(pairs, "&")
}

func buildUrl(req *TrackerRequest) string {

	builder := make(urlBuilder)
	builder.Add(infoHash, string(req.InfoHash))
	builder.Add(numWanted, strconv.FormatUint(uint64(req.NumWanted), 10))
	builder.Add(peerId, string(PeerId))
	builder.Add(left, strconv.FormatUint(req.Left, 10))
	return builder.Build(req.Url)
}

func toPeerAddresses(v interface{}) []PeerAddress {

	var peers []PeerAddress
	switch val := v.(type) {

	// Binary model
	case string:

		if len(val)%6 != 0 {
			panic(errors.New("peers value value is malformed."))
		}

		// Chop up list
		peers = make([]PeerAddress, 0, len(val)/6)
		for buf := []byte(val); len(buf) != 0; buf = buf[6:] {
			peers = append(peers, PeerAddress{
				Id:   "unknown",
				Ip:   fmt.Sprintf("%d.%d.%d.%d", buf[0], buf[1], buf[2], buf[3]),
				Port: (uint(buf[4]) << 8 & 0xFF00) + uint(buf[5]),
			})
		}

	// Dictionary model
	case []interface{}:

		peers = make([]PeerAddress, 0, len(val))
		for _, v := range val {

			// Check for map
			var dict map[string]interface{}
			var ok bool
			if dict, ok = v.(map[string]interface{}); !ok {
				panic(errors.New("Unknown type of peers value."))
			}

			peers = append(peers, PeerAddress{
				Id:   bs(dict, "peer id"),
				Ip:   bs(dict, "ip"),
				Port: uint(i(dict, "port")),
			})
		}
	default:
		panic(errors.New("Unknown type of peers value."))
	}

	return peers
}

func (pa PeerAddress) GetIpAndPort() string {
	return fmt.Sprintf("%v:%d", pa.Ip, pa.Port)
}
