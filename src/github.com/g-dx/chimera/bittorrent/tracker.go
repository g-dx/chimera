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
	numWanted   = "numWanted"
	peerId      = "peerId"
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
		Interval : i(bdata, interval),
		MinInterval : i(bdata, minInterval),
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
