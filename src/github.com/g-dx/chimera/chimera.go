package main

import (
	"fmt"
	"os"
	"runtime/pprof"
	"io/ioutil"
	"bytes"
	"github.com/g-dx/chimera/bittorrent"
	"time"
	"runtime"
)

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

	buf, err := ioutil.ReadFile("/Users/Dakeyras/Downloads/CentOS 6.5 x86_64 bin DVD1to2.torrent")
	if err != nil {
		fmt.Println(err)
	}

	metaInfo, err := bittorrent.NewMetaInfo(bytes.NewReader(buf))
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

//	fmt.Printf("Duration: %s\n", time.Since(now))
//	_, err = json.MarshalIndent(metaInfo, "", " ")
//	fmt.Printf("Decoded Data:\n%v\n", metaInfo)

	// Create request
	req := &bittorrent.TrackerRequest{
		Url : metaInfo.Announce,
		InfoHash : metaInfo.InfoHash,
		NumWanted : 50,
		Left : metaInfo.TotalLength(),
	}

//	reqJson, err := json.MarshalIndent(req, "", " ")
//	fmt.Printf("Params:%v\n", reqJson)

	_, err = bittorrent.QueryTracker(req)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}
	fmt.Printf("Tracker Respose: %+v\n ")

	// Create log directory
	dir := fmt.Sprintf("/Users/Dakeyras/.chimera/%v [...%x]",
		               time.Now().Format("2006-01-02 15.04.05"),
		               metaInfo.InfoHash[15:])
	err = os.Mkdir(dir, os.ModeDir | os.ModePerm)
	if err != nil {
		fmt.Printf("Failed to create torrent dir: %v\n", err)
	}

//	tr := make(chan *bittorrent.TrackerResponse)
//	pc, err := bittorrent.NewPeerCoordinator(metaInfo, dir, tr)
//	if err != nil {
//		fmt.Printf("Failed to create coordinator: %v\n", err)
//		return
//	}
//
//	// Send tracker response
//	tr <- resp
//
//	// Wait until everything is finished
//	pc.AwaitDone()

//	fmt.Printf("No of CPUs: %v\n", runtime.NumCPU())

	dIn := make(chan bittorrent.DiskMessage)
	dOut := make(chan bittorrent.DiskMessageResult)
	_, err = bittorrent.NewDiskAccess(metaInfo, dIn, dOut, dir, nil)
}
