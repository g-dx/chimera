package main

import (
	"fmt"
	"os"
	"runtime/pprof"
	"io/ioutil"
	"bytes"
	"github.com/g-dx/chimera/bittorrent"
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

	resp, err := bittorrent.QueryTracker(req)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}
	fmt.Printf("Tracker Respose: %+v\n ", resp)


//	buffer := new(bytes.Buffer)
//	bittorrent.Marshal(buffer, bittorrent.Choke)
//	fmt.Printf("Choke:%v\n", buffer.Bytes())
//	buffer.Reset()
//
//	bittorrent.Marshal(buffer, bittorrent.Unchoke)
//	fmt.Printf("Unchoke:%v\n", buffer.Bytes())
//	buffer.Reset()
//
//	bittorrent.Marshal(buffer, bittorrent.Interested)
//	fmt.Printf("Interested:%v\n", buffer.Bytes())
//	buffer.Reset()
//
//	bittorrent.Marshal(buffer, bittorrent.Uninterested)
//	fmt.Printf("Uninterested:%v\n", buffer.Bytes())
//	buffer.Reset()
//
//	bittorrent.Marshal(buffer, bittorrent.Have(len(buf)))
//	fmt.Printf("Have:%v\n", buffer.Bytes())
//	buffer.Reset()
//
//	bittorrent.Marshal(buffer, bittorrent.Bitfield([]byte {0xFF, 0xAA, 0x34, 0x99, 0xDC}))
//	fmt.Printf("Bitfield:%v\n", buffer.Bytes())
//	buffer.Reset()
//
//	bittorrent.Marshal(buffer, bittorrent.Request(23, 90, 12))
//	fmt.Printf("Request:%v\n", buffer.Bytes())
//	buffer.Reset()
//
//	bittorrent.Marshal(buffer, bittorrent.Piece(23, 90, []byte {11, 34, 123, 45, 90}))
//	fmt.Printf("Piece:%v\n", buffer.Bytes())
//	buffer.Reset()
//
//	bittorrent.Marshal(buffer, bittorrent.Cancel(23, 90, 12))
//	fmt.Printf("Cancel:%v\n", buffer.Bytes())
//	buffer.Reset()

	in := make(chan bittorrent.ProtocolMessage)
	out := make(chan bittorrent.ProtocolMessage)
	e := make(chan error)
	outHandshake := bittorrent.Handshake(metaInfo.InfoHash)

	// Connect
	var c *bittorrent.PeerConnection = nil
	for _, pa := range resp.PeerAddresses[4:] {

		c, err = bittorrent.NewConnection(pa.GetIpAndPort())
		if err != nil {
			fmt.Println("Error: ", err)
			continue
		}
		c.Establish(in, out, e, outHandshake)
		break;
	}

	// Create empty bitfield message
//	bits := len(metaInfo.Hashes)/8
//	fmt.Printf("Bits: %v\n", bits)
//	if len(metaInfo.Hashes) % 8 != 0 {
//		bits++
//	}
//	in <-bittorrent.Bitfield(make([]byte, bits))
	for _ = range out {
		// Discard messages...for now!
//		if msg == bittorrent.Unchoke {
//			in <- bittorrent.Request(0, 0, )
//		}
	}
}
