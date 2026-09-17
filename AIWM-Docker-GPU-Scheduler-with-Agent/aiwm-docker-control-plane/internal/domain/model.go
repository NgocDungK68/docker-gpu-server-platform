package domain

import (
	"encoding/json"
	"time"
)

type ServerStatus string

const (
	ServerOnline   ServerStatus = "ONLINE"
	ServerOffline  ServerStatus = "OFFLINE"
	ServerDraining ServerStatus = "DRAINING"
)

type GPUState string

const (
	GPUFree            GPUState = "FREE"
	GPUReserved        GPUState = "RESERVED"
	GPUAllocated       GPUState = "ALLOCATED"
	GPUOccupiedLegacy  GPUState = "OCCUPIED_LEGACY"
	GPUOccupiedUnknown GPUState = "OCCUPIED_UNKNOWN"
	GPUUnhealthy       GPUState = "UNHEALTHY"
)

type ContainerOrigin string

const (
	ContainerManaged ContainerOrigin = "MANAGED"
	ContainerLegacy  ContainerOrigin = "LEGACY"
	ContainerUnknown ContainerOrigin = "UNKNOWN"
)

type JobStatus string

const (
	JobQueued    JobStatus = "QUEUED"
	JobAssigned  JobStatus = "ASSIGNED"
	JobStarting  JobStatus = "STARTING"
	JobRunning   JobStatus = "RUNNING"
	JobStopping  JobStatus = "STOPPING"
	JobStopped   JobStatus = "STOPPED"
	JobSucceeded JobStatus = "SUCCEEDED"
	JobFailed    JobStatus = "FAILED"
	JobCancelled JobStatus = "CANCELLED"
)

type CommandType string

const (
	CommandStartContainer CommandType = "START_CONTAINER"
	CommandStopContainer  CommandType = "STOP_CONTAINER"
)

type CommandStatus string

const (
	CommandPending   CommandStatus = "PENDING"
	CommandDelivered CommandStatus = "DELIVERED"
	CommandSucceeded CommandStatus = "SUCCEEDED"
	CommandFailed    CommandStatus = "FAILED"
)

type SchedulingStrategy string

const (
	StrategyFirstFit      SchedulingStrategy = "first-fit"
	StrategyBestFit       SchedulingStrategy = "best-fit"
	StrategyBinPack       SchedulingStrategy = "bin-pack"
	StrategyFragmentation SchedulingStrategy = "fragmentation-aware"
)

type ExecutionBackend string

const (
	BackendDocker     ExecutionBackend = "DOCKER"
	BackendKubernetes ExecutionBackend = "KUBERNETES"
)

type Server struct {
	OrganizationID      string            `json:"organizationId"`
	SchedulingReady     bool              `json:"schedulable"`
	SchedulingReason    string            `json:"schedulingReason"`
	Host                HostInfo          `json:"host"`
	ID                  string            `json:"id"`
	MachineID           string            `json:"machineId"`
	Name                string            `json:"name"`
	Address             string            `json:"address,omitempty"`
	AgentVersion        string            `json:"agentVersion,omitempty"`
	Labels              map[string]string `json:"labels,omitempty"`
	Status              ServerStatus      `json:"status"`
	Drained             bool              `json:"drained"`
	LastHeartbeatAt     time.Time         `json:"lastHeartbeatAt"`
	LastInventoryAt     time.Time         `json:"lastInventoryAt,omitempty"`
	InventoryReceivedAt time.Time         `json:"inventoryReceivedAt,omitempty"`
	DockerVersion       string            `json:"dockerVersion,omitempty"`
	InventoryVersion    uint64            `json:"inventoryVersion"`
	GPUs                []GPU             `json:"gpus"`
	Containers          []Container       `json:"containers"`
	TokenHash           [32]byte          `json:"-"`
}

// HostInfo describes observed machine hardware independently from its scheduling state.
type HostInfo struct {
	Hostname       string `json:"hostname"`
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	CPUCount       int    `json:"cpuCount"`
	MemoryTotalMiB int64  `json:"memoryTotalMiB"`
}

// Schedulable requires fresh connectivity and a complete recent inventory.
func (s Server) Schedulable(now time.Time, offlineAfter time.Duration) bool {
	return s.Status == ServerOnline && !s.Drained && !s.InventoryReceivedAt.IsZero() &&
		now.Sub(s.LastHeartbeatAt) <= offlineAfter && now.Sub(s.InventoryReceivedAt) <= offlineAfter
}

type GPU struct {
	UUID              string   `json:"uuid"`
	Index             int      `json:"index"`
	Model             string   `json:"model"`
	MemoryTotalMiB    int64    `json:"memoryTotalMiB"`
	MemoryUsedMiB     int64    `json:"memoryUsedMiB"`
	UtilizationPct    float64  `json:"utilizationPct"`
	TemperatureC      float64  `json:"temperatureC,omitempty"`
	Healthy           bool     `json:"healthy"`
	State             GPUState `json:"state"`
	AssignedJobID     string   `json:"assignedJobId,omitempty"`
	ObservedConsumers []string `json:"observedConsumers,omitempty"`
	StateReason       string   `json:"stateReason,omitempty"`
}

func (g GPU) AvailableMemoryMiB() int64 {
	remaining := g.MemoryTotalMiB - g.MemoryUsedMiB
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (g GPU) Schedulable() bool {
	return g.Healthy && g.State == GPUFree && g.AssignedJobID == ""
}

type Container struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Image      string            `json:"image"`
	State      string            `json:"state"`
	Origin     ContainerOrigin   `json:"origin"`
	JobID      string            `json:"jobId,omitempty"`
	GPUUUIDs   []string          `json:"gpuUuids,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	StartedAt  *time.Time        `json:"startedAt,omitempty"`
	FinishedAt *time.Time        `json:"finishedAt,omitempty"`
	ExitCode   *int              `json:"exitCode,omitempty"`
}

type GPUProcess struct {
	PID           int    `json:"pid"`
	GPUUUID       string `json:"gpuUuid"`
	UsedMemoryMiB int64  `json:"usedMemoryMiB"`
	ContainerID   string `json:"containerId,omitempty"`
	ProcessName   string `json:"processName,omitempty"`
}

type Job struct {
	OrganizationID string `json:"organizationId"`
	AllocationIntent
	Policy         PolicyDecision     `json:"policy"`
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Image          string             `json:"image"`
	Backend        ExecutionBackend   `json:"backend"`
	Command        []string           `json:"command,omitempty"`
	Environment    map[string]string  `json:"environment,omitempty"`
	Resources      ResourceRequest    `json:"resources"`
	Priority       int                `json:"priority"`
	ServerSelector map[string]string  `json:"serverSelector,omitempty"`
	Strategy       SchedulingStrategy `json:"strategy"`
	Status         JobStatus          `json:"status"`
	StatusReason   string             `json:"statusReason,omitempty"`
	Assignment     *Assignment        `json:"assignment,omitempty"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
	LastObservedAt *time.Time         `json:"lastObservedAt,omitempty"`
	ContainerID    string             `json:"containerId,omitempty"`
	Events         []JobEvent         `json:"events"`
}

type ResourceRequest struct {
	PerformanceProfile string   `json:"performanceProfile,omitempty"`
	FP8Required        bool     `json:"fp8Required"`
	ResolvedModels     []string `json:"-"`
	GPUCount           int      `json:"gpuCount"`
	GPUModel           string   `json:"gpuModel,omitempty"`
	MinVRAMMiB         int64    `json:"minVramMiB,omitempty"`
	CPUMilli           int64    `json:"cpuMilli,omitempty"`
	MemoryMiB          int64    `json:"memoryMiB,omitempty"`
	AllowSharedGPU     bool     `json:"allowSharedGpu"`
}

type Assignment struct {
	ServerID         string             `json:"serverId"`
	GPUUUIDs         []string           `json:"gpuUuids"`
	CommandID        string             `json:"commandId"`
	AssignedAt       time.Time          `json:"assignedAt"`
	ReservationState string             `json:"reservationState"`
	ReleasedAt       *time.Time         `json:"releasedAt,omitempty"`
	Strategy         SchedulingStrategy `json:"strategy"`
	Score            float64            `json:"score"`
	Reason           string             `json:"reason"`
}

type Command struct {
	ID          string          `json:"id"`
	AgentID     string          `json:"agentId"`
	Type        CommandType     `json:"type"`
	Status      CommandStatus   `json:"status"`
	Payload     json.RawMessage `json:"payload"`
	Attempts    int             `json:"attempts"`
	CreatedAt   time.Time       `json:"createdAt"`
	DeliveredAt *time.Time      `json:"deliveredAt,omitempty"`
	LeaseUntil  *time.Time      `json:"leaseUntil,omitempty"`
	CompletedAt *time.Time      `json:"completedAt,omitempty"`
	Error       string          `json:"error,omitempty"`
}

type Placement struct {
	Policy   PolicyDecision     `json:"-"`
	ServerID string             `json:"serverId"`
	GPUUUIDs []string           `json:"gpuUuids"`
	Strategy SchedulingStrategy `json:"strategy"`
	Score    float64            `json:"score"`
	Reason   string             `json:"reason"`
}

type ClusterSummary struct {
	ServersOffline int `json:"serversOffline"`
	GPUsReserved int `json:"gpusReserved"`
	GPUsAllocated int `json:"gpusAllocated"`
	GPUsUnhealthy int `json:"gpusUnhealthy"`
	ServersTotal  int        `json:"serversTotal"`
	ServersOnline int        `json:"serversOnline"`
	GPUsTotal     int        `json:"gpusTotal"`
	GPUsFree      int        `json:"gpusFree"`
	GPUsLegacy    int        `json:"gpusOccupiedLegacy"`
	GPUsUnknown   int        `json:"gpusOccupiedUnknown"`
	JobsQueued    int        `json:"jobsQueued"`
	JobsRunning   int        `json:"jobsRunning"`
	GPUsOccupied  int        `json:"gpusOccupied"`
	RecentEvents  []JobEvent `json:"recentEvents"`
}

// JobEvent records a lifecycle transition without including command/env secrets.
type JobEvent struct {
	JobID  string    `json:"jobId"`
	Status JobStatus `json:"status"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// Terminal means a job can no longer launch or return to the queue.
func (s JobStatus) Terminal() bool {
	return s == JobStopped || s == JobSucceeded || s == JobFailed || s == JobCancelled
}

// ValidStrategy enumerates the supported whole-GPU placement policies.
func ValidStrategy(s SchedulingStrategy) bool {
	return s == StrategyFirstFit || s == StrategyBestFit || s == StrategyBinPack || s == StrategyFragmentation
}
