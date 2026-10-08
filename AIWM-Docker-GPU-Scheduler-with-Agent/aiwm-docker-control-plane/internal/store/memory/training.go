package memory

import (
	"context"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

// UpdateTraining atomically publishes metadata against the latest training revision.
func (s *Store) UpdateTraining(_ context.Context, id string, expected uint64, next domain.TrainingState, at time.Time) (domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return domain.Job{}, domain.ErrNotFound
	}
	if job.WorkloadType != domain.WorkloadTraining || job.Training.Revision != expected {
		return domain.Job{}, domain.ErrConflict
	}
	if next.CheckpointStatus == domain.CheckpointRequested && job.Training.CheckpointStatus != domain.CheckpointRequested && job.Status != domain.JobRunning {
		return domain.Job{}, domain.ErrConflict
	}
	// Resume ownership and warning time are immutable after admission.
	next.ResumeCheckpointURI, next.ResumeFromJobID = job.Training.ResumeCheckpointURI, job.Training.ResumeFromJobID
	next.CheckpointWarningAt = job.Training.CheckpointWarningAt
	next.Revision = expected + 1
	job.Training, job.UpdatedAt = next, at
	s.jobs[id] = cloneJob(job)
	return cloneJob(job), nil
}
