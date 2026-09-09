package agent

import (
	"context"
	"errors"
	"time"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

var (
	ErrUnauthorized = errors.New("agent authentication rejected")
	ErrUnsafeGPU    = errors.New("requested GPU is not safely available")
	ErrLegacyTarget = errors.New("refusing to control a legacy container")
)

type RuntimeContainer struct {
	ID                 string
	Name               string
	Image              string
	State              string
	Labels             map[string]string
	GPUUUIDs           []string
	GPUIndexes         []int
	UnboundedGPUAccess bool
	InitPID            int
	StartedAt          *time.Time
	FinishedAt         *time.Time
	ExitCode           *int
}

type DockerRuntime interface {
	Ping(ctx context.Context) error
	Version(ctx context.Context) (string, error)
	ListContainers(ctx context.Context) ([]RuntimeContainer, error)
	StartManagedContainer(ctx context.Context, payload agentv1.StartContainerPayload) (string, error)
	StopManagedJob(ctx context.Context, payload agentv1.StopContainerPayload) (string, error)
	WatchContainerEvents(ctx context.Context) (<-chan struct{}, <-chan error)
	Close() error
}

type GPUReader interface {
	Snapshot(ctx context.Context) ([]agentv1.GPU, []agentv1.GPUProcess, error)
	Close() error
}

type InventorySource interface {
	Snapshot(ctx context.Context, sequence uint64) (agentv1.InventoryReport, error)
}

type ControlPlaneClient interface {
	Register(ctx context.Context, request agentv1.RegisterRequest, enrollmentToken string) (agentv1.RegisterResponse, error)
	Heartbeat(ctx context.Context, agentID, token string, request agentv1.HeartbeatRequest) error
	ReportInventory(ctx context.Context, agentID, token string, report agentv1.InventoryReport) error
	PollCommands(ctx context.Context, agentID, token string, limit int) ([]agentv1.Command, error)
	AckCommand(ctx context.Context, agentID, token, commandID string, ack agentv1.CommandAckRequest) error
}

type PersistentState struct {
	AgentID           string                   `json:"agentId,omitempty"`
	AgentToken        string                   `json:"agentToken,omitempty"`
	MachineID         string                   `json:"machineId"`
	InventorySequence uint64                   `json:"inventorySequence"`
	ProcessedCommands map[string]CommandResult `json:"processedCommands,omitempty"`
}

type CommandResult struct {
	Ack         agentv1.CommandAckRequest `json:"ack"`
	CompletedAt time.Time                 `json:"completedAt"`
}

type StateStore interface {
	Load() (PersistentState, error)
	Save(PersistentState) error
}
