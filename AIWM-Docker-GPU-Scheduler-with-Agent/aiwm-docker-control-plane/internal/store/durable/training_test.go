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

func TestTrainingMetadataDurabilityAndCAS(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime.gob")
	s, err := Open(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	j := domain.Job{ID: "job", AllocationIntent: domain.AllocationIntent{WorkloadType: domain.WorkloadTraining}, Status: domain.JobRunning, TrainingToken: "per-job-test-only", Training: domain.TrainingState{CheckpointWarningAt: now}}
	if err = s.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	state := j.Training
	state.CheckpointStatus = domain.CheckpointAvailable
	state.LatestCheckpointURI = "s3://bucket/job/checkpoint"
	state.CheckpointStep = 9
	state.CheckpointCreatedAt = now
	saved, err := s.UpdateTraining(ctx, j.ID, 0, state, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateTraining(ctx, j.ID, 0, state, now); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("stale writer accepted")
	}
	realSave := s.save
	s.save = func(memory.Snapshot) error { return errors.New("disk failure") }
	failed := saved.Training
	failed.CheckpointStatus = domain.CheckpointFailed
	if _, err = s.UpdateTraining(ctx, j.ID, saved.Training.Revision, failed, now); err == nil {
		t.Fatal("save failure ignored")
	}
	s.save = realSave
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.GetJob(ctx, j.ID)
	if err != nil || got.Training.CheckpointStatus != domain.CheckpointAvailable || got.Training.CheckpointStep != 9 || got.Training.LatestCheckpointURI != state.LatestCheckpointURI || got.TrainingToken != j.TrainingToken {
		t.Fatal("metadata did not survive restart/rollback")
	}
}
