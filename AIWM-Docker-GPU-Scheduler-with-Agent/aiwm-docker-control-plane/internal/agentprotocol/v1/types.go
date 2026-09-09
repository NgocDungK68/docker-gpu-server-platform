// Package v1 defines the versioned wire contract between the AIWM Control
// Plane and Agents. Keep these DTOs independent from persistence/domain types:
// an Agent binary and a Control Plane binary may be upgraded separately.
package v1

import (
	"encoding/json"
	"time"
)

const (
	ProtocolVersion       = "v1"
	EnrollmentTokenHeader = "X-Enrollment-Token"
	AuthorizationHeader   = "Authorization"
	ManagedLabel          = "aiwm.managed"
	JobIDLabel            = "aiwm.job-id"
)

type RegisterRequest struct {
	ProtocolVersion string            `json:"protocolVersion"`
	MachineID       string            `json:"machineId"`
	Name            string            `json:"name"`
	Address         string            `json:"address,omitempty"`
	AgentVersion    string            `json:"agentVersion,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Capabilities    []string          `json:"capabilities,omitempty"`
}

type RegisterResponse struct {
	AgentID                  string `json:"agentId"`
	AgentToken               string `json:"agentToken"`
	HeartbeatIntervalSeconds int    `json:"heartbeatIntervalSeconds"`
}

type HeartbeatRequest struct {
	ObservedAt time.Time `json:"observedAt"`
}

type InventoryReport struct {
	Host          HostInfo     `json:"host"`
	Sequence      uint64       `json:"sequence"`
	ObservedAt    time.Time    `json:"observedAt"`
	DockerVersion string       `json:"dockerVersion,omitempty"`
	GPUs          []GPU        `json:"gpus"`
	Containers    []Container  `json:"containers"`
	Processes     []GPUProcess `json:"processes,omitempty"`
}

// HostInfo is a hardware observation, never a desired resource allocation.
type HostInfo struct {
	Hostname       string `json:"hostname"`
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	CPUCount       int    `json:"cpuCount"`
	MemoryTotalMiB int64  `json:"memoryTotalMiB"`
}

type GPU struct {
	UUID           string  `json:"uuid"`
	Index          int     `json:"index"`
	Model          string  `json:"model"`
	MemoryTotalMiB int64   `json:"memoryTotalMiB"`
	MemoryUsedMiB  int64   `json:"memoryUsedMiB"`
	UtilizationPct float64 `json:"utilizationPct"`
	TemperatureC   float64 `json:"temperatureC,omitempty"`
	Healthy        bool    `json:"healthy"`
	// State is only a conservative Agent observation. The Control Plane is
	// authoritative and recomputes FREE/LEGACY/UNKNOWN/ALLOCATED.
	State string `json:"state,omitempty"`
}

type Container struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Image      string            `json:"image"`
	State      string            `json:"state"`
	Origin     string            `json:"origin"`
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

type Command struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type StartContainerPayload struct {
	JobID       string            `json:"jobId"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	GPUUUIDs    []string          `json:"gpuUuids"`
	CPUMilli    int64             `json:"cpuMilli,omitempty"`
	MemoryMiB   int64             `json:"memoryMiB,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type StopContainerPayload struct {
	JobID        string `json:"jobId"`
	ContainerID  string `json:"containerId,omitempty"`
	GraceSeconds int    `json:"graceSeconds"`
}

type CommandAckRequest struct {
	Succeeded   bool   `json:"succeeded"`
	Message     string `json:"message,omitempty"`
	ContainerID string `json:"containerId,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Response[T any] struct {
	Data  T         `json:"data"`
	Error *APIError `json:"error,omitempty"`
}
