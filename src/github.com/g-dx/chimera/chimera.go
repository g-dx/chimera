package main

import (
	"fmt"
	"os"
	"runtime/pprof"
	"io/ioutil"
	"bytes"
	"time"
//	"encoding/json"
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

	data, err := Decode(bytes.NewReader(buf))
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	fmt.Printf("Duration: %s\n", time.Since(now))
//	p, err := json.MarshalIndent(data, "", " ")
//	fmt.Printf("Decoded Data:\n%v", string(p))

	metaInfo, err := NewMetaInfo(data.(map[string] interface {}))
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}
	fmt.Println("", metaInfo)
}

