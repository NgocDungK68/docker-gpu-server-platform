package application

import (
	"context"
	"errors"
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
	p,authenticated:=domain.CurrentPrincipal(ctx)
	if c.metadata!=nil && !authenticated { return domain.Job{},domain.ErrUnauthorized }
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
	r.NeededAt = needed.UTC().Format(time.RFC3339)
	now := c.now().UTC()
	job := domain.Job{OrganizationID:p.User.OrganizationID,AllocationIntent: r.AllocationIntent, Name: strings.TrimSpace(r.Name), Image: r.Image, Backend: r.Backend,
		Command: r.Command, Environment: r.Environment, Resources: resources, Strategy: c.scheduler.defaultStrategy,
		Status: domain.JobQueued, CreatedAt: now, UpdatedAt: now}
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
	return JobPreview{job.WorkloadType, job.Policy.Status, job.Policy.Reason, policy.NecessityLabel(job.NecessityLevel),
		sizing, quota, job.Policy.Source, match, job.Policy.EvaluatedAt}, nil
}
func (c *ControlPlane) matchJob(ctx context.Context, job domain.Job) (ResourceMatch, error) {
	servers, err := c.repository.ListServers(ctx)
	if err != nil {
		return ResourceMatch{}, err
	}
	now := c.now().UTC()
	result := ResourceMatch{RecommendedGPUModels: []string{}}
	models := map[string]bool{}
	for _, server := range servers {
		if !domain.SameOrganization(job.OrganizationID,server.OrganizationID) { continue }
		if !server.Schedulable(now, c.offlineAfter) || !labelsMatch(server.Labels, job.ServerSelector) {
			continue
		}
		matching := matchingGPUs(server.GPUs, job.Resources)
		if len(matching) > result.MatchedGPUCount {
			result.MatchedGPUCount = len(matching)
		}
		for _, gpu := range matching {
			models[gpu.Model] = true
		}
	}
	for model := range models {
		result.RecommendedGPUModels = append(result.RecommendedGPUModels, model)
	}
	sort.Strings(result.RecommendedGPUModels)
	_, err = c.scheduler.Plan(job, servers, now)
	if errors.Is(err, domain.ErrInsufficientGPU) {
		result.UnsatisfiedReason = friendlyPlacementReason(err.Error())
		result.RecommendationText = "Chờ tài nguyên: " + result.UnsatisfiedReason
		return result, nil
	}
	if err != nil {
		return ResourceMatch{}, err
	}
	result.Satisfiable = true
	result.RecommendationText = fmt.Sprintf("Có thể đáp ứng %d GPU trên một server; cấp phát theo thứ tự hàng đợi.", job.Resources.GPUCount)
	return result, nil
}
func friendlyPlacementReason(reason string) string {
	for _, item := range [][2]string{
		{"organization ownership", "Chưa có server thuộc organization của tài khoản."},
		{"no online agent", "Chưa có server online với inventory mới, hoặc server đang drain."},
		{"model unavailable", "Chưa có GPU đáp ứng profile hiệu năng và yêu cầu FP8."},
		{"no healthy", "GPU phù hợp chưa healthy."},
		{"external workload", "Workload External đang chiếm GPU; cần chờ GPU khác."},
		{"ownership unknown", "GPU có occupancy chưa xác định nên chưa được cấp phát."},
		{"reserved or allocated", "GPU phù hợp đang được reserve hoặc cấp phát."},
		{"VRAM", "Chưa đủ VRAM khả dụng trên mỗi GPU."},
	} {
		if strings.Contains(reason, item[0]) {
			return item[1]
		}
	}
	return "Chưa đủ số GPU phù hợp trên cùng một server."
}
