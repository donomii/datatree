package driverlib

import (
	"encoding/json"
	"fmt"
	"os"
)

func SendRangeError(enc *json.Encoder, message string) {
	switch err := enc.Encode(RangeResp{Error: message}); {
	case err != nil:
		fmt.Fprintf(os.Stderr, "cannot encode range error %q: %v\n", message, err)
	default:
	}
}

// RunProvider keeps provider navigation descriptors private. Ordinary byte
// filters use Run, which also retains their complete upstream document.
func RunProvider(name string, list ListFunc) {
	RunWithReader(name, list, memoryChild)
}

func memoryChild(_ string, value []byte, offset int64, length int64) ([]byte, error) {
	return SliceRange(value, offset, length), nil
}

// WithSource preserves input bytes even when a selected filter rejects them.
func WithSource(list ListFunc) ListFunc {
	return func(raw []byte) ([]Resource, map[string][]byte, error) {
		resources, children, err := list(raw)
		switch {
		case err != nil:
			resources = []Resource{{Name: err.Error(), Kind: "error"}}
			children = map[string][]byte{}
		default:
		}
		switch {
		case children == nil:
			children = map[string][]byte{}
		default:
		}
		return RetainSource(resources, children, raw)
	}
}

func RetainSource(resources []Resource, children map[string][]byte, raw []byte) ([]Resource, map[string][]byte, error) {
	switch {
	case children == nil:
		children = map[string][]byte{}
	default:
	}
	key := unusedKey(children, "\x00datatree:source")
	resources = append(resources, Resource{Name: "[source bytes]", Kind: "raw", Key: key, Size: int64(len(raw))})
	children[key] = raw
	return resources, children, nil
}

// BindPayloads retains values supplied by presenters, never values guessed
// from display labels. Unique keys keep repeated labels independently readable.
func BindPayloads(resources []Resource, children map[string][]byte) ([]Resource, map[string][]byte) {
	switch {
	case children == nil:
		children = map[string][]byte{}
	default:
	}
	for index := range resources {
		switch {
		case resources[index].Payload == nil:
			continue
		default:
		}
		key := unusedKey(children, fmt.Sprintf("\x00datatree:value:%d", index))
		children[key] = resources[index].Payload
		resources[index].Key, resources[index].Readable = key, true
	}
	return resources, children
}

func unusedKey(children map[string][]byte, key string) string {
	for {
		switch _, exists := children[key]; {
		case exists:
			key += ":"
		default:
			return key
		}
	}
}
