package main

import (
	"fmt"
	"path/filepath"
)

func checkRestartRecovery(directory string) error {
	key := filepath.Join(directory, "recovery", "key")
	first, err := openRecoveryStore(key)
	switch {
	case err != nil:
		return err
	default:
	}
	defer first.stopDrivers()
	first.rootDirectory = directory
	resources, err := first.loadRootDriver()
	switch {
	case err != nil:
		return err
	default:
	}
	path := []struct{ name, kind, driver string }{
		{"aws", "aws", "aws_driver"},
		{"default", "aws", "aws_driver"},
		{"[regions and resources]", "aws", "aws_driver"},
		{"us-east-1 (opt-in-not-required)", "aws", "aws_driver"},
		{"[service catalog]", "aws", "aws_driver"},
		{"S3 buckets", "aws", "aws_driver"},
		{"example-bucket", "aws", "aws_driver"},
		{"archive.zip", "archive", "zip_driver"},
		{"nested.zip", "archive", "zip_driver"},
	}
	for _, step := range path {
		handle, err := findResourceHandle(first, resources, step.name, step.kind)
		switch {
		case err != nil:
			return err
		default:
		}
		resources, err = first.loadDriverForHandle(step.driver, handle)
		switch {
		case err != nil:
			return err
		default:
		}
	}
	leaf, err := findResourceHandle(first, resources, "deep.txt", "text")
	switch {
	case err != nil:
		return err
	default:
	}
	savedID := leaf.Hid
	first.stopDrivers()
	switch err := checkRecoveryInNewProcess(key, savedID); {
	case err != nil:
		return err
	default:
	}
	// A newly constructed router and newly loaded key simulate a fresh process:
	// no runtime handles, sessions or blobs are copied across the restart.
	second, err := openRecoveryStore(key)
	switch {
	case err != nil:
		return err
	default:
	}
	defer second.stopDrivers()
	second.rootDirectory = "/"
	recovered, err := second.resolveHandle(savedID)
	switch {
	case err != nil:
		return fmt.Errorf("cannot rebuild nested S3 handle after restart: %v", err)
	default:
	}
	raw, err := second.serveChildRange(recovered, 0, -1)
	switch {
	case err != nil || string(raw) != "deep https://example.com/deep\n":
		return fmt.Errorf("recovered S3 file has wrong bytes: %q: %v", raw, err)
	default:
	}
	second.stopDriver(recovered.DriverName)
	text, err := second.loadDriverForHandle("text_driver", recovered)
	switch {
	case err != nil:
		return fmt.Errorf("cannot revive idle nested provider session: %v", err)
	default:
	}
	_, err = findResource(text, "https://example.com/deep", "url")
	return err
}
