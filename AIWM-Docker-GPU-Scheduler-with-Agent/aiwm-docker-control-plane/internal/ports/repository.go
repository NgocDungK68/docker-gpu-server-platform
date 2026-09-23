package ports

import (
	"context"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

// Repository is the persistence boundary of the control plane. The in-memory
// implementation is used for the MVP; a PostgreSQL adapter can replace it
// without changing HTTP handlers, scheduling policy, or agent protocol.
type Repository interface {
	// ReplanReservations runs a pure planning callback on a consistent snapshot and
	// commits all future reservation changes atomically. Callback must not call Repository.
	ReplanReservations(ctx context.Context, at time.Time, plan ReservationPlanner) (int, error)
	UpsertServer(ctx context.Context, server domain.Server) (domain.Server, error)
	AuthenticateAgent(ctx context.Context, agentID, token string) error
	Heartbeat(ctx context.Context, agentID string, at time.Time) (domain.Server, error)
	ReplaceInventory(ctx context.Context, agentID string, report domain.InventoryReport) (domain.Server, error)
	SetServerDrained(ctx context.Context, serverID string, drained bool) (domain.Server, error)
	MarkStaleServers(ctx context.Context, staleBefore time.Time) (int, error)
	GetServer(ctx context.Context, id string) (domain.Server, error)
	ListServers(ctx context.Context) ([]domain.Server, error)

	CreateJob(ctx context.Context, job domain.Job) error
	GetJob(ctx context.Context, id string) (domain.Job, error)
	ListJobs(ctx context.Context) ([]domain.Job, error)
	ListQueuedJobs(ctx context.Context) ([]domain.Job, error)
	SetJobStatus(ctx context.Context, id string, status domain.JobStatus, reason string, at time.Time) (domain.Job, error)

	// CommitAssignment must reserve all selected GPUs, update the job, and
	// enqueue the command atomically. It is the double-allocation guard.
	CommitAssignment(ctx context.Context, jobID string, placement domain.Placement, command domain.Command, at time.Time) (domain.Job, error)
	EnqueueCommand(ctx context.Context, command domain.Command) error
	RequestStop(ctx context.Context, jobID string, command domain.Command, at time.Time) (domain.Job, error)
	LeaseCommands(ctx context.Context, agentID string, now time.Time, lease time.Duration, limit int) ([]domain.Command, error)
	AckCommand(ctx context.Context, agentID, commandID string, ack domain.CommandAckRequest, at time.Time) (domain.Command, error)
}

type ReservationPlanner func(jobs []domain.Job, servers []domain.Server) ([]domain.ReservationPlan, error)
