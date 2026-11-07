package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

func checkAllFilterBytes(store *Store, archive []byte) error {
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/lS8AAAAASUVORK5CYII=")
	switch {
	case err != nil:
		return err
	default:
	}
	config := []byte(`{"kind":"ConfigMap","binaryData":{"archive.zip":"` + base64.StdEncoding.EncodeToString(archive) + `"}}`)
	fixtures := []struct {
		driver string
		raw    []byte
	}{
		{"text_driver", []byte("exact text\r\nhttps://example.com/source\n")},
		{"html_driver", []byte("<title>Actual title</title><a href=\"https://example.com/source\">link</a>")},
		{"pdf_driver", []byte("%PDF-1.7 /Type /Page /URI (https://example.com/source)")},
		{"image_driver", png},
		{"csv_driver", []byte("field,value\r\nname,\"actual data\"\r\n")},
		{"diff_driver", []byte("@@ -1 +1 @@\r\n-old\r\n+new\r\n")},
		{"json_driver", []byte(" {\"field\": [true, null, \"actual value\"]} \n")},
		{"json_driver", []byte(`{"apiVersion":"datatree.local/v1","kind":"DriverReadError","message":"this is actual file data"}`)},
		{"zip_driver", archive},
		{"k8s_configmap_driver", config},
		{"k8s_pod_driver", []byte(fakePodManifest)},
		{"k8s_workload_driver", []byte(`{"kind":"Deployment","spec":{"replicas":1},"status":{}}`)},
		{"k8s_service_driver", []byte(`{"kind":"Service","spec":{"type":"ClusterIP","ports":[{"port":80}]}}`)},
		{"k8s_node_driver", []byte(`{"kind":"Node","status":{"nodeInfo":{"architecture":"arm64"}}}`)},
		{"zip_driver", []byte("invalid ZIP still retains its exact input")},
	}
	for _, fixture := range fixtures {
		resources, err := loadFixtureDriver(store, fixture.driver, fixture.raw)
		switch {
		case err != nil:
			return err
		default:
		}
		source, err := findResourceHandle(store, resources, "[source bytes]", "raw")
		switch {
		case err != nil:
			return fmt.Errorf("%s lost original input: %v", fixture.driver, err)
		default:
		}
		raw, err := store.serveChildRange(source, 0, -1)
		switch {
		case err != nil || !bytes.Equal(raw, fixture.raw):
			return fmt.Errorf("%s source byte mismatch: %v", fixture.driver, err)
		default:
		}
		view, err := store.loadDriverForHandle("text_driver", source)
		switch {
		case err != nil:
			return fmt.Errorf("cannot compose text on %s source: %v", fixture.driver, err)
		default:
		}
		_, err = findResourceHandle(store, view, "[source bytes]", "raw")
		switch {
		case err != nil:
			return err
		default:
		}
		switch fixture.driver {
		case "k8s_configmap_driver":
			payload, err := findResourceHandle(store, resources, "archive.zip", "archive")
			switch {
			case err != nil:
				return err
			default:
			}
			switch err := checkS3NestedZip(store, payload); {
			case err != nil:
				return fmt.Errorf("ConfigMap ZIP composition failed: %v", err)
			default:
			}
		default:
		}
	}
	return nil
}

func loadFixtureDriver(store *Store, driver string, raw []byte) ([]Resource, error) {
	id, err := store.putBlob(raw)
	switch {
	case err != nil:
		return nil, err
	default:
		return store.loadDriverForBlob(driver, id)
	}
}

func checkProfilerByteComposition(store *Store, root []Resource) error {
	usb, err := findResourceHandle(store, root, "usb", "usb")
	switch {
	case err != nil:
		return err
	default:
	}
	buses, err := store.loadDriverForHandle("system_profiler_driver", usb)
	switch {
	case err != nil:
		return err
	default:
	}
	bus, err := findResourceHandle(store, buses, "Test USB Bus", "usb")
	switch {
	case err != nil:
		return err
	default:
	}
	raw, err := store.serveChildRange(bus, 0, -1)
	var node struct {
		Name string `json:"_name"`
	}
	switch {
	case err != nil:
		return err
	case json.Unmarshal(raw, &node) != nil || node.Name != "Test USB Bus":
		return fmt.Errorf("USB child is not actual node JSON: %q", raw)
	default:
	}
	fields, err := store.loadDriverForHandle("json_driver", bus)
	switch {
	case err != nil:
		return err
	default:
	}
	_, err = findResource(fields, "_name: Test USB Bus", "kv")
	return err
}
