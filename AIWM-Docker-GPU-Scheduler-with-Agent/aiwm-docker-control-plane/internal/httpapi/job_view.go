package httpapi

import (
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"time"
)

// JobView is a public read model. Secret environment values never leave the server API.
type JobView struct {
	ID             string                    `json:"id"`
	Name           string                    `json:"name"`
	Image          string                    `json:"image"`
	Backend        domain.ExecutionBackend   `json:"backend"`
	Command        []string                  `json:"command,omitempty"`
	Environment    map[string]string         `json:"environment,omitempty"`
	Resources      domain.ResourceRequest    `json:"resources"`
	Priority       int                       `json:"priority"`
	ServerSelector map[string]string         `json:"serverSelector,omitempty"`
	Strategy       domain.SchedulingStrategy `json:"strategy"`
	Status         domain.JobStatus          `json:"status"`
	StatusReason   string                    `json:"statusReason,omitempty"`
	Assignment     *domain.Assignment        `json:"assignment,omitempty"`
	CreatedAt      time.Time                 `json:"createdAt"`
	UpdatedAt      time.Time                 `json:"updatedAt"`
	LastObservedAt *time.Time                `json:"lastObservedAt,omitempty"`
	ContainerID    string                    `json:"containerId,omitempty"`
	Events         []domain.JobEvent         `json:"events"`
}

func publicJob(j domain.Job) JobView {
	environment := make(map[string]string, len(j.Environment))
	for key := range j.Environment {
		environment[key] = "[redacted]"
	}
	return JobView{ID: j.ID, Name: j.Name, Image: j.Image, Backend: j.Backend, Command: j.Command, Environment: environment,
		Resources: j.Resources, Priority: j.Priority, ServerSelector: j.ServerSelector, Strategy: j.Strategy, Status: j.Status,
		StatusReason: j.StatusReason, Assignment: j.Assignment, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt,
		LastObservedAt: j.LastObservedAt, ContainerID: j.ContainerID, Events: j.Events}
}
func publicJobs(jobs []domain.Job) []JobView {
	result := make([]JobView, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, publicJob(job))
	}
	return result
}
