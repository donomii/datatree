package driverlib

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
)

func TestSourcesSurviveRejectedFiltersAndKeyCollisions(t *testing.T) {
	input := []byte("exact bytes\x00\xff\r\n")
	view := WithSource(func([]byte) ([]Resource, map[string][]byte, error) {
		return nil, nil, fmt.Errorf("wrong format")
	})
	resources, children, err := view(input)
	switch {
	case err != nil || len(resources) != 2 || resources[0].Kind != "error":
		t.Fatalf("expected error and retained source: %+v %v", resources, err)
	case !bytes.Equal(children[resources[1].Key], input):
		t.Fatal("filter rejection lost input bytes")
	default:
	}
	resources, children, err = RetainSource([]Resource{{Name: "[source bytes]", Kind: "file", Key: "\x00datatree:source"}}, map[string][]byte{"\x00datatree:source": []byte("entry")}, input)
	switch {
	case err != nil || resources[0].Key == resources[1].Key || string(children[resources[0].Key]) != "entry" || !bytes.Equal(children[resources[1].Key], input):
		t.Fatalf("source collision lost bytes: %+v %v", resources, err)
	default:
	}
}

func TestReadErrorsCannotBeConfusedWithFileBytes(t *testing.T) {
	data := []byte(`{"apiVersion":"datatree.local/v1","kind":"DriverReadError","message":"actual user data"}`)
	for _, failure := range []string{"", "permission denied"} {
		var input, output bytes.Buffer
		err := json.NewEncoder(&input).Encode(RangeResp{Data: base64.StdEncoding.EncodeToString(data), Size: int64(len(data)), Error: failure})
		switch {
		case err != nil:
			t.Fatal(err)
		default:
		}
		raw, err := FetchUpstreamBytes(json.NewEncoder(&output), json.NewDecoder(&input))
		switch {
		case failure != "" && err == nil:
			t.Fatal("transport failure masqueraded as file data")
		case failure == "" && (err != nil || !bytes.Equal(raw, data)):
			t.Fatalf("legitimate JSON bytes were mistaken for a read error: %q %v", raw, err)
		default:
		}
	}
}

func TestPresenterValuesKeepIndependentPayloads(t *testing.T) {
	resources := []Resource{KV("value", "first"), KV("value", "second"), KV("empty", "")}
	resources, children := BindPayloads(resources, nil)
	for index, expected := range []string{"first", "second", ""} {
		value, present := children[resources[index].Key]
		switch {
		case !present || !resources[index].Readable || string(value) != expected:
			t.Fatalf("presenter value %d lost bytes: %+v %q", index, resources[index], value)
		default:
		}
	}
}
