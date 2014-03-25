package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"runtime/pprof"
	"strconv"
	"time"
	sha1Hash "crypto/sha1"
	"encoding/json"
)

// Function map for decoding
var decodeFunctions map[rune]func([]byte) (interface{}, []byte)

const (
	TYPE_TERMINATOR       rune = 'e'
	BYTE_STRING_SEPARATOR rune = ':'
)

func Decode(buf []byte) (v interface{}, err error) {

	// Recover from any decoding panics & return error
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				panic(r)
			}
			err = r.(error)
		}
	}()

	// Decode & check all data processed
	v, remainingBuf := decoderFn(nextRune(buf))(buf)
	if len(remainingBuf) != 0 {
		panic(errors.New("Trailing data detected: " + string(remainingBuf)))
	}
	return v, nil
}

func decodeInteger(buf []byte) (interface{}, []byte) {

	// Check for terminating character
	i := bytes.IndexRune(buf, TYPE_TERMINATOR)
	if i == -1 {
		panic(errors.New("Failed to decode integer as no ending 'e' found"))
	}

	// Convert to signed 64-bit int
	v, _ := strconv.ParseInt(string(buf[1:i]), 10, 64)
	return v, buf[i+1:]
}

func decodeByteString(buf []byte) (interface{}, []byte) {

	// Check for terminating character
	i := bytes.IndexRune(buf, BYTE_STRING_SEPARATOR)
	if i == -1 {
		panic(errors.New("Failed to decode byte string as no terminating ':' found"))
	}

	// Calculate length of string & discard length bytes
	strLen, _ := strconv.ParseUint(string(buf[:i]), 10, 64)
	buf = buf[i+1:]
	return string(buf[:strLen]), buf[strLen:]
}

func decodeList(buf []byte) (interface{}, []byte) {

	list := make([]interface{}, 10)
	var v interface{}

	// Drop leading 'l' and consume until terminating character
	buf = buf[1:]
	for r := nextRune(buf); r != TYPE_TERMINATOR; r = nextRune(buf) {
		v, buf = decoderFn(r)(buf)
		list = append(list, v)
	}

	return list, buf[1:]
}

func decodeDictionary(buf []byte) (interface{}, []byte) {

	dict := make(map[string]interface{})
	var k, v interface{}
	var preValueBuf []byte

	// Drop leading 'd' and consume until terminating character
	buf = buf[1:]
	for r := nextRune(buf); r != TYPE_TERMINATOR; r = nextRune(buf) {
		k, preValueBuf = decodeByteString(buf)
		v, buf = decoderFn(nextRune(preValueBuf))(preValueBuf)
		dict[k.(string)] = v

		// SPECIAL CASE:
		// Any dictionary key named "info" gets a SHA-1 hash of its value added to the result
		if k.(string) == "info" {
			dict["info_hash"] = sha1(preValueBuf[0:len(preValueBuf)-len(buf)])
		}
	}

	return dict, buf[1:]
}

func sha1(buf []byte) []byte {
	hash := sha1Hash.New()
	hash.Write(buf) // Guaranteed not to return an error
	return hash.Sum(nil)
}

func nextRune(buf []byte) rune {
	return rune(buf[0])
}

func decoderFn(r rune) func([]byte) (interface{}, []byte) {
	fn := decodeFunctions[r]
	if fn == nil {
		panic(errors.New(fmt.Sprint("No decoding function found for character: %s", r)))
	}
	return fn
}

// Build function map
func init() {
	decodeFunctions = map[rune]func([]byte) (interface{}, []byte){
		'i': decodeInteger,
		'l': decodeList,
		'd': decodeDictionary,
		'-': decodeByteString,
		'0': decodeByteString,
		'1': decodeByteString,
		'2': decodeByteString,
		'3': decodeByteString,
		'4': decodeByteString,
		'5': decodeByteString,
		'6': decodeByteString,
		'7': decodeByteString,
		'8': decodeByteString,
		'9': decodeByteString,
	}
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

	data, err := Decode(buf)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}

	fmt.Printf("Duration: %s\n", time.Since(now))
	p, err := json.MarshalIndent(data, "", " ")
	fmt.Printf("Decoded Data:\n%v", string(p))
}
