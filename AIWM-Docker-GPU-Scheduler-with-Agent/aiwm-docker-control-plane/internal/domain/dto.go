package domain

import "time"

// InventoryReport and CommandAckRequest are internal application/persistence
// values. The external Agent wire contract lives in agentprotocol/v1.
type InventoryReport struct {
	Host          HostInfo     `json:"host"`
	Sequence      uint64       `json:"sequence"`
	ObservedAt    time.Time    `json:"observedAt"`
	GPUs          []GPU        `json:"gpus"`
	Containers    []Container  `json:"containers"`
	Processes     []GPUProcess `json:"processes,omitempty"`
	ReceivedAt    time.Time    `json:"-"`
	DockerVersion string       `json:"dockerVersion,omitempty"`
}

type CommandAckRequest struct {
	Succeeded   bool   `json:"succeeded"`
	Message     string `json:"message,omitempty"`
	ContainerID string `json:"containerId,omitempty"`
}

type CreateJobRequest struct {
	Name           string             `json:"name"`
	Image          string             `json:"image"`
	Backend        ExecutionBackend   `json:"backend,omitempty"`
	Command        []string           `json:"command,omitempty"`
	Environment    map[string]string  `json:"environment,omitempty"`
	Resources      ResourceRequest    `json:"resources"`
	Priority       int                `json:"priority"`
	ServerSelector map[string]string  `json:"serverSelector,omitempty"`
	Strategy       SchedulingStrategy `json:"strategy,omitempty"`
}

type DrainServerRequest struct {
	Drained bool `json:"drained"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type APIResponse struct {
	Data  any       `json:"data,omitempty"`
	Error *APIError `json:"error,omitempty"`
	Meta  any       `json:"meta,omitempty"`
}
