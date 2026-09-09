package policy

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

func TestCoefficientsAndWaitingBoundaries(t *testing.T) {
	for value, want := range map[domain.SystemImportance]float64{domain.ImportanceCritical: 1.4, domain.ImportanceVeryImportant: 1.2, domain.ImportanceImportant: 1, domain.ImportanceNormal: .8} {
		got, ok := ImportanceCoefficient(value)
		if !ok || got != want {
			t.Fatalf("%s=%v", value, got)
		}
	}
	now := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		age  time.Duration
		want float64
	}{{-time.Hour, 1}, {7*24*time.Hour - time.Nanosecond, 1}, {7 * 24 * time.Hour, 1.1}, {14 * 24 * time.Hour, 1.2}, {21 * 24 * time.Hour, 1.3}, {28 * 24 * time.Hour, 1.4}, {365 * 24 * time.Hour, 1.4}} {
		if got := WaitingCoefficient(now.Add(-tc.age), now); got != tc.want {
			t.Errorf("age=%v got=%v", tc.age, got)
		}
	}
}
func TestEligibilityAndQuota(t *testing.T) {
	now := time.Now()
	j := domain.Job{AllocationIntent: domain.AllocationIntent{SystemImportance: domain.ImportanceImportant}, Resources: domain.ResourceRequest{GPUCount: 2}, CreatedAt: now}
	for _, tc := range []struct {
		plan   bool
		used   int
		status domain.PolicyStatus
		score  float64
	}{{true, 0, domain.PolicyAutoEligible, 1}, {false, 0, domain.PolicyCompetitive, 1}, {true, 3, domain.PolicyCompetitive, .8}, {false, 3, domain.PolicyCompetitive, .8}} {
		got, err := New(DevelopmentFacts{InSizingPlan: tc.plan, QuotaGPUs: 4, UsedGPUs: tc.used}).Evaluate(context.Background(), j, now)
		if err != nil || got.Status != tc.status || math.Abs(got.AuxiliaryScore-tc.score) > 1e-9 {
			t.Fatalf("%+v %v", got, err)
		}
	}
}
func TestLexicographicOrdering(t *testing.T) {
	now := time.Now()
	levels := []domain.NecessityLevel{domain.Necessity1, domain.Necessity2, domain.Necessity3, domain.Necessity4}
	for i := 0; i < 3; i++ {
		a := domain.Job{AllocationIntent: domain.AllocationIntent{NecessityLevel: levels[i]}, Policy: domain.PolicyDecision{Status: domain.PolicyCompetitive, AuxiliaryScore: .64}}
		b := domain.Job{AllocationIntent: domain.AllocationIntent{NecessityLevel: levels[i+1]}, Policy: domain.PolicyDecision{Status: domain.PolicyCompetitive, AuxiliaryScore: 1.96}}
		if !Before(a, b) || Before(b, a) {
			t.Fatal("auxiliary score reversed necessity")
		}
	}
	a := domain.Job{ID: "a", CreatedAt: now, AllocationIntent: domain.AllocationIntent{NecessityLevel: domain.Necessity1}, Policy: domain.PolicyDecision{Status: domain.PolicyCompetitive, AuxiliaryScore: 1}}
	b := a
	b.ID = "b"
	if !Before(a, b) {
		t.Fatal("ID tie")
	}
	b.CreatedAt = now.Add(-time.Second)
	if !Before(b, a) {
		t.Fatal("FIFO tie")
	}
	a.Policy.AuxiliaryScore = 1.2
	if !Before(a, b) {
		t.Fatal("auxiliary tie")
	}
	b.Policy.Status = domain.PolicyAutoEligible
	b.NecessityLevel = domain.Necessity4
	if !Before(b, a) {
		t.Fatal("auto lane")
	}
}
func TestWorkloadReasonProfiles(t *testing.T) {
	counts := map[domain.WorkloadType][]int{domain.WorkloadTraining: {4, 3, 2, 2}, domain.WorkloadInference: {4, 1, 3, 4}}
	for _, p := range Profiles() {
		if len(p.Reasons) != counts[p.WorkloadType][necessityRank(p.Level)-1] {
			t.Fatal(p)
		}
		for _, r := range p.Reasons {
			fields := map[string]string{}
			Validate(domain.AllocationIntent{WorkloadType: p.WorkloadType, NecessityLevel: p.Level, NecessityReason: r.ID, SystemImportance: domain.ImportanceNormal}, fields)
			if len(fields) != 0 {
				t.Fatal(fields)
			}
		}
	}
	for _, intent := range []domain.AllocationIntent{
		{WorkloadType: "UNKNOWN", NecessityLevel: domain.Necessity1, NecessityReason: "CUSTOM", NecessityExplanation: "ok"},
		{WorkloadType: domain.WorkloadTraining, NecessityLevel: domain.Necessity2, NecessityReason: "CUSTOM"},
		{WorkloadType: domain.WorkloadInference, NecessityLevel: domain.Necessity2, NecessityReason: "GO_LIVE_90_DAYS"},
		{WorkloadType: domain.WorkloadTraining, NecessityLevel: domain.Necessity1, NecessityReason: "PERIODIC_RETRAINING"},
	} {
		fields := map[string]string{}
		intent.SystemImportance = domain.ImportanceNormal
		Validate(intent, fields)
		if len(fields) == 0 {
			t.Fatal("invalid intent accepted")
		}
	}
}

func TestEquivalentAuxiliaryProductsRetainFIFO(t *testing.T) {
	now := time.Now()
	a := domain.Job{ID: "a", AllocationIntent: domain.AllocationIntent{NecessityLevel: domain.Necessity2, SystemImportance: domain.ImportanceVeryImportant}, CreatedAt: now.Add(-28 * 24 * time.Hour), Resources: domain.ResourceRequest{GPUCount: 1}}
	b := a
	b.ID = "b"
	b.SystemImportance = domain.ImportanceCritical
	b.CreatedAt = now.Add(-14 * 24 * time.Hour)
	ordered, err := New(DevelopmentFacts{QuotaGPUs: 0}).Order(context.Background(), []domain.Job{b, a}, now)
	if err != nil || ordered[0].ID != "a" || ordered[0].Policy.AuxiliaryScore != ordered[1].Policy.AuxiliaryScore {
		t.Fatalf("%+v %v", ordered, err)
	}
}
