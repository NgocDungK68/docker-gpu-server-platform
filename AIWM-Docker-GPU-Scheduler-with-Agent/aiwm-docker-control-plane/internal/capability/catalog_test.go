package capability

import (
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"testing"
)

func TestCatalogFP8ResolutionAndUnknownModels(t *testing.T) {
	catalog := Default()
	for _, tc := range []struct {
		profile string
		fp8     bool
		model   string
		want    bool
	}{
		{"h100-equivalent", true, "NVIDIA H100 80GB HBM3", true},
		{"h100-equivalent", true, "A100", false},
		{"a100-equivalent", true, "A100", false},
		{"general", false, "Tesla T4", true},
		{"general", false, "unknown-new-model", false},
		{"AUTO", false, "unknown-new-model", true},
		{"AUTO", true, "unknown-new-model", false},
		{"AUTO", false, "A100", true},
		{"AUTO", true, "A100", false},
		{"AUTO", true, "L40S", true},
		{"HIGH_PERFORMANCE", false, "A100", false},
		{"HIGH_PERFORMANCE", false, "H100", true},
		{"HIGH_PERFORMANCE", true, "H200", true},
		{"HIGH_PERFORMANCE", true, "L40S", false},
	} {
		models, err := catalog.Resolve(tc.profile, tc.fp8)
		if err != nil {
			t.Fatal(err)
		}
		r := domain.ResourceRequest{PerformanceProfile: tc.profile, FP8Required: tc.fp8, ResolvedModels: models}
		if got := r.CapabilityMatches(domain.GPU{Model: tc.model}); got != tc.want {
			t.Fatalf("%+v match=%v", tc, got)
		}
	}
	if _, err := catalog.Resolve("unknown", false); err == nil {
		t.Fatal("unknown profile accepted")
	}
	profiles := catalog.Profiles()
	if len(profiles) != 2 || profiles[0].ID != "AUTO" || profiles[1].ID != "HIGH_PERFORMANCE" {
		t.Fatalf("public profiles: %+v", profiles)
	}
	// Caller mutation cannot change the catalog.
	models, _ := catalog.Resolve("general", false)
	models[0] = "injected"
	fresh, _ := catalog.Resolve("general", false)
	if fresh[0] == "injected" {
		t.Fatal("aliased resolver output")
	}
}
