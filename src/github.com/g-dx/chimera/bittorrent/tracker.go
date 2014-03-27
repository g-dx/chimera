package bittorrent

import (
	"net"
	"time"
	"io/ioutil"
)

type Response struct {

}

func Request(url string, port int, numWanted int) Response {

	// Connect
	conn, err := net.Dial("tcp", buildUrl())
	if err != nil {
		// TODO: Return empty response?
	}
	defer conn.Close()

	// Read response
	conn.SetReadDeadline(time.Now().Add(time.Second * 10)) // timeout
	_, err = ioutil.ReadAll(conn)
	if err != nil {
		// TODO: Return empty response?
	}

	// Parse response
	return Response{}
}

func buildUrl() string {
	return ""
}
