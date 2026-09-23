package httpapi

import (
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/policy"
	"strings"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

// JobView is a public read model. Secret environment values never leave the server API.
type JobView struct {
	domain.TimeWindow
	OrganizationID string `json:"organizationId"`
	NecessityLabel string `json:"necessityLabel"`
	domain.AllocationIntent
	Policy         domain.PolicyDecision     `json:"policy"`
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
	// Giữ chi tiết score trong persistence; không đưa score qua status/event text ra UI.
	j.StatusReason = publicReason(j.StatusReason)
	events := append([]domain.JobEvent{}, j.Events...)
	for i := range events {
		events[i].Reason = publicReason(events[i].Reason)
	}
	j.Events = events
	environment := make(map[string]string, len(j.Environment))
	for key := range j.Environment {
		environment[key] = "[redacted]"
	}
	return JobView{TimeWindow: j.RequestedWindow(), OrganizationID: j.OrganizationID, NecessityLabel: policy.NecessityLabel(j.NecessityLevel), AllocationIntent: j.AllocationIntent, Policy: j.Policy, ID: j.ID, Name: j.Name, Image: j.Image, Backend: j.Backend, Command: j.Command, Environment: environment,
		Resources: j.Resources, Priority: j.Priority, ServerSelector: j.ServerSelector, Strategy: j.Strategy, Status: j.Status,
		StatusReason: j.StatusReason, Assignment: j.Assignment, CreatedAt: j.CreatedAt, UpdatedAt: j.UpdatedAt,
		LastObservedAt: j.LastObservedAt, ContainerID: j.ContainerID, Events: j.Events}
}

func publicReason(reason string) string {
	if strings.Contains(reason, ", score ") {
		return "Đã chọn server trong đơn vị và reserve GPU; chờ Agent xác nhận runtime."
	}
	return reason
}
func publicJobs(jobs []domain.Job) []JobView {
	result := make([]JobView, 0, len(jobs))
	for _, job := range jobs {
		result = append(result, publicJob(job))
	}
	return result
}
