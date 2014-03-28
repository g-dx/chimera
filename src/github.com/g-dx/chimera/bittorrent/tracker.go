package bittorrent

import (
	"time"
	"strings"
	"net/url"
	"github.com/g-dx/chimera/bencode"
	"strconv"
	"errors"
	"net/http"
)

// Request & response dictionary keys
// TODO: Consider consolidating all of these in one file
const (
	//	infoHash    = "info_hash"
	numWanted   = "numwant"
	peerId      = "peer_id"
	minInterval = "min interval"
	interval    = "interval"
	failure     = "failure"
)

type TrackerRequest struct {
	Url       string
	InfoHash  []byte
	PeerId    string
	NumWanted int64
}

type TrackerResponse struct {
	Interval    int64
	MinInterval int64
	PeerAddresses []PeerAddress
}

type PeerAddress struct {
	Id string
	Ip string
	Port uint
}

func QueryTracker(req *TrackerRequest, timeout time.Duration) (*TrackerResponse, error) {

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
		Interval 		: i(bdata, interval),
		MinInterval 	: i(bdata, minInterval),
		PeerAddresses 	: toPeerAddresses(bdata["peers"]),
	}, nil

}

func buildUrl(req *TrackerRequest) string {

	// Collect params
	params := make(map[string] string)
	params[infoHash] = string(req.InfoHash)
	params[numWanted] = strconv.FormatInt(req.NumWanted, 10)
	params[peerId] = req.PeerId

	// Join params
	pairs := make([]string, 0, len(params))
	for k, v := range params {
		pairs = append(pairs, k + "=" + url.QueryEscape(v))
	}

	return req.Url + "?" + strings.Join(pairs, "&")
}

func toPeerAddresses(v interface {}) []PeerAddress {

	var peers
	switch val:= v.(type) {

	// Binary model
	case string:

		if len(val) % 6 != 0 {
			panic(errors.New("peers value value is malformed."))
		}

		// Chop up list
		peers = make([]PeerAddress, 0, len(val)/6)
		for buf := []byte(val); len(buf) != 0; buf = buf[6:] {
			peers = append(peers, PeerAddress{
					Id : "unknown",
					Ip : string(buf[:4]),
					Port : uint(buf[4:6]),
				})
		}

	// Dictionary model
	case []map[string]interface{}:

		peers = make([]PeerAddress, 0, len(val))
		for dict, _ := range val {
			peers = append(peers, PeerAddress {
					Id : bs("peer id", val),
					Ip : bs("ip", val),
					Port: i("port" , val),
				})
		}
	default:
		panic(errors.New("Unknown type of peers value."))
	}

	return peers
}
