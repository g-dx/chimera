package bittorrent

import (
	"errors"
	"fmt"
)

func newError(format string, args...interface {}) error {
	return errors.New(fmt.Sprintf(format, args))
}

func optBs(entries map[string] interface {}, key string) (string) {
	v, ok := entries[key]
	if !ok {
		return ""
	}
	return v.(string)
}

func optI(entries map[string] interface {}, key string) (int64) {

	v, ok := entries[key]
	if !ok {
		return 0
	}
	return v.(int64)
}

func optL(entries map[string] interface {}, key string) ([]interface {}) {

	v, ok := entries[key]
	if !ok {
		return nil
	}
	return v.([]interface {})
}

func bs(entries map[string] interface{}, key string) (string) {

	v, ok := entries[key]
	if !ok {
		panic(newError("Mandatory bytestring (%v) not found.", key))
	}
	return v.(string)
}

func l(entries map[string] interface {}, key string) ([]interface {}) {

	v, ok := entries[key]
	if !ok {
		panic(newError("Mandatory list (%v) not found.", key))
	}
	return v.([]interface {})
}

func i(entries map[string] interface {}, key string) (int64) {

	v, ok := entries[key]
	if !ok {
		panic(newError("Mandatory integer (%v) not found.", key))
	}
	return v.(int64)
}

func d(entries map[string] interface {}, key string) (map[string] interface {}) {

	v, ok := entries[key]
	if !ok {
		panic(newError("Mandatory dictionary (%v) not found.", key))
	}
	return v.(map[string] interface {})
}
