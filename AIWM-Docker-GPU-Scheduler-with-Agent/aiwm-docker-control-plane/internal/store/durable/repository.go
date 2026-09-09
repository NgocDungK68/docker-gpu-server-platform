package durable

import (
	"context"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"time"
)

// UpsertServer commits the repository mutation before returning.
func (s *Store) UpsertServer(ctx context.Context, server domain.Server) (domain.Server, error) {
	return transact(s, func() (domain.Server, error) { return s.core.UpsertServer(ctx, server) })

}

// AuthenticateAgent reads a consistent committed snapshot.
func (s *Store) AuthenticateAgent(ctx context.Context, agentID, token string) error {
	_, err := read(s, func() (struct{}, error) { return struct{}{}, s.core.AuthenticateAgent(ctx, agentID, token) })
	return err
}

// Heartbeat commits the repository mutation before returning.
func (s *Store) Heartbeat(ctx context.Context, agentID string, at time.Time) (domain.Server, error) {
	return transact(s, func() (domain.Server, error) { return s.core.Heartbeat(ctx, agentID, at) })

}

// ReplaceInventory commits the repository mutation before returning.
func (s *Store) ReplaceInventory(ctx context.Context, agentID string, report domain.InventoryReport) (domain.Server, error) {
	return transact(s, func() (domain.Server, error) { return s.core.ReplaceInventory(ctx, agentID, report) })

}

// SetServerDrained commits the repository mutation before returning.
func (s *Store) SetServerDrained(ctx context.Context, serverID string, drained bool) (domain.Server, error) {
	return transact(s, func() (domain.Server, error) { return s.core.SetServerDrained(ctx, serverID, drained) })

}

// MarkStaleServers commits the repository mutation before returning.
func (s *Store) MarkStaleServers(ctx context.Context, staleBefore time.Time) (int, error) {
	return transact(s, func() (int, error) { return s.core.MarkStaleServers(ctx, staleBefore) })

}

// GetServer reads a consistent committed snapshot.
func (s *Store) GetServer(ctx context.Context, id string) (domain.Server, error) {
	return read(s, func() (domain.Server, error) { return s.core.GetServer(ctx, id) })

}

// ListServers reads a consistent committed snapshot.
func (s *Store) ListServers(ctx context.Context) ([]domain.Server, error) {
	return read(s, func() ([]domain.Server, error) { return s.core.ListServers(ctx) })

}

// CreateJob commits the repository mutation before returning.
func (s *Store) CreateJob(ctx context.Context, job domain.Job) error {
	_, err := transact(s, func() (struct{}, error) { return struct{}{}, s.core.CreateJob(ctx, job) })
	return err
}

// GetJob reads a consistent committed snapshot.
func (s *Store) GetJob(ctx context.Context, id string) (domain.Job, error) {
	return read(s, func() (domain.Job, error) { return s.core.GetJob(ctx, id) })

}

// ListJobs reads a consistent committed snapshot.
func (s *Store) ListJobs(ctx context.Context) ([]domain.Job, error) {
	return read(s, func() ([]domain.Job, error) { return s.core.ListJobs(ctx) })

}

// ListQueuedJobs reads a consistent committed snapshot.
func (s *Store) ListQueuedJobs(ctx context.Context) ([]domain.Job, error) {
	return read(s, func() ([]domain.Job, error) { return s.core.ListQueuedJobs(ctx) })

}

// SetJobStatus commits the repository mutation before returning.
func (s *Store) SetJobStatus(ctx context.Context, id string, status domain.JobStatus, reason string, at time.Time) (domain.Job, error) {
	return transact(s, func() (domain.Job, error) { return s.core.SetJobStatus(ctx, id, status, reason, at) })

}

// CommitAssignment commits the repository mutation before returning.
func (s *Store) CommitAssignment(ctx context.Context, jobID string, placement domain.Placement, command domain.Command, at time.Time) (domain.Job, error) {
	return transact(s, func() (domain.Job, error) { return s.core.CommitAssignment(ctx, jobID, placement, command, at) })

}

// EnqueueCommand commits the repository mutation before returning.
func (s *Store) EnqueueCommand(ctx context.Context, command domain.Command) error {
	_, err := transact(s, func() (struct{}, error) { return struct{}{}, s.core.EnqueueCommand(ctx, command) })
	return err
}

// RequestStop commits the repository mutation before returning.
func (s *Store) RequestStop(ctx context.Context, jobID string, command domain.Command, at time.Time) (domain.Job, error) {
	return transact(s, func() (domain.Job, error) { return s.core.RequestStop(ctx, jobID, command, at) })

}

// LeaseCommands commits the repository mutation before returning.
func (s *Store) LeaseCommands(ctx context.Context, agentID string, now time.Time, lease time.Duration, limit int) ([]domain.Command, error) {
	return transact(s, func() ([]domain.Command, error) { return s.core.LeaseCommands(ctx, agentID, now, lease, limit) })

}

// AckCommand commits the repository mutation before returning.
func (s *Store) AckCommand(ctx context.Context, agentID, commandID string, ack domain.CommandAckRequest, at time.Time) (domain.Command, error) {
	return transact(s, func() (domain.Command, error) { return s.core.AckCommand(ctx, agentID, commandID, ack, at) })

}
