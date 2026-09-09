package policy

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

// FactsProvider is the integration boundary for planning/quota systems.
type FactsProvider interface {
	Facts(context.Context, domain.Job) (Facts, error)
}
type Facts struct {
	InSizingPlan, WithinQuota bool
	QuotaUsage                int
	Source                    string
}

// Evaluator can replace the corporate policy without changing placement or HTTP.
type Evaluator interface {
	Evaluate(context.Context, domain.Job, time.Time) (domain.PolicyDecision, error)
	Order(context.Context, []domain.Job, time.Time) ([]domain.Job, error)
}
type Engine struct{ facts FactsProvider }

func New(facts FactsProvider) *Engine { return &Engine{facts: facts} }

// DevelopmentFacts is deliberately static and does not implement quota accounting.
type DevelopmentFacts struct {
	InSizingPlan        bool
	QuotaGPUs, UsedGPUs int
}

func (p DevelopmentFacts) Facts(_ context.Context, j domain.Job) (Facts, error) {
	return Facts{p.InSizingPlan, j.Resources.GPUCount <= p.QuotaGPUs-p.UsedGPUs, p.UsedGPUs, "DEVELOPMENT_CONFIG"}, nil
}
func ImportanceCoefficient(value domain.SystemImportance) (float64, bool) {
	switch value {
	case domain.ImportanceCritical:
		return 1.4, true
	case domain.ImportanceVeryImportant:
		return 1.2, true
	case domain.ImportanceImportant:
		return 1, true
	case domain.ImportanceNormal:
		return .8, true
	default:
		return 0, false
	}
}
func WaitingCoefficient(createdAt, now time.Time) float64 {
	weeks := int(now.Sub(createdAt) / (7 * 24 * time.Hour))
	if weeks < 0 {
		weeks = 0
	}
	if weeks > 4 {
		weeks = 4
	}
	return 1 + float64(weeks)/10
}
func (e *Engine) Evaluate(ctx context.Context, j domain.Job, now time.Time) (domain.PolicyDecision, error) {
	f, err := e.facts.Facts(ctx, j)
	if err != nil {
		return domain.PolicyDecision{}, err
	}
	importance, valid := ImportanceCoefficient(j.SystemImportance)
	if !valid {
		importance = .8
	} // legacy persisted jobs have no intent
	quota := 1.0
	if !f.WithinQuota {
		quota = .8
	}
	status, reason := domain.PolicyCompetitive, "Ngoài quy hoạch hoặc vượt hạn mức; xử lý theo thứ tự ưu tiên khi có tài nguyên."
	if f.InSizingPlan && f.WithinQuota {
		status, reason = domain.PolicyAutoEligible, "Trong quy hoạch và hạn mức; đủ điều kiện tự động xếp lịch."
	}
	// All three coefficients have one decimal place. Normalize the product so
	// equivalent combinations retain FIFO ties despite binary floating-point rounding.
	score := math.Round(importance*quota*WaitingCoefficient(j.CreatedAt, now)*1000) / 1000
	return domain.PolicyDecision{Status: status, Reason: reason, InSizingPlan: f.InSizingPlan, WithinQuota: f.WithinQuota,
		QuotaUsage: f.QuotaUsage, Source: f.Source, EvaluatedAt: now.UTC(), AuxiliaryScore: score}, nil
}

// Order refreshes policy facts/aging on each queue read and scheduling cycle.
func (e *Engine) Order(ctx context.Context, jobs []domain.Job, now time.Time) ([]domain.Job, error) {
	result := append([]domain.Job{}, jobs...)
	for i := range result {
		decision, err := e.Evaluate(ctx, result[i], now)
		if err != nil {
			return nil, err
		}
		result[i].Policy = decision
	}
	sort.SliceStable(result, func(i, j int) bool { return Before(result[i], result[j]) })
	return result, nil
}
func necessityRank(n domain.NecessityLevel) int {
	switch n {
	case domain.Necessity1:
		return 1
	case domain.Necessity2:
		return 2
	case domain.Necessity3:
		return 3
	case domain.Necessity4:
		return 4
	default:
		return 5
	}
}

// Before is lexicographic: lane, necessity, auxiliary score, creation time, ID.
func Before(a, b domain.Job) bool {
	if a.Policy.Status != b.Policy.Status {
		return a.Policy.Status == domain.PolicyAutoEligible
	}
	if necessityRank(a.NecessityLevel) != necessityRank(b.NecessityLevel) {
		return necessityRank(a.NecessityLevel) < necessityRank(b.NecessityLevel)
	}
	if a.Policy.AuxiliaryScore != b.Policy.AuxiliaryScore {
		return a.Policy.AuxiliaryScore > b.Policy.AuxiliaryScore
	}
	if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ID < b.ID
}
func Validate(intent domain.AllocationIntent, fields map[string]string) {
	if intent.WorkloadType != domain.WorkloadTraining && intent.WorkloadType != domain.WorkloadInference {
		fields["workloadType"] = "Chọn Training hoặc Inference"
	}
	if necessityRank(intent.NecessityLevel) == 5 {
		fields["necessityLevel"] = "Chọn Cần thiết 1 đến 4"
	}
	if _, ok := ImportanceCoefficient(intent.SystemImportance); !ok {
		fields["systemImportance"] = "Chọn mức độ quan trọng hệ thống"
	}
	validReason := false
	for _, p := range Profiles() {
		if p.WorkloadType != intent.WorkloadType || p.Level != intent.NecessityLevel {
			continue
		}
		for _, reason := range p.Reasons {
			validReason = validReason || reason.ID == intent.NecessityReason
		}
	}
	if intent.NecessityReason == "CUSTOM" {
		validReason = true
		if strings.TrimSpace(intent.NecessityExplanation) == "" {
			fields["necessityExplanation"] = "Nhập giải trình cho lý do khác"
		}
	}
	if !validReason {
		fields["necessityReason"] = "Chọn lý do phù hợp loại workload và tính cần thiết"
	}
	if len(intent.NecessityExplanation) > 4096 {
		fields["necessityExplanation"] = "Giải trình tối đa 4096 byte"
	}
}
