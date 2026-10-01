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
			OrganizationID: "test-org", ID: "large", Name: "large", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now,
			GPUs: []domain.GPU{
				gpu("large-0", domain.GPUOccupiedLegacy), gpu("large-1", domain.GPUFree),
				gpu("large-2", domain.GPUFree), gpu("large-3", domain.GPUFree),
			},
		},
		{
			OrganizationID: "test-org", ID: "exact", Name: "exact", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now,
			GPUs: []domain.GPU{gpu("exact-0", domain.GPUFree), gpu("exact-1", domain.GPUFree)},
		},
	}
	job := domain.Job{OrganizationID: "test-org", Resources: domain.ResourceRequest{GPUCount: 2, GPUModel: "NVIDIA-A100-80GB", MinVRAMMiB: 40000}}

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
		OrganizationID: "test-org", ID: "stale", Status: domain.ServerOnline, LastHeartbeatAt: now.Add(-time.Minute),
		GPUs: []domain.GPU{gpu("gpu-0", domain.GPUFree)},
	}
	_, err := scheduler.Plan(domain.Job{OrganizationID: "test-org", Resources: domain.ResourceRequest{GPUCount: 1}}, []domain.Server{server}, now)
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

func TestLegacyUserLabelsDoNotConstrainPlacement(t *testing.T) {
	now := time.Now().UTC()
	job := domain.Job{OrganizationID: "own", ServerSelector: map[string]string{"site": "old-selector"}, Resources: domain.ResourceRequest{GPUCount: 1}}
	servers := []domain.Server{
		{ID: "foreign", OrganizationID: "other", Labels: job.ServerSelector, Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now, GPUs: []domain.GPU{gpu("foreign", domain.GPUFree)}},
		{ID: "own", OrganizationID: "own", Labels: map[string]string{"site": "different"}, Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now, GPUs: []domain.GPU{gpu("own", domain.GPUFree)}},
	}
	for _, strategy := range []domain.SchedulingStrategy{domain.StrategyFirstFit, domain.StrategyBestFit, domain.StrategyBinPack, domain.StrategyFragmentation} {
		placement, err := NewScheduler(strategy, time.Minute).Plan(job, servers, now)
		if err != nil || placement.ServerID != "own" {
			t.Fatalf("%s: placement=%+v err=%v", strategy, placement, err)
		}
	}
}
