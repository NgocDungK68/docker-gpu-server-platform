package durable

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/store/memory"
)

func TestCalendarCommitRollbackAndRestart(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	start := now.Add(time.Hour)
	path := filepath.Join(t.TempDir(), "calendar.gob")
	s, err := Open(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	_, err = s.UpsertServer(ctx, domain.Server{ID: "s", MachineID: "m", OrganizationID: "org", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now, GPUs: []domain.GPU{{UUID: "g", Healthy: true, State: domain.GPUFree, MemoryTotalMiB: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		err = s.CreateJob(ctx, domain.Job{ID: id, OrganizationID: "org", Status: domain.JobQueued, Resources: domain.ResourceRequest{GPUCount: 1, MinVRAMMiB: 1}, AllocationIntent: domain.AllocationIntent{NeededAt: start.Format(time.RFC3339Nano), TTLSeconds: 3600}})
		if err != nil {
			t.Fatal(err)
		}
	}
	p := domain.Placement{ServerID: "s", GPUUUIDs: []string{"g"}}
	invalid := func([]domain.Job, []domain.Server) ([]domain.ReservationPlan, error) {
		return []domain.ReservationPlan{{JobID: "a", Placement: &p}, {JobID: "b", Placement: &p}}, nil
	}
	if _, err = s.ReplanReservations(ctx, now, invalid); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("overlapping atomic batch accepted", err)
	}
	a, _ := s.GetJob(ctx, "a")
	if a.Assignment != nil {
		t.Fatal("partial batch committed")
	}
	valid := func([]domain.Job, []domain.Server) ([]domain.ReservationPlan, error) {
		return []domain.ReservationPlan{{JobID: "a", Placement: &p}}, nil
	}
	s.save = func(memory.Snapshot) error { return errors.New("disk full") }
	if _, err = s.ReplanReservations(ctx, now, valid); err == nil {
		t.Fatal("disk failure acknowledged")
	}
	a, _ = s.GetJob(ctx, "a")
	if a.Assignment != nil {
		t.Fatal("failed persistence exposed reservation")
	}
	s.save = s.saveSnapshot
	if _, err = s.ReplanReservations(ctx, now, valid); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	a, _ = s.GetJob(ctx, "a")
	if a.Assignment == nil || !a.Assignment.StartAt.Equal(start) || a.Assignment.ReservationState != "PLANNED" || len(s.core.Export().Commands) != 0 {
		t.Fatal("calendar did not survive restart", a)
	}
	server, _ := s.GetServer(ctx, "s")
	if server.Status != domain.ServerOffline {
		t.Fatal("restart skipped fresh inventory gate")
	}
}
