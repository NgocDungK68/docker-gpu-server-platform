package domain

import (
	"strings"
	"time"
)

// AllocationIntent contains user-declared business needs, never placement decisions.
type AllocationIntent struct {
	WorkloadType         WorkloadType     `json:"workloadType"`
	NecessityLevel       NecessityLevel   `json:"necessityLevel"`
	NecessityReason      string           `json:"necessityReason"`
	NecessityExplanation string           `json:"necessityExplanation,omitempty"`
	SystemImportance     SystemImportance `json:"systemImportance"`
	NeededAt             string           `json:"neededAt"`
	TTLSeconds           int64            `json:"ttlSeconds"`
}
type WorkloadType string

const (
	WorkloadTraining  WorkloadType = "TRAINING"
	WorkloadInference WorkloadType = "INFERENCE"
)

type NecessityLevel string

const (
	Necessity1 NecessityLevel = "NECESSITY_1"
	Necessity2 NecessityLevel = "NECESSITY_2"
	Necessity3 NecessityLevel = "NECESSITY_3"
	Necessity4 NecessityLevel = "NECESSITY_4"
)

type SystemImportance string

const (
	ImportanceCritical      SystemImportance = "CRITICAL_SPECIAL"
	ImportanceVeryImportant SystemImportance = "VERY_IMPORTANT"
	ImportanceImportant     SystemImportance = "IMPORTANT"
	ImportanceNormal        SystemImportance = "NORMAL"
)

type PolicyStatus string

const (
	PolicyAutoEligible PolicyStatus = "AUTO_ELIGIBLE"
	PolicyCompetitive  PolicyStatus = "COMPETITIVE"
)

// PolicyDecision is a point-in-time assessment, separate from Job.Status.
type PolicyDecision struct {
	Status         PolicyStatus `json:"status"`
	Reason         string       `json:"reason"`
	InSizingPlan   bool         `json:"inSizingPlan"`
	WithinQuota    bool         `json:"withinQuota"`
	QuotaUsage     int          `json:"quotaUsage"`
	Source         string       `json:"source"`
	EvaluatedAt    time.Time    `json:"evaluatedAt"`
	AuxiliaryScore float64      `json:"-"`
}

// AllocationResources is the public constraint DTO. Internal selectors cannot be decoded into it.
type AllocationResources struct {
	GPUCount           int    `json:"gpuCount"`
	MinVRAMMiB         int64  `json:"minVramMiB"`
	PerformanceProfile string `json:"performanceProfile"`
	FP8Required        *bool  `json:"fp8Required"`
	CPUMilli           int64  `json:"cpuMilli,omitempty"`
	MemoryMiB          int64  `json:"memoryMiB,omitempty"`
	AllowSharedGPU     bool   `json:"allowSharedGpu,omitempty"`
}

func (r AllocationResources) Domain() ResourceRequest {
	return ResourceRequest{GPUCount: r.GPUCount, MinVRAMMiB: r.MinVRAMMiB, PerformanceProfile: r.PerformanceProfile,
		FP8Required: r.FP8Required != nil && *r.FP8Required, CPUMilli: r.CPUMilli, MemoryMiB: r.MemoryMiB}
}

// CapabilityMatches is shared by planning and atomic commit. Resolved models are
// trusted backend data pinned at admission, persisted with the immutable request.
func (r ResourceRequest) CapabilityMatches(g GPU) bool {
	// AUTO has no model restriction unless a known FP8 capability is required.
	if r.PerformanceProfile == "AUTO" && !r.FP8Required {
		return true
	}
	if r.PerformanceProfile != "" || r.FP8Required {
		for _, model := range r.ResolvedModels {
			if strings.EqualFold(strings.TrimSpace(g.Model), model) {
				return true
			}
		}
		return false
	}
	return r.GPUModel == "" || strings.EqualFold(g.Model, r.GPUModel) // legacy persisted jobs / benchmark
}
func (r ResourceRequest) Matches(g GPU) bool {
	return g.Schedulable() && r.CapabilityMatches(g) && g.AvailableMemoryMiB() >= r.MinVRAMMiB
}

// ValidationError exposes safe field-specific messages, without echoing input values.
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "Dữ liệu yêu cầu chưa hợp lệ" }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }
