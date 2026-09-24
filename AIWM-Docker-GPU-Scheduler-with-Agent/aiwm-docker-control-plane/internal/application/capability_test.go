package application

import (
	"reflect"
	"testing"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

func TestSimplifiedCapabilityPreviewAndFuturePlanning(t *testing.T) {
	for _, tc := range []struct {
		name, profile, model string
		fp8, want            bool
		state                domain.GPUState
		org                  string
		count                int
	}{
		{"auto unknown model", "AUTO", "new-model", false, true, domain.GPUFree, "test-org", 1},
		{"auto fp8 unknown rejected", "AUTO", "new-model", true, false, domain.GPUFree, "test-org", 1},
		{"high H200", "HIGH_PERFORMANCE", "H200", true, true, domain.GPUFree, "test-org", 1},
		{"high excludes A100", "HIGH_PERFORMANCE", "A100", false, false, domain.GPUFree, "test-org", 1},
		{"external excluded", "AUTO", "H100", false, false, domain.GPUOccupiedLegacy, "test-org", 1},
		{"org excluded", "AUTO", "H100", false, false, domain.GPUFree, "other", 1},
		{"count required", "AUTO", "H100", false, false, domain.GPUFree, "test-org", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cp, repo := allocationFixture(t)
			ctx := allocationContext()
			snapshot := repo.Export()
			server := snapshot.Servers["s"]
			server.OrganizationID = tc.org
			server.GPUs[0].Model, server.GPUs[0].State = tc.model, tc.state
			snapshot.Servers["s"] = server
			repo.Restore(snapshot)
			r := validAllocation()
			r.NeededAt = cp.now().Add(time.Hour).Format(time.RFC3339)
			r.Resources.PerformanceProfile, r.Resources.FP8Required, r.Resources.GPUCount = tc.profile, &tc.fp8, tc.count
			// No CPU/RAM input is required for admission or GPU matching.
			before := repo.Export()
			preview, err := cp.PreviewJob(ctx, r)
			if err != nil || preview.ResourceMatch.Satisfiable != tc.want {
				t.Fatalf("preview=%+v err=%v", preview, err)
			}
			if !reflect.DeepEqual(before, repo.Export()) {
				t.Fatal("preview mutated resources")
			}
			job, err := cp.CreateJob(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := cp.ScheduleOnce(ctx); err != nil {
				t.Fatal(err)
			}
			stored, _ := repo.GetJob(ctx, job.ID)
			if (stored.Assignment != nil) != tc.want {
				t.Fatalf("assignment=%+v", stored)
			}
			if len(repo.Export().Commands) != 0 {
				t.Fatal("future planning dispatched early")
			}
		})
	}
}
