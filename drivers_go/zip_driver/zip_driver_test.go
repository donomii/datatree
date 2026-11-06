package zipdriver

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

func TestDuplicateZIPNamesKeepIndependentBytes(t *testing.T) {
	var raw bytes.Buffer
	writer := zip.NewWriter(&raw)
	for _, body := range []string{"first content", "second content"} {
		entry, err := writer.Create("same.txt")
		switch {
		case err != nil:
			t.Fatal(err)
		default:
		}
		switch _, err := entry.Write([]byte(body)); {
		case err != nil:
			t.Fatal(err)
		default:
		}
	}
	switch err := writer.Close(); {
	case err != nil:
		t.Fatal(err)
	default:
	}
	resources, children, err := listZipEntries(raw.Bytes())
	switch {
	case err != nil || len(resources) != 2:
		t.Fatalf("expected both ZIP entries: %+v %v", resources, err)
	case resources[0].Key == resources[1].Key:
		t.Fatal("duplicate names share one byte handle")
	case string(children[resources[0].Key]) != "first content" || string(children[resources[1].Key]) != "second content":
		t.Fatal("ZIP entry bytes were replaced")
	default:
	}
}

func TestCorruptZIPDoesNotPretendToBeEmpty(t *testing.T) {
	switch _, _, err := listZipEntries([]byte("not an archive")); {
	case err == nil || !strings.Contains(err.Error(), "cannot browse ZIP"):
		t.Fatalf("expected visible archive error, got %v", err)
	default:
	}
}
