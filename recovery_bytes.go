package main

import "fmt"

func (s *Store) serveChildRange(handle ChildHandle, offset int64, length int64) ([]byte, error) {
	live, err := s.resolveHandle(handle.Hid)
	switch {
	case err != nil:
		return nil, err
	default:
	}
	raw, err := s.serveLiveChildRange(live, offset, length)
	switch {
	case err == nil:
		return raw, nil
	case live.Route == nil:
		return nil, err
	default:
	}
	// A driver process or provider session can disappear between lookup and
	// read. Recreate it and retry the read once; actions are never replayed.
	reconnected, recoveryErr := s.restoreHandle(handle.Hid)
	switch {
	case recoveryErr != nil:
		return nil, fmt.Errorf("resource read failed (%v); reconnect failed: %v", err, recoveryErr)
	default:
		return s.serveLiveChildRange(reconnected, offset, length)
	}
}
