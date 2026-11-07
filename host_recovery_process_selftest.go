package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

type recoveryWorkerRequest struct {
	KeyPath string `json:"keyPath"`
	Handle  string `json:"handle"`
}

// The self-test's private subprocess mode starts a new router with no runtime
// handles or drivers. Only the durable key and browser handle cross processes.
func runRecoveryWorker() error {
	var request recoveryWorkerRequest
	switch err := json.NewDecoder(os.Stdin).Decode(&request); {
	case err != nil:
		return err
	default:
	}
	store, err := openRecoveryStore(request.KeyPath)
	switch {
	case err != nil:
		return err
	default:
	}
	defer store.stopDrivers()
	handle, err := store.resolveHandle(request.Handle)
	switch {
	case err != nil:
		return err
	default:
	}
	raw, err := store.serveChildRange(handle, 0, -1)
	switch {
	case err != nil:
		return err
	default:
		return json.NewEncoder(os.Stdout).Encode(raw)
	}
}

func checkRecoveryInNewProcess(key string, handle string) error {
	executable, err := os.Executable()
	switch {
	case err != nil:
		return err
	default:
	}
	request, err := json.Marshal(recoveryWorkerRequest{KeyPath: key, Handle: handle})
	switch {
	case err != nil:
		return err
	default:
	}
	cmd := exec.Command(executable, "-test-recovery-worker")
	cmd.Stdin = bytes.NewReader(request)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	response, err := cmd.Output()
	switch {
	case err != nil:
		return fmt.Errorf("fresh-process resource recovery failed: %v; stderr: %s", err, stderr.String())
	default:
	}
	var raw []byte
	switch err := json.Unmarshal(response, &raw); {
	case err != nil:
		return fmt.Errorf("fresh-process recovery returned invalid data: %v", err)
	case string(raw) != "deep https://example.com/deep\n":
		return fmt.Errorf("fresh-process recovery returned wrong file bytes: %q", raw)
	default:
		return nil
	}
}
