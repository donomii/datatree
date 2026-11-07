package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Keep the browser's origin reachable across ordinary no-argument restarts.
// A configured fixed address remains authoritative; an occupied old port falls
// back to an available port rather than contacting the process occupying it.
func listenForRestart(address string, path string) (net.Listener, error) {
	switch address {
	case defaultAddr:
		raw, err := os.ReadFile(path)
		switch {
		case os.IsNotExist(err):
			return saveListener(address, path)
		case err != nil:
			return nil, fmt.Errorf("cannot read saved Datatree listener %q: %v", path, err)
		default:
		}
		previous := string(raw)
		host, port, parseErr := net.SplitHostPort(previous)
		number, numberErr := strconv.Atoi(port)
		switch {
		case parseErr != nil || host != "127.0.0.1" || numberErr != nil || number < 1 || number > 65535 || strings.TrimSpace(previous) != previous:
			return nil, fmt.Errorf("invalid saved Datatree listener %q: expected 127.0.0.1 and a valid nonzero port", path)
		default:
		}
		listener, bindErr := net.Listen("tcp", previous)
		switch {
		case bindErr == nil:
			return listener, nil
		default:
			return saveListener(address, path)
		}
	default:
		return net.Listen("tcp", address)
	}
}

func saveListener(address string, path string) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	switch {
	case err != nil:
		return nil, err
	default:
	}
	switch err := os.MkdirAll(filepath.Dir(path), 0700); {
	case err != nil:
		_ = listener.Close()
		return nil, fmt.Errorf("cannot create listener-state directory: %v", err)
	default:
	}
	switch err := os.WriteFile(path, []byte(listener.Addr().String()), 0600); {
	case err != nil:
		_ = listener.Close()
		return nil, fmt.Errorf("cannot save listener for next restart: %v", err)
	default:
		return listener, nil
	}
}

func listenPersistent(address string) (net.Listener, error) {
	directory, err := os.UserConfigDir()
	switch {
	case err != nil:
		return nil, err
	default:
		return listenForRestart(address, filepath.Join(directory, "datatree", "listener"))
	}
}
