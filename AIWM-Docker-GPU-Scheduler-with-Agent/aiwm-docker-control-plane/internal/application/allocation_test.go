package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/policy"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/store/memory"
)

func validAllocation() domain.CreateJobRequest {
	fp8 := false
	return domain.CreateJobRequest{AllocationIntent: domain.AllocationIntent{WorkloadType: domain.WorkloadTraining, NecessityLevel: domain.Necessity2, NecessityReason: "GO_LIVE_90_DAYS", SystemImportance: domain.ImportanceImportant, NeededAt: "2026-09-09T09:00:00+07:00", TTLSeconds: 3600},
		Name: "allocation-test", Image: "alpine:3.21", Resources: domain.AllocationResources{GPUCount: 1, MinVRAMMiB: 1024, PerformanceProfile: "a100-equivalent", FP8Required: &fp8}}
}
func allocationFixture(t *testing.T) (*ControlPlane, *memory.Store) {
	t.Helper()
	now := time.Now().UTC()
	repo := memory.New(time.Minute)
	_, err := repo.UpsertServer(context.Background(), domain.Server{ID: "s", MachineID: "m", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now,
		GPUs: []domain.GPU{{UUID: "GPU-a", Model: "A100", MemoryTotalMiB: 40960, Healthy: true, State: domain.GPUFree}}})
	if err != nil {
		t.Fatal(err)
	}
	cp := New(repo, Options{OfflineAfter: time.Minute, DefaultStrategy: domain.StrategyBestFit})
	cp.now = func() time.Time { return now }
	return cp, repo
}
func TestAllocationValidation(t *testing.T) {
	cases := []struct {
		field  string
		change func(*domain.CreateJobRequest)
	}{
		{"resources.gpuCount", func(r *domain.CreateJobRequest) { r.Resources.GPUCount = 0 }},
		{"resources.gpuCount", func(r *domain.CreateJobRequest) { r.Resources.GPUCount = -1 }},
		{"resources.gpuCount", func(r *domain.CreateJobRequest) { r.Resources.GPUCount = 65 }},
		{"resources.minVramMiB", func(r *domain.CreateJobRequest) { r.Resources.MinVRAMMiB = 0 }},
		{"resources.minVramMiB", func(r *domain.CreateJobRequest) { r.Resources.MinVRAMMiB = -1 }},
		{"resources.performanceProfile", func(r *domain.CreateJobRequest) { r.Resources.PerformanceProfile = "arbitrary" }},
		{"resources.fp8Required", func(r *domain.CreateJobRequest) { r.Resources.FP8Required = nil }},
		{"workloadType", func(r *domain.CreateJobRequest) { r.WorkloadType = "UNKNOWN" }},
		{"necessityLevel", func(r *domain.CreateJobRequest) { r.NecessityLevel = "NECESSITY_5" }},
		{"systemImportance", func(r *domain.CreateJobRequest) { r.SystemImportance = "OTHER" }},
		{"ttlSeconds", func(r *domain.CreateJobRequest) { r.TTLSeconds = 0 }},
		{"ttlSeconds", func(r *domain.CreateJobRequest) { r.TTLSeconds = -1 }},
		{"ttlSeconds", func(r *domain.CreateJobRequest) { r.TTLSeconds = 2592001 }},
		{"neededAt", func(r *domain.CreateJobRequest) { r.NeededAt = "2026-02-30T10:00:00Z" }},
		{"neededAt", func(r *domain.CreateJobRequest) { r.NeededAt = "2026-09-09T10:00:00" }},
		{"necessityExplanation", func(r *domain.CreateJobRequest) { r.NecessityReason = "CUSTOM" }},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			cp, repo := allocationFixture(t)
			r := validAllocation()
			tc.change(&r)
			_, err := cp.CreateJob(context.Background(), r)
			var validation *domain.ValidationError
			if !errors.As(err, &validation) || validation.Fields[tc.field] == "" {
				t.Fatalf("%v", err)
			}
			if len(repo.Export().Jobs) != 0 {
				t.Fatal("invalid input persisted")
			}
		})
	}
	cp, _ := allocationFixture(t)
	r := validAllocation()
	r.WorkloadType = domain.WorkloadInference
	r.NecessityReason = "CUSTOM"
	r.NecessityExplanation = "Hợp lệ"
	if _, err := cp.PreviewJob(context.Background(), r); err != nil {
		t.Fatal(err)
	}
}
func TestResourceMatchingAndPreviewHasNoMutation(t *testing.T) {
	for _, kind := range []string{"valid", "count", "vram", "profile", "fp8", "unhealthy", "offline", "stale", "external", "unknown", "reserved", "allocated"} {
		t.Run(kind, func(t *testing.T) {
			cp, repo := allocationFixture(t)
			r := validAllocation()
			snap := repo.Export()
			s := snap.Servers["s"]
			switch kind {
			case "count":
				r.Resources.GPUCount = 2
			case "vram":
				r.Resources.MinVRAMMiB = 81920
			case "profile":
				r.Resources.PerformanceProfile = "h100-equivalent"
			case "fp8":
				*r.Resources.FP8Required = true
			case "unhealthy":
				s.GPUs[0].Healthy = false
			case "offline":
				s.Status = domain.ServerOffline
			case "stale":
				s.InventoryReceivedAt = cp.now().Add(-time.Hour)
			case "external":
				s.GPUs[0].State = domain.GPUOccupiedLegacy
			case "unknown":
				s.GPUs[0].State = domain.GPUOccupiedUnknown
			case "reserved":
				s.GPUs[0].State = domain.GPUReserved
			case "allocated":
				s.GPUs[0].State = domain.GPUAllocated
			}
			snap.Servers["s"] = s
			repo.Restore(snap)
			before := repo.Export()
			preview, err := cp.PreviewJob(context.Background(), r)
			if err != nil || preview.ResourceMatch.Satisfiable != (kind == "valid") {
				t.Fatalf("%+v %v", preview, err)
			}
			if !reflect.DeepEqual(before, repo.Export()) {
				t.Fatal("preview mutated jobs/commands/GPUs")
			}
		})
	}
}

type mutableFacts struct {
	plan  bool
	calls int
}

func (f *mutableFacts) Facts(_ context.Context, _ domain.Job) (policy.Facts, error) {
	f.calls++
	return policy.Facts{InSizingPlan: f.plan, WithinQuota: true, Source: "test"}, nil
}
func TestSubmitReevaluatesPreviewAndSchedulingRechecks(t *testing.T) {
	cp, repo := allocationFixture(t)
	facts := &mutableFacts{plan: true}
	cp.policy = policy.New(facts)
	ctx := context.Background()
	preview, err := cp.PreviewJob(ctx, validAllocation())
	if err != nil || !preview.ResourceMatch.Satisfiable {
		t.Fatal(err)
	}
	facts.plan = false
	if _, err := repo.SetServerDrained(ctx, "s", true); err != nil {
		t.Fatal(err)
	}
	j, err := cp.CreateJob(ctx, validAllocation())
	if err != nil || j.Policy.Status != domain.PolicyCompetitive || facts.calls != 2 {
		t.Fatalf("%+v %v calls=%d", j, err, facts.calls)
	}
	n, err := cp.ScheduleOnce(ctx)
	if err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if len(repo.Export().Commands) != 0 {
		t.Fatal("stale preview reserved")
	}
	_, _ = repo.SetServerDrained(ctx, "s", false)
	facts.plan = true
	n, err = cp.ScheduleOnce(ctx)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	stored, _ := repo.GetJob(ctx, j.ID)
	if stored.Assignment == nil || stored.Policy.Status != domain.PolicyAutoEligible || len(repo.Export().Commands) != 1 {
		t.Fatal(stored)
	}
}
func TestPolicyOrderDeterminesWinnerAndCommitRechecksCapability(t *testing.T) {
	cp, repo := allocationFixture(t)
	ctx := context.Background()
	low := validAllocation()
	low.NecessityLevel = domain.Necessity4
	low.NecessityReason = "MODEL_EXPERIMENT"
	low.SystemImportance = domain.ImportanceCritical
	lowJob, _ := cp.CreateJob(ctx, low)
	high := validAllocation()
	high.NecessityLevel = domain.Necessity1
	high.NecessityReason = "EXECUTIVE_DIRECTION"
	high.SystemImportance = domain.ImportanceNormal
	highJob, _ := cp.CreateJob(ctx, high)
	n, err := cp.ScheduleOnce(ctx)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	a, _ := repo.GetJob(ctx, highJob.ID)
	b, _ := repo.GetJob(ctx, lowJob.ID)
	if a.Assignment == nil || b.Assignment != nil {
		t.Fatal("wrong business winner")
	}
	cp, repo = allocationFixture(t)
	j, _ := cp.CreateJob(ctx, validAllocation())
	servers, _ := repo.ListServers(ctx)
	placement, _ := cp.scheduler.Plan(j, servers, cp.now())
	snap := repo.Export()
	s := snap.Servers["s"]
	s.GPUs[0].Model = "T4"
	snap.Servers["s"] = s
	repo.Restore(snap)
	before := repo.Export()
	_, err = repo.CommitAssignment(ctx, j.ID, placement, domain.Command{ID: "cmd", AgentID: "s"}, cp.now())
	if !errors.Is(err, domain.ErrConflict) || !reflect.DeepEqual(before, repo.Export()) {
		t.Fatal("commit ignored changed capability")
	}
}
