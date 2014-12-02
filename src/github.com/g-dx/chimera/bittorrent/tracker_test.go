package bittorrent

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUrlBuilder(t *testing.T) {

	// Test basic concatenation
	url := make(urlBuilder).Add("key1", "value1").Build("http://www.example.com/path1/path2")
	stringEquals(t, "http://www.example.com/path1/path2?key1=value1", url)

	// Test leading slash
	url = make(urlBuilder).Add("key1", "value1").Build("http://www.example.com/")
	stringEquals(t, "http://www.example.com?key1=value1", url)

	// Test URL escaping works correctly
	url = make(urlBuilder).Add("kéy1", "value 1").Build("http://www.example.com/")
	stringEquals(t, "http://www.example.com?k%C3%A9y1=value+1", url)

}

func TestTrackerFailure(t *testing.T) {

	// Configure mock tracker
	tk := tracker(func() string {
		return "d14:failure reason15:No reason givene"
	})
	defer tk.Close()

	// Get response
	_, err := QueryTracker(request(tk.URL))
	errEquals(t, errors.New("No reason given"), err)
}

func TestDictionaryModel(t *testing.T) {

	// Configure mock tracker
	tk := tracker(func() string {
		return "d8:intervali1800e5:peersld2:ip13:67.188.136.267:peer id20:-TR2820-9xt2vdu8kjxp4:porti51413eeee"
	})
	defer tk.Close()

	// Get response
	resp, err := QueryTracker(request(tk.URL))
	isNil(t, err)

	// Check contents
	uintEquals(t, 1800, resp.Interval)
	intEquals(t, 1, len(resp.PeerAddresses))
	addr := resp.PeerAddresses[0]
	stringEquals(t, "67.188.136.26:51413", addr.GetIpAndPort())
	stringEquals(t, "-TR2820-9xt2vdu8kjxp", addr.Id)
}

func TestBinaryModel(t *testing.T) {

	// Configure mock tracker
	tk := tracker(func() string {
		return "d8:intervali60e5:peers12:\x01\x02\x03\x04\x00\xFF\x05\x06\x07\x08\x10\x00e"
	})
	defer tk.Close()

	// Get response
	resp, err := QueryTracker(request(tk.URL))
	isNil(t, err)

	// Check contents
	uintEquals(t, 60, resp.Interval)
	intEquals(t, 2, len(resp.PeerAddresses))
	addr := resp.PeerAddresses[0]
	stringEquals(t, "1.2.3.4:255", addr.GetIpAndPort())
	stringEquals(t, "unknown", addr.Id)
	addr = resp.PeerAddresses[1]
	stringEquals(t, "5.6.7.8:4096", addr.GetIpAndPort())
	stringEquals(t, "unknown", addr.Id)
}

func tracker(resp func() string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, resp())
	}))
}

func request(url string) *TrackerRequest {

	return &TrackerRequest{
		Url:       url,
		InfoHash:  []byte{},
		NumWanted: 10,
		Left:      0,
		Port:      6881,
	}
}
