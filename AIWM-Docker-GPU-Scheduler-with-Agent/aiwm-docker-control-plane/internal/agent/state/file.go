package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
)

type FileStore struct {
	path string
	mu   sync.Mutex
}

func NewFileStore(path string) *FileStore { return &FileStore{path: path} }

func (s *FileStore) Load() (agent.PersistentState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return agent.PersistentState{ProcessedCommands: make(map[string]agent.CommandResult)}, nil
	}
	if err != nil {
		return agent.PersistentState{}, err
	}
	var result agent.PersistentState
	if err := json.Unmarshal(content, &result); err != nil {
		return agent.PersistentState{}, fmt.Errorf("decode %s: %w", s.path, err)
	}
	if result.ProcessedCommands == nil {
		result.ProcessedCommands = make(map[string]agent.CommandResult)
	}
	return result, nil
}

func (s *FileStore) Save(value agent.PersistentState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".aiwm-agent-state-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, s.path)
}
