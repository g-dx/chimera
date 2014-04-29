package bencode

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"runtime"
	"strconv"
	sha1Hash "crypto/sha1"
	"io"
)

// Function map for decoding
var decodeFunctions map[rune]func([]byte) (interface{}, []byte)

const (
	typeTerminator      rune = 'e'
	byteStringSeparator rune = ':'
	intSize                  = 64
)

func DecodeAsDict(r io.Reader) (map[string]interface{}, error) {

	// Decode and check for error
	data, err := Decode(r)
	if err != nil {
		return nil, err
	}

	// Check type
	m, ok := data.(map[string] interface{})
	if !ok {
		return nil, errors.New("Supplied data is not a dictionary.")
	}
	return m, nil
}

func Decode(r io.Reader) (v interface{}, err error) {

	// Recover from any decoding panics & return error
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(runtime.Error); ok {
				panic(r)
			}
			err = r.(error)
		}
	}()

	// Read bytes
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Decode & check all data processed
	v, remainingBuf := decodeFnFor(nextRune(buf))(buf)
	if len(remainingBuf) != 0 {
		panic(errors.New("Trailing data detected: " + string(remainingBuf)))
	}
	return v, nil
}

func decodeInteger(buf []byte) (interface{}, []byte) {

	// Check for terminating character
	i := bytes.IndexRune(buf, typeTerminator)
	if i == -1 {
		panic(errors.New("Failed to decode integer as no ending 'e' found"))
	}

	// Convert to signed 64-bit int
	v, _ := strconv.ParseInt(string(buf[1:i]), 10, intSize)
	return v, buf[i+1:]
}

func decodeByteString(buf []byte) (interface{}, []byte) {

	// Check for terminating character
	i := bytes.IndexRune(buf, byteStringSeparator)
	if i == -1 {
		panic(errors.New("Failed to decode byte string as no terminating ':' found"))
	}

	// Calculate length of string & discard length bytes
	strLen, _ := strconv.ParseUint(string(buf[:i]), 10, intSize)
	buf = buf[i+1:]
	return string(buf[:strLen]), buf[strLen:]
}

func decodeList(buf []byte) (interface{}, []byte) {

	list := make([]interface{}, 0, 10)
	var v interface{}

	// Drop leading 'l' and consume until terminating character
	buf = buf[1:]
	for r := nextRune(buf); r != typeTerminator; r = nextRune(buf) {
		v, buf = decodeFnFor(r)(buf)
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
	for r := nextRune(buf); r != typeTerminator; r = nextRune(buf) {
		k, preValueBuf = decodeByteString(buf)
		v, buf = decodeFnFor(nextRune(preValueBuf))(preValueBuf)
		dict[k.(string)] = v

		// SPECIAL CASE:
		// Any dictionary key named "info" gets a SHA-1 hash of its value added to the result
		if k.(string) == "info" {
			dict["info_hash"] = string(sha1(preValueBuf[0:len(preValueBuf)-len(buf)]))
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

func decodeFnFor(r rune) func([]byte) (interface{}, []byte) {
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
