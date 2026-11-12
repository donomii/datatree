package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

const tsvFixture = "name\tnote\r\nAlice, Bob\t\"tab\there\nnext line\"\r\n"

func checkTSVComposition(store *Store, handle ChildHandle) error {
	resources, err := store.loadDriverForHandle("tsv_driver", handle)
	switch {
	case err != nil:
		return fmt.Errorf("cannot mount TSV on data handle: %v", err)
	default:
	}
	source, err := findResourceHandle(store, resources, "[source bytes]", "raw")
	switch {
	case err != nil:
		return err
	default:
	}
	raw, err := store.serveChildRange(source, 0, -1)
	switch {
	case err != nil || !bytes.Equal(raw, []byte(tsvFixture)):
		return fmt.Errorf("TSV source changed bytes: %q %v", raw, err)
	default:
	}
	row, err := findResourceHandle(store, resources, "[0]", "json")
	switch {
	case err != nil:
		return err
	default:
	}
	fields, err := store.loadDriverForHandle("json_driver", row)
	switch {
	case err != nil:
		return fmt.Errorf("cannot compose JSON on TSV row: %v", err)
	default:
	}
	cell, err := findResourceHandle(store, fields, "note: tab\there\nnext line", "kv")
	switch {
	case err != nil:
		return err
	default:
	}
	view, err := store.loadDriverForHandle("text_driver", cell)
	switch {
	case err != nil:
		return fmt.Errorf("cannot compose text on TSV cell: %v", err)
	default:
		_, err = findResourceHandle(store, view, "[source bytes]", "raw")
		return err
	}
}

func checkLocalTSV(store *Store, directory string) error {
	path := filepath.Join(directory, "table.tsv")
	switch err := os.WriteFile(path, []byte(tsvFixture), 0600); {
	case err != nil:
		return fmt.Errorf("cannot create local TSV fixture %s: %v", path, err)
	default:
	}
	resources, err := loadFixtureDriver(store, "directory_driver", []byte(directory))
	switch {
	case err != nil:
		return err
	default:
	}
	handle, err := findResourceHandle(store, resources, "table.tsv", "tsv")
	switch {
	case err != nil:
		return err
	default:
		return checkTSVComposition(store, handle)
	}
}
