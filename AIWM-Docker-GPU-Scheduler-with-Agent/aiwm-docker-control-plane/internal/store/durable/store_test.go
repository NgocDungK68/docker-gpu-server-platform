package durable

import (
	"context"
	"errors"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/store/memory"
	"path/filepath"
	"testing"
	"time"
)

func TestRestartPreservesStateAndExcludesStaleInventory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.gob")
	s, err := Open(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	_, err = s.UpsertServer(context.Background(), domain.Server{ID: "s", MachineID: "m", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CreateJob(context.Background(), domain.Job{ID: "j", Status: domain.JobQueued}); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.GetJob(context.Background(), "j"); err != nil {
		t.Fatal(err)
	}
	server, _ := s.GetServer(context.Background(), "s")
	if server.Status != domain.ServerOffline || server.Schedulable(now, time.Minute) {
		t.Fatal("restart trusted old inventory")
	}
}
func TestDiskFailureRollsBackAndLockExcludesSecondWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.gob")
	s, err := Open(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	second, err := Open(path, time.Minute)
	if err == nil {
		second.Close()
		t.Fatal("second writer accepted")
	}
	s.save = func(memory.Snapshot) error { return errors.New("disk full") }
	if err = s.CreateJob(context.Background(), domain.Job{ID: "j"}); err == nil {
		t.Fatal("failed disk commit acknowledged")
	}
	if _, err = s.GetJob(context.Background(), "j"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("failed write visible")
	}
}
