package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"datahumper/drivers_go/driverlib"
)

func gzipFixture(raw []byte, name string) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	writer.Name = name
	switch _, err := writer.Write(raw); {
	case err != nil:
		return nil, err
	default:
	}
	switch err := writer.Close(); {
	case err != nil:
		return nil, err
	default:
		return buffer.Bytes(), nil
	}
}

func makeFormatsFixture() ([]byte, error) {
	jsonGzip, err := gzipFixture([]byte(`{"answer":"actual bytes"}`), "data.json")
	switch {
	case err != nil:
		return nil, err
	default:
	}
	xml := []byte("<document><payload>" + base64.StdEncoding.EncodeToString(jsonGzip) + "</payload></document>")
	office, err := makeTestZip([]testZipEntry{{Name: "word/document.xml", Data: xml}})
	switch {
	case err != nil:
		return nil, err
	default:
	}
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	switch err := writer.WriteHeader(&tar.Header{Name: "document.docx", Mode: 0600, Size: int64(len(office))}); {
	case err != nil:
		return nil, err
	default:
	}
	switch _, err := writer.Write(office); {
	case err != nil:
		return nil, err
	default:
	}
	switch err := writer.Close(); {
	case err != nil:
		return nil, err
	default:
	}
	return gzipFixture(archive.Bytes(), "bundle.tar")
}

func checkFormatsComposition(store *Store, handle ChildHandle) error {
	path := []struct{ driver, name, kind string }{
		{"gzip_driver", "bundle.tar", "tar"},
		{"tar_driver", "document.docx", "archive"},
		{"zip_driver", "word/document.xml", "xml"},
		{"xml_driver", "payload", "xml"},
		{"xml_driver", "[text]", "text"},
		{"base64_driver", "[decoded Base64]", "gzip"},
		{"gzip_driver", "data.json", "json"},
		{"json_driver", "answer: actual bytes", "kv"},
		{"jsonstring_driver", "[decoded string]", "text"},
	}
	for _, step := range path {
		resources, err := store.loadDriverForHandle(step.driver, handle)
		switch {
		case err != nil:
			return fmt.Errorf("cannot compose %s: %v", step.driver, err)
		default:
		}
		next, err := findResourceHandle(store, resources, step.name, step.kind)
		switch {
		case err != nil:
			return fmt.Errorf("%s composition: %v", step.driver, err)
		default:
			handle = next
		}
	}
	raw, err := store.serveChildRange(handle, 0, -1)
	switch {
	case err != nil || string(raw) != "actual bytes":
		return fmt.Errorf("nested formats lost original value: %q %v", raw, err)
	default:
		return nil
	}
}

func checkAdditionalFilters(store *Store, directory string) error {
	// Every advertised embedded filter must preserve even rejected input bytes.
	input := []byte("exact bytes\x00\xff\r\n")
	for _, format := range driverlib.AdditionalFilters() {
		resources, err := loadFixtureDriver(store, format.Driver, input)
		switch {
		case err != nil:
			return err
		default:
		}
		source, err := findResourceHandle(store, resources, "[source bytes]", "raw")
		switch {
		case err != nil:
			return fmt.Errorf("%s did not retain source: %v", format.Driver, err)
		default:
		}
		raw, err := store.serveChildRange(source, 0, -1)
		switch {
		case err != nil || !bytes.Equal(raw, input):
			return fmt.Errorf("%s source mismatch: %q %v", format.Driver, raw, err)
		default:
		}
	}
	fixture, err := makeFormatsFixture()
	switch {
	case err != nil:
		return err
	default:
	}
	path := filepath.Join(directory, "formats.tgz")
	switch err := os.WriteFile(path, fixture, 0600); {
	case err != nil:
		return err
	default:
	}
	resources, err := loadFixtureDriver(store, "directory_driver", []byte(directory))
	switch {
	case err != nil:
		return err
	default:
	}
	handle, err := findResourceHandle(store, resources, "formats.tgz", "gzip")
	switch {
	case err != nil:
		return err
	default:
		return checkFormatsComposition(store, handle)
	}
}
