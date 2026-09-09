package application

import (
	"errors"
	"testing"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

func TestBestFitExcludesLegacyGPU(t *testing.T) {
	now := time.Now().UTC()
	scheduler := NewScheduler(domain.StrategyBestFit, time.Minute)
	servers := []domain.Server{
		{
			ID: "large", Name: "large", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now,
			GPUs: []domain.GPU{
				gpu("large-0", domain.GPUOccupiedLegacy), gpu("large-1", domain.GPUFree),
				gpu("large-2", domain.GPUFree), gpu("large-3", domain.GPUFree),
			},
		},
		{
			ID: "exact", Name: "exact", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now,
			GPUs: []domain.GPU{gpu("exact-0", domain.GPUFree), gpu("exact-1", domain.GPUFree)},
		},
	}
	job := domain.Job{Resources: domain.ResourceRequest{GPUCount: 2, GPUModel: "NVIDIA-A100-80GB", MinVRAMMiB: 40000}}

	placement, err := scheduler.Plan(job, servers, now)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if placement.ServerID != "exact" {
		t.Fatalf("Plan() server = %q, want exact", placement.ServerID)
	}
	for _, uuid := range placement.GPUUUIDs {
		if uuid == "large-0" {
			t.Fatal("legacy GPU was selected")
		}
	}
}

func TestPlanRejectsOfflineServer(t *testing.T) {
	now := time.Now().UTC()
	scheduler := NewScheduler(domain.StrategyFirstFit, 10*time.Second)
	server := domain.Server{
		ID: "stale", Status: domain.ServerOnline, LastHeartbeatAt: now.Add(-time.Minute),
		GPUs: []domain.GPU{gpu("gpu-0", domain.GPUFree)},
	}
	_, err := scheduler.Plan(domain.Job{Resources: domain.ResourceRequest{GPUCount: 1}}, []domain.Server{server}, now)
	if !errors.Is(err, domain.ErrInsufficientGPU) {
		t.Fatalf("Plan() error = %v, want ErrInsufficientGPU", err)
	}
}

func gpu(uuid string, state domain.GPUState) domain.GPU {
	return domain.GPU{
		UUID: uuid, Model: "NVIDIA-A100-80GB", MemoryTotalMiB: 81920,
		Healthy: true, State: state,
	}
}
