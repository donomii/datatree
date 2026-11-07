package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type handleRoute struct {
	Version int         `json:"version"`
	Root    string      `json:"root"`
	Steps   []routeStep `json:"steps"`
}

type routeStep struct {
	Driver string `json:"driver"`
	Name   string `json:"name"`
	Key    string `json:"key"`
	Kind   string `json:"kind"`
}

type routeOrigin struct {
	Route  handleRoute
	Driver string
}

const recoveryTokenPrefix = "hdl-v1."

func openPersistentStore() (*Store, error) {
	directory, err := os.UserConfigDir()
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot locate Datatree recovery configuration: %v", err)
	default:
		return openRecoveryStore(filepath.Join(directory, "datatree", "recovery.key"))
	}
}

func openRecoveryStore(path string) (*Store, error) {
	switch err := os.MkdirAll(filepath.Dir(path), 0700); {
	case err != nil:
		return nil, fmt.Errorf("cannot create recovery-key directory %q: %v", filepath.Dir(path), err)
	default:
	}
	key, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		key = make([]byte, 32)
		switch _, err := rand.Read(key); {
		case err != nil:
			return nil, fmt.Errorf("cannot generate recovery encryption key: %v", err)
		default:
		}
		file, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		switch {
		case os.IsExist(createErr):
			return openRecoveryStore(path)
		case createErr != nil:
			return nil, fmt.Errorf("cannot create recovery key %q: %v", path, createErr)
		default:
		}
		_, writeErr := file.Write(key)
		closeErr := file.Close()
		switch {
		case writeErr != nil || closeErr != nil:
			return nil, fmt.Errorf("cannot save recovery key %q: write=%v close=%v", path, writeErr, closeErr)
		default:
		}
	case err != nil:
		return nil, fmt.Errorf("cannot read recovery key %q: %v", path, err)
	default:
	}
	info, err := os.Lstat(path)
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot inspect recovery key %q: %v", path, err)
	case !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || len(key) != 32:
		return nil, fmt.Errorf("invalid recovery key %q: expected a private regular file containing exactly 32 bytes", path)
	default:
		store := newConfiguredStore()
		store.recoveryKey = key
		return store, nil
	}
}

func routeCipher(key []byte) cipher.AEAD {
	block, err := aes.NewCipher(key)
	switch {
	case err != nil:
		panic(fmt.Sprintf("invalid recovery encryption key: %v", err))
	default:
	}
	aead, err := cipher.NewGCM(block)
	switch {
	case err != nil:
		panic(fmt.Sprintf("cannot initialize handle recovery encryption: %v", err))
	default:
		return aead
	}
}

func sealRoute(key []byte, route handleRoute) string {
	raw, err := json.Marshal(route)
	switch {
	case err != nil:
		panic(fmt.Sprintf("cannot encode resource recovery route: %v", err))
	default:
	}
	aead := routeCipher(key)
	nonce := make([]byte, aead.NonceSize())
	switch _, err := rand.Read(nonce); {
	case err != nil:
		panic(fmt.Sprintf("cannot generate handle nonce: %v", err))
	default:
		sealed := aead.Seal(nonce, nonce, raw, []byte(recoveryTokenPrefix))
		return recoveryTokenPrefix + base64.RawURLEncoding.EncodeToString(sealed)
	}
}

func openRoute(key []byte, token string) (handleRoute, error) {
	switch {
	case !strings.HasPrefix(token, recoveryTokenPrefix):
		return handleRoute{}, fmt.Errorf("resource handle is not recoverable; uploaded data must be uploaded again after restart")
	default:
	}
	sealed, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, recoveryTokenPrefix))
	aead := routeCipher(key)
	switch {
	case err != nil || len(sealed) < aead.NonceSize()+aead.Overhead():
		return handleRoute{}, fmt.Errorf("invalid resource recovery token")
	default:
	}
	raw, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte(recoveryTokenPrefix))
	switch {
	case err != nil:
		return handleRoute{}, fmt.Errorf("resource recovery token cannot be authenticated with this installation's recovery key")
	default:
	}
	var route handleRoute
	switch err := json.Unmarshal(raw, &route); {
	case err != nil:
		return route, fmt.Errorf("invalid resource recovery route: %v", err)
	case route.Version != 1 || len(route.Steps) == 0 || route.Steps[0].Driver != "root_driver" || (route.Root != "" && !filepath.IsAbs(route.Root)):
		return route, fmt.Errorf("invalid resource recovery route: expected version 1, root driver and absolute root directory")
	default:
	}
	for _, step := range route.Steps {
		valid := false
		for _, driver := range driverNames {
			valid = valid || driver == step.Driver
		}
		switch {
		case !valid || step.Key == "" || step.Kind == "":
			return route, fmt.Errorf("invalid driver or child selector in resource recovery route")
		default:
		}
	}
	return route, nil
}

func (s *Store) setOrigin(session string, origin routeOrigin) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.origins[session] = origin
}

func (s *Store) origin(session string) (routeOrigin, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	origin, known := s.origins[session]
	return origin, known
}

func (s *Store) recoverableHandle(h ChildHandle) ChildHandle {
	origin, known := s.origin(h.DriverName)
	switch {
	case !known:
		return h
	default:
		steps := append([]routeStep{}, origin.Route.Steps...)
		steps = append(steps, routeStep{Driver: origin.Driver, Name: recoveryName(h), Key: h.ChildName, Kind: h.Kind})
		route := handleRoute{Version: 1, Root: origin.Route.Root, Steps: steps}
		h.Route, h.Hid = &route, sealRoute(s.recoveryKey, route)
		return h
	}
}

func (s *Store) resolveHandle(id string) (ChildHandle, error) {
	handle, known := s.getHandle(id)
	switch {
	case known:
		switch _, running := s.getDriver(handle.DriverName); {
		case running:
			return handle, nil
		default:
		}
	default:
	}
	return s.restoreHandle(id)
}

func (s *Store) restoreHandle(id string) (ChildHandle, error) {
	route, err := openRoute(s.recoveryKey, id)
	switch {
	case err != nil:
		return ChildHandle{}, err
	default:
	}
	// A channel serializes session reconstruction without holding a data lock
	// across driver calls. Each replay uses an isolated root/session namespace.
	<-s.recoveryGate
	defer func() { s.recoveryGate <- struct{}{} }()
	worker := NewStore()
	worker.recoveryKey, worker.rootDirectory = s.recoveryKey, route.Root
	transferred := false
	defer func() {
		switch {
		case transferred:
			return
		default:
			worker.stopDrivers()
		}
	}()
	resources, err := worker.loadRootDriver()
	switch {
	case err != nil:
		return ChildHandle{}, fmt.Errorf("cannot reconnect resource root: %v", err)
	default:
	}
	var current ChildHandle
	for index, step := range route.Steps {
		switch {
		case index != 0:
			resources, err = worker.loadDriverForHandle(step.Driver, current)
			switch {
			case err != nil:
				return current, fmt.Errorf("cannot reconnect %s layer: %v", step.Driver, err)
			default:
			}
		default:
		}
		current, err = selectRecoveredChild(worker, resources, step)
		switch {
		case err != nil:
			return current, err
		default:
		}
	}
	s.adoptRecovered(worker, id, current, route)
	transferred = true
	return s.resolveHandle(id)
}

func selectRecoveredChild(store *Store, resources []Resource, step routeStep) (ChildHandle, error) {
	matches := []ChildHandle{}
	for _, resource := range resources {
		handle, known := store.getHandle(resource.Handle)
		switch {
		case !known:
			return handle, fmt.Errorf("reconnected resource %q has no handle", resource.Name)
		case recoveryName(handle) != step.Name || resource.Kind != step.Kind:
			continue
		default:
		}
		switch {
		case handle.ChildName == step.Key:
			return handle, nil
		default:
			matches = append(matches, handle)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	default:
		for _, resource := range resources {
			switch resource.Kind {
			case "error":
				return ChildHandle{}, fmt.Errorf("cannot reconnect %q: %s", step.Name, resource.Name)
			default:
			}
		}
		return ChildHandle{}, fmt.Errorf("cannot reconnect resource %q: expected one matching %s child, found %d; resource may have moved or permissions may have changed", step.Name, step.Kind, len(matches))
	}
}

func recoveryName(handle ChildHandle) string {
	switch {
	case handle.ChildName == "\x00json:scalar":
		return ""
	case handle.Kind == "kv":
		label, _, _ := strings.Cut(handle.Name, ": ")
		return label
	default:
		return handle.Name
	}
}

func (s *Store) adoptRecovered(worker *Store, id string, current ChildHandle, route handleRoute) {
	prefix := newID("recovered") + "/"
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, driver := range worker.drivers {
		s.drivers[prefix+name] = driver
	}
	for name, origin := range worker.origins {
		s.origins[prefix+name] = origin
	}
	for key, handle := range worker.handles {
		handle.DriverName = prefix + handle.DriverName
		s.handles[key] = handle
	}
	current.DriverName, current.Hid, current.Route = prefix+current.DriverName, id, &route
	s.handles[id] = current
}
