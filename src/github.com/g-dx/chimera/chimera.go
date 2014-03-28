package main

import (
	"fmt"
	"os"
	"runtime/pprof"
	"io/ioutil"
	"bytes"
	"time"
	"encoding/json"
	"github.com/g-dx/chimera/bittorrent"
)

type test struct {
	Counter int
}

func main() {

	cpu, err := os.Create("/Users/Dakeyras/Desktop/bencode.cpuprofile")
	if err != nil {
		fmt.Println(err)
		return
	}
	mem, err := os.Create("/Users/Dakeyras/Desktop/bencode.memprofile")
	if err != nil {
		fmt.Println(err)
		return
	}

	pprof.StartCPUProfile(cpu)
	pprof.WriteHeapProfile(mem)
	defer pprof.StopCPUProfile()
	defer cpu.Close()
	defer mem.Close()

	now := time.Now()
	buf, err := ioutil.ReadFile("/Users/Dakeyras/Downloads/The Chris Gethard Show - Episodes 1 - 120   Specials.torrent")
	if err != nil {
		fmt.Println(err)
	}

	metaInfo, err := bittorrent.NewMetaInfo(bytes.NewReader(buf))
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	fmt.Printf("Duration: %s\n", time.Since(now))
	_, err = json.MarshalIndent(metaInfo, "", " ")
	fmt.Printf("Decoded Data:\n%v\n", "skipped")

	// Create request
	req := &bittorrent.TrackerRequest{
		Url : metaInfo.Announce,
		InfoHash : metaInfo.InfoHash,
		PeerId : "Chimera",
		NumWanted : 10,
	}

	reqJson, err := json.MarshalIndent(req, "", " ")
	fmt.Printf("Params:%v\n", reqJson)

	resp, err := bittorrent.QueryTracker(req)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}
	fmt.Println("Url: ", resp)

	// Connect to peers


//	_, err = bittorrent.Request(url)
//	if err != nil {
//		fmt.Println("Error: ", err)
//	}
}
