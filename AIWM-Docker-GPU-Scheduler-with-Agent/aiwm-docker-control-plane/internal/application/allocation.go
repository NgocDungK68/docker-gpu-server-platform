package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/capability"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/policy"
)

type AllocationOptions struct {
	Limits              RequestLimits             `json:"limits"`
	PerformanceProfiles []capability.Profile      `json:"performanceProfiles"`
	NecessityProfiles   []policy.NecessityProfile `json:"necessityProfiles"`
	SystemImportance    []policy.Option           `json:"systemImportance"`
	WorkloadTypes       []policy.Option           `json:"workloadTypes"`
	CustomReason        policy.Option             `json:"customReason"`
}

func (c *ControlPlane) AllocationOptions() AllocationOptions {
	return AllocationOptions{c.limits, c.catalog.Profiles(), policy.Profiles(), policy.ImportanceOptions(),
		[]policy.Option{{ID: "TRAINING", Label: "Training"}, {ID: "INFERENCE", Label: "Inference"}}, policy.Option{ID: "CUSTOM", Label: "Lý do khác (cần giải trình)"}}
}

type ResourceMatch struct {
	Satisfiable          bool     `json:"satisfiable"`
	RecommendedGPUModels []string `json:"recommendedGpuModels"`
	MatchedGPUCount      int      `json:"matchedGpuCount"`
	RecommendationText   string   `json:"recommendationText"`
	UnsatisfiedReason    string   `json:"unsatisfiedReason,omitempty"`
}
type JobPreview struct {
	domain.TimeWindow
	PlanningStatus   string              `json:"planningStatus"`
	WorkloadType     domain.WorkloadType `json:"workloadType"`
	PolicyStatus     domain.PolicyStatus `json:"policyStatus"`
	PolicyReason     string              `json:"policyReason"`
	NecessityLabel   string              `json:"necessityLabel"`
	SizingPlanStatus string              `json:"sizingPlanStatus"`
	QuotaStatus      string              `json:"quotaStatus"`
	PolicySource     string              `json:"policySource"`
	ResourceMatch    ResourceMatch       `json:"resourceMatch"`
	EvaluatedAt      time.Time           `json:"evaluatedAt"`
}

// prepareJob is shared by preview and submit; client evaluation results are never accepted.
func (c *ControlPlane) prepareJob(ctx context.Context, r domain.CreateJobRequest) (domain.Job, error) {
	p, authenticated := domain.CurrentPrincipal(ctx)
	if c.metadata != nil && !authenticated {
		return domain.Job{}, domain.ErrUnauthorized
	}
	if r.Backend == "" {
		r.Backend = domain.BackendDocker
	}
	if err := validateJobRequest(r, c.limits, c.catalog); err != nil {
		return domain.Job{}, err
	}
	resources := r.Resources.Domain()
	models, err := c.catalog.Resolve(resources.PerformanceProfile, resources.FP8Required)
	if err != nil {
		return domain.Job{}, err
	}
	resources.ResolvedModels = models
	needed, _ := time.Parse(time.RFC3339, r.NeededAt)
	r.NeededAt = needed.UTC().Format(time.RFC3339Nano)
	now := c.now().UTC()
	if needed.Before(now.Add(-time.Minute)) {
		return domain.Job{}, &domain.ValidationError{Fields: map[string]string{"neededAt": "Thời điểm bắt đầu không được ở quá khứ quá 60 giây"}}
	}
	job := domain.Job{OrganizationID: p.User.OrganizationID, AllocationIntent: r.AllocationIntent, Name: strings.TrimSpace(r.Name), Image: r.Image, Backend: r.Backend,
		Command: r.Command, Environment: r.Environment, Resources: resources, Strategy: c.scheduler.defaultStrategy,
		Status: domain.JobQueued, CreatedAt: now, UpdatedAt: now}
	window := job.RequestedWindow()
	if !window.Valid() || !now.Before(window.EndAt) {
		return domain.Job{}, &domain.ValidationError{Fields: map[string]string{"ttlSeconds": "Khoảng thời gian sử dụng phải hợp lệ và chưa kết thúc"}}
	}
	job.Policy, err = c.policy.Evaluate(ctx, job, now)
	return job, err
}

// PreviewJob only reads inventory; it creates no ID, job, command or reservation.
func (c *ControlPlane) PreviewJob(ctx context.Context, r domain.CreateJobRequest) (JobPreview, error) {
	job, err := c.prepareJob(ctx, r)
	if err != nil {
		return JobPreview{}, err
	}
	match, err := c.matchJob(ctx, job)
	if err != nil {
		return JobPreview{}, err
	}
	sizing, quota := "Ngoài Quy hoạch định cỡ", "Vượt hạn mức hiện tại"
	if job.Policy.InSizingPlan {
		sizing = "Trong Quy hoạch định cỡ"
	}
	if job.Policy.WithinQuota {
		quota = "Trong hạn mức hiện tại"
	}
	planningStatus := "CONFLICT"
	if match.Satisfiable {
		planningStatus = "AVAILABLE"
	}
	return JobPreview{job.RequestedWindow(), planningStatus, job.WorkloadType, job.Policy.Status, job.Policy.Reason, policy.NecessityLabel(job.NecessityLevel),
		sizing, quota, job.Policy.Source, match, job.Policy.EvaluatedAt}, nil
}

// matchJob uses the same greedy planner without persisting its hypothetical result.
func (c *ControlPlane) matchJob(ctx context.Context, job domain.Job) (ResourceMatch, error) {
	servers, err := c.repository.ListServers(ctx)
	if err != nil {
		return ResourceMatch{}, err
	}
	jobs, err := c.repository.ListJobs(ctx)
	if err != nil {
		return ResourceMatch{}, err
	}
	enabled, err := c.enabledOrganizations(ctx)
	if err != nil {
		return ResourceMatch{}, err
	}
	job.ID = "preview-candidate" // Stored IDs always use the job- prefix.
	jobs = append(jobs, job)
	plans, err := c.planReservations(ctx, jobs, servers, enabled, c.now().UTC())
	if err != nil {
		return ResourceMatch{}, err
	}
	result := ResourceMatch{RecommendedGPUModels: []string{}}
	for _, entry := range plans {
		if entry.JobID != job.ID {
			continue
		}
		if entry.Placement == nil {
			result.UnsatisfiedReason = entry.Reason
			result.RecommendationText = entry.Reason
			return result, nil
		}
		result.Satisfiable = true
		result.MatchedGPUCount = len(entry.Placement.GPUUUIDs)
		models := map[string]bool{}
		for _, server := range servers {
			if server.ID != entry.Placement.ServerID {
				continue
			}
			for _, gpu := range server.GPUs {
				for _, uuid := range entry.Placement.GPUUUIDs {
					if gpu.UUID == uuid {
						models[gpu.Model] = true
					}
				}
			}
		}
		for model := range models {
			result.RecommendedGPUModels = append(result.RecommendedGPUModels, model)
		}
		sort.Strings(result.RecommendedGPUModels)
		result.RecommendationText = fmt.Sprintf("Có thể giữ %d GPU trong khoảng thời gian yêu cầu; chỉ chạy khi đến giờ và kiểm tra lại tài nguyên.", job.Resources.GPUCount)
		return result, nil
	}
	result.UnsatisfiedReason = waitingWindow
	result.RecommendationText = waitingWindow
	return result, nil
}
