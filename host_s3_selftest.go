package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
)

// Exercise the real provider/filter subprocess protocol, never an AWS account.
func checkS3Composition(store *Store, fakeAWSPath string, archive []byte) error {
	jsonBytes := []byte(`{"nested":{"answer":42},"text":"visible value"}`)
	large := bytes.Repeat([]byte("data\n"), 1800000)
	deep := archive
	for layer := 0; layer < 12; layer++ {
		wrapped, err := makeTestZip([]testZipEntry{{Name: "next.zip", Data: deep}})
		switch {
		case err != nil:
			return err
		default:
			deep = wrapped
		}
	}
	objects := []struct {
		name string
		data []byte
	}{
		{"archive.zip", archive}, {"unlabelled", archive}, {"data.json", jsonBytes}, {"large.txt", large}, {"empty.txt", []byte{}},
		{"deep.zip", deep},
	}
	script := "#!/bin/sh\ncase \"$1 $2\" in\n" +
		"'configure list-profiles') printf 'default\\n' ;;\n" +
		"'configure get') case \"$3\" in region) printf 'us-east-1\\n' ;; *) exit 0 ;; esac ;;\n" +
		"'ec2 describe-regions') printf '%s' '{\"Regions\":[{\"RegionName\":\"us-east-1\",\"OptInStatus\":\"opt-in-not-required\"}]}' ;;\n" +
		"'s3api list-buckets') printf '%s' '{\"Buckets\":[{\"Name\":\"example-bucket\"}]}' ;;\n" +
		"'s3api list-objects-v2') printf '%s' '{\"Contents\":["
	for index, object := range objects {
		switch {
		case index != 0:
			script += ","
		default:
		}
		script += fmt.Sprintf("{\"Key\":\"%s\",\"Size\":%d}", object.name, len(object.data))
	}
	script += "]}' ;;\n's3 cp') case \"$3\" in\n"
	// Large data lives in a fixture file so the fake CLI does not embed it in
	// its command arguments. The fixture directory is controlled by this test.
	for _, object := range objects {
		path := fakeAWSPath + "-" + object.name + ".b64"
		switch err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(object.data)), 0600); {
		case err != nil:
			return fmt.Errorf("cannot write S3 fixture %s: %v", path, err)
		default:
		}
		script += fmt.Sprintf("'s3://example-bucket/%s') base64 -d -i '%s' ;;\n", object.name, path)
	}
	script += "*) echo 'AccessDenied' >&2; exit 1 ;;\nesac ;;\n*) echo 'unexpected AWS command' >&2; exit 97 ;;\nesac\n"
	switch err := os.WriteFile(fakeAWSPath, []byte(script), 0700); {
	case err != nil:
		return fmt.Errorf("cannot write fake S3 CLI: %v", err)
	default:
	}
	state := []byte(`{"view":"s3-objects","profile":"default","region":"us-east-1","operation":"s3-buckets","identifier":"example-bucket"}`)
	id, err := store.putBlob(state)
	switch {
	case err != nil:
		return err
	default:
	}
	listing, err := store.loadDriverForBlob("aws_driver", id)
	switch {
	case err != nil:
		return fmt.Errorf("S3 listing failed: %v", err)
	default:
	}
	for _, object := range objects {
		kind := "file"
		switch object.name {
		case "archive.zip", "deep.zip":
			kind = "archive"
		case "data.json":
			kind = "json"
		case "large.txt", "empty.txt":
			kind = "text"
		default:
		}
		handle, err := findResourceHandle(store, listing, object.name, kind)
		switch {
		case err != nil:
			return err
		default:
		}
		raw, err := store.serveChildRange(handle, 0, -1)
		switch {
		case err != nil || !bytes.Equal(raw, object.data):
			return fmt.Errorf("S3 object %s byte mismatch: got %d expected %d bytes: %v", object.name, len(raw), len(object.data), err)
		default:
		}
		switch object.name {
		case "deep.zip":
			for layer := 0; layer < 12; layer++ {
				children, err := store.loadDriverForHandle("zip_driver", handle)
				switch {
				case err != nil:
					return fmt.Errorf("ZIP composition failed at layer %d: %v", layer, err)
				default:
				}
				next, err := findResourceHandle(store, children, "next.zip", "archive")
				switch {
				case err != nil:
					return err
				default:
					handle = next
				}
			}
			switch err := checkS3NestedZip(store, handle); {
			case err != nil:
				return err
			default:
			}
		case "archive.zip", "unlabelled":
			switch err := checkS3NestedZip(store, handle); {
			case err != nil:
				return err
			default:
			}
		case "data.json":
			fields, err := store.loadDriverForHandle("json_driver", handle)
			switch {
			case err != nil:
				return err
			default:
			}
			scalar, err := findResourceHandle(store, fields, "text: visible value", "kv")
			switch {
			case err != nil:
				return err
			default:
			}
			raw, err := store.serveChildRange(scalar, 0, -1)
			switch {
			case err != nil || string(raw) != `"visible value"`:
				return fmt.Errorf("JSON scalar lost its bytes: %q, %v", raw, err)
			default:
			}
		default:
		}
	}
	return nil
}

func checkS3NestedZip(store *Store, handle ChildHandle) error {
	outer, err := store.loadDriverForHandle("zip_driver", handle)
	switch {
	case err != nil:
		return fmt.Errorf("cannot mount ZIP directly on S3 file: %v", err)
	default:
	}
	nested, err := findResourceHandle(store, outer, "nested.zip", "archive")
	switch {
	case err != nil:
		return err
	default:
	}
	inner, err := store.loadDriverForHandle("zip_driver", nested)
	switch {
	case err != nil:
		return fmt.Errorf("cannot compose ZIP inside S3 ZIP: %v", err)
	default:
	}
	leaf, err := findResourceHandle(store, inner, "deep.txt", "text")
	switch {
	case err != nil:
		return err
	default:
	}
	text, err := store.loadDriverForHandle("text_driver", leaf)
	switch {
	case err != nil:
		return fmt.Errorf("cannot compose text inside nested S3 ZIP: %v", err)
	default:
	}
	_, err = findResource(text, "https://example.com/deep", "url")
	return err
}
