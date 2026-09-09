// Package durable adds crash-safe single-process persistence to the existing repository.
// It does not provide distributed/HA scheduling or replace a future PostgreSQL adapter.
package durable

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/ports"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/store/memory"
)

var _ ports.Repository = (*Store)(nil)

// Store serializes reads/writes around snapshot + atomic disk commit.
type Store struct {
	mu   sync.Mutex
	core *memory.Store
	path string
	lock *os.File
	save func(memory.Snapshot) error
}

// Open validates the snapshot and takes an OS lock released automatically on process death.
func Open(path string, offlineAfter time.Duration) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockFile(lock); err != nil {
		lock.Close()
		return nil, fmt.Errorf("another control plane owns this state file: %w", err)
	}
	s := &Store{core: memory.New(offlineAfter), path: path, lock: lock}
	s.save = s.saveSnapshot
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.Close()
		return nil, err
	}
	if len(data) > 0 {
		var snapshot memory.Snapshot
		if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&snapshot); err != nil {
			s.Close()
			return nil, fmt.Errorf("invalid state snapshot: %w", err)
		}
		if snapshot.Version != 1 || snapshot.Servers == nil || snapshot.Machines == nil || snapshot.Jobs == nil || snapshot.Commands == nil {
			s.Close()
			return nil, fmt.Errorf("unsupported or incomplete state snapshot")
		}
		// After restart, require a new inventory before making a placement.
		for id, server := range snapshot.Servers {
			server.Status = domain.ServerOffline
			server.InventoryReceivedAt = time.Time{}
			snapshot.Servers[id] = server
		}
		s.core.Restore(snapshot)
	} else if err == nil {
		s.Close()
		return nil, fmt.Errorf("empty state snapshot")
	}
	return s, nil
}

// Close releases the process lock; every prior successful mutation is already synced.
func (s *Store) Close() error { return s.lock.Close() }

func transact[T any](s *Store, fn func() (T, error)) (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := s.core.Export()
	value, err := fn()
	if err != nil {
		s.core.Restore(before)
		return value, err
	}
	if err := s.save(s.core.Export()); err != nil {
		s.core.Restore(before)
		var zero T
		return zero, fmt.Errorf("commit control-plane snapshot: %w", err)
	}
	return value, nil
}

func read[T any](s *Store, fn func() (T, error)) (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn()
}

func (s *Store) saveSnapshot(snapshot memory.Snapshot) error {
	file, err := os.CreateTemp(filepath.Dir(s.path), ".aiwm-state-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		err = gob.NewEncoder(file).Encode(snapshot)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, s.path)
}
