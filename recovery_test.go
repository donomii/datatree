package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoveryUsesFieldIdentityWithoutSavingItsValue(t *testing.T) {
	store := NewStore()
	origin := routeOrigin{Route: handleRoute{Version: 1, Root: "/", Steps: []routeStep{{Driver: "root_driver", Name: "data.json", Key: "data.json", Kind: "json"}}}, Driver: "json_driver"}
	store.setOrigin("json", origin)
	id := store.addHandle(ChildHandle{Name: "secret: sensitive-value", ChildName: "\x00json:field:secret", Kind: "kv", DriverName: "json"})
	route, err := openRoute(store.recoveryKey, id)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	raw, err := json.Marshal(route)
	switch {
	case err != nil || bytes.Contains(raw, []byte("sensitive-value")):
		t.Fatalf("route retained field value: %s %v", raw, err)
	default:
	}
	newID := store.addHandle(ChildHandle{Name: "secret: updated-value", ChildName: "\x00json:field:secret", Kind: "kv", DriverName: "json"})
	handle, err := selectRecoveredChild(store, []Resource{{Name: "secret: updated-value", Kind: "kv", Handle: newID}}, route.Steps[len(route.Steps)-1])
	switch {
	case err != nil || handle.Name != "secret: updated-value":
		t.Fatalf("field-value change broke recovery: %+v %v", handle, err)
	default:
	}
}

func TestRecoveryKeyPersistsAndTokensAreAuthenticated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "key")
	first, err := openRecoveryStore(path)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	second, err := openRecoveryStore(path)
	switch {
	case err != nil || !bytes.Equal(first.recoveryKey, second.recoveryKey):
		t.Fatalf("restart lost key: %v", err)
	default:
	}
	route := handleRoute{Version: 1, Root: "/", Steps: []routeStep{{Driver: "root_driver", Name: "private-resource", Key: "private-resource", Kind: "directory"}}}
	token := sealRoute(first.recoveryKey, route)
	switch recovered, err := openRoute(second.recoveryKey, token); {
	case err != nil || recovered.Steps[0].Name != "private-resource":
		t.Fatalf("cannot restore token: %+v %v", recovered, err)
	case strings.Contains(token, "private-resource"):
		t.Fatal("token exposed route labels")
	default:
	}
	for _, invalid := range []string{token[:len(token)-12] + "invalid-data", "hdl-1", sealRoute(NewStore().recoveryKey, route)} {
		switch _, err := openRoute(second.recoveryKey, invalid); {
		case err == nil:
			t.Fatalf("accepted invalid token %q", invalid)
		default:
		}
	}
}

func TestRecoveryRejectsInvalidKeyInsteadOfReplacingIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	switch err := os.WriteFile(path, []byte("invalid"), 0600); {
	case err != nil:
		t.Fatal(err)
	default:
	}
	switch _, err := openRecoveryStore(path); {
	case err == nil:
		t.Fatal("accepted malformed recovery key")
	default:
	}
	raw, err := os.ReadFile(path)
	switch {
	case err != nil || string(raw) != "invalid":
		t.Fatalf("rewrote invalid key: %q %v", raw, err)
	default:
	}
}

func TestListenerReusesOriginAndHandlesOccupiedPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "listener")
	first, err := listenForRestart(defaultAddr, path)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	address := first.Addr().String()
	switch err := first.Close(); {
	case err != nil:
		t.Fatal(err)
	default:
	}
	second, err := listenForRestart(defaultAddr, path)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	defer second.Close()
	switch {
	case second.Addr().String() != address:
		t.Fatalf("restart changed browser origin from %s to %s", address, second.Addr())
	default:
	}
	third, err := listenForRestart(defaultAddr, path)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	defer third.Close()
	switch {
	case third.Addr().String() == address:
		t.Fatal("occupied old port was reused")
	default:
	}
}
