package application

import (
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"strings"
	"testing"
	"time"
)

func TestAllPoliciesAndDeterministicTies(t *testing.T) {
	now := time.Now()
	makeServer := func(id string, free int) domain.Server {
		s := domain.Server{ID: id, Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now}
		for i := 0; i < 4; i++ {
			state := domain.GPUOccupiedLegacy
			if i < free {
				state = domain.GPUFree
			}
			s.GPUs = append(s.GPUs, gpu(id+string(rune('0'+i)), state))
		}
		return s
	}
	servers := []domain.Server{makeServer("a-empty", 4), makeServer("b-partial", 1)}
	for _, strategy := range []domain.SchedulingStrategy{domain.StrategyFirstFit, domain.StrategyBestFit, domain.StrategyBinPack, domain.StrategyFragmentation} {
		t.Run(string(strategy), func(t *testing.T) {
			scheduler := NewScheduler(strategy, time.Minute)
			want := "b-partial"
			if strategy == domain.StrategyFirstFit {
				want = "a-empty"
			}
			for _, input := range [][]domain.Server{servers, {servers[1], servers[0]}} {
				placement, err := scheduler.Plan(domain.Job{Resources: domain.ResourceRequest{GPUCount: 1}}, input, now)
				if err != nil || placement.ServerID != want {
					t.Fatalf("got %+v %v, want %s", placement, err, want)
				}
			}
		})
	}
}
func TestSchedulerFiltersAndPendingReasons(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name, want string
		mutate     func(*domain.Server, *domain.Job)
	}{
		{"offline", "no online", func(s *domain.Server, _ *domain.Job) { s.Status = domain.ServerOffline }},
		{"stale inventory", "no online", func(s *domain.Server, _ *domain.Job) { s.InventoryReceivedAt = now.Add(-time.Hour) }},
		{"unhealthy", "healthy", func(s *domain.Server, _ *domain.Job) { s.GPUs[0].Healthy = false }},
		{"model", "model unavailable", func(_ *domain.Server, j *domain.Job) { j.Resources.GPUModel = "H100" }},
		{"vram", "VRAM", func(_ *domain.Server, j *domain.Job) { j.Resources.MinVRAMMiB = 999999 }},
		{"count", "GPU count", func(_ *domain.Server, j *domain.Job) { j.Resources.GPUCount = 2 }},
		{"external", "external workload", func(s *domain.Server, _ *domain.Job) { s.GPUs[0].State = domain.GPUOccupiedLegacy }},
		{"unknown", "ownership unknown", func(s *domain.Server, _ *domain.Job) { s.GPUs[0].State = domain.GPUOccupiedUnknown }},
		{"reserved", "reserved or allocated", func(s *domain.Server, _ *domain.Job) { s.GPUs[0].State = domain.GPUReserved }},
		{"selector", "labels", func(_ *domain.Server, j *domain.Job) { j.ServerSelector = map[string]string{"site": "other"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := domain.Server{ID: "s", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now, GPUs: []domain.GPU{gpu("GPU-0", domain.GPUFree)}}
			j := domain.Job{Resources: domain.ResourceRequest{GPUCount: 1}}
			tc.mutate(&s, &j)
			_, err := NewScheduler(domain.StrategyBestFit, time.Minute).Plan(j, []domain.Server{s}, now)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v want %s", err, tc.want)
			}
		})
	}
}

// A system scorer can be replaced without changing API, business priority or safety filters.
func TestInjectedPlacementPolicyKeepsHardConstraints(t *testing.T) {
	now := time.Now()
	servers := []domain.Server{
		{ID: "a", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now, GPUs: []domain.GPU{gpu("GPU-a", domain.GPUFree)}},
		{ID: "b", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now, GPUs: []domain.GPU{gpu("GPU-b", domain.GPUFree)}},
		{ID: "c", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now, GPUs: []domain.GPU{gpu("GPU-c", domain.GPUOccupiedLegacy)}},
	}
	scorer := scoreFunc(func(server domain.Server, _, _ []domain.GPU) float64 {
		if server.ID == "c" {
			return -100
		}
		if server.ID == "b" {
			return -1
		}
		return 0
	})
	scheduler := NewScheduler(domain.StrategyBestFit, time.Minute).WithPolicy(domain.StrategyBestFit, scorer)
	job := domain.Job{Strategy: domain.StrategyFirstFit, Resources: domain.ResourceRequest{GPUCount: 1}}
	placement, err := scheduler.Plan(job, servers, now)
	if err != nil || placement.ServerID != "b" || placement.Strategy != domain.StrategyBestFit {
		t.Fatalf("%+v %v", placement, err)
	}
}
