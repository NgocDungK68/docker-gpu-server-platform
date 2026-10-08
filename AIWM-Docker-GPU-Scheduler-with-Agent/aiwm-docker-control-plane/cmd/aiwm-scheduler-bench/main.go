package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/application"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

type result struct {
	Strategy              domain.SchedulingStrategy `json:"strategy"`
	ScheduledJobs         int                       `json:"scheduledJobs"`
	QueuedJobs            int                       `json:"queuedJobs"`
	AllocatableGPUs       int                       `json:"allocatableGpus"`
	AllocatedGPUs         int                       `json:"allocatedGpus"`
	UtilizationPct        float64                   `json:"utilizationPct"`
	CompletelyFreeServers int                       `json:"completelyFreeServers"`
	FragmentedServers     int                       `json:"fragmentedServers"`
}

func main() {
	strategies := []domain.SchedulingStrategy{domain.StrategyFirstFit, domain.StrategyBestFit, domain.StrategyBinPack, domain.StrategyFragmentation}
	results := make([]result, 0, len(strategies))
	for _, strategy := range strategies {
		results = append(results, simulate(strategy))
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func simulate(strategy domain.SchedulingStrategy) result {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	servers := fixtureServers(now)
	// Small jobs arrive before two large single-server jobs. First Fit can strand capacity,
	// while Best Fit/Bin Pack preserve the 4-GPU hosts for the large jobs.
	requests := []int{1, 3, 4, 4, 2}
	scheduler := application.NewScheduler(strategy, time.Minute)
	metrics := result{Strategy: strategy}
	for _, server := range servers {
		for _, gpu := range server.GPUs {
			if gpu.State == domain.GPUFree {
				metrics.AllocatableGPUs++
			}
		}
	}
	for index, count := range requests {
		job := domain.Job{OrganizationID: "benchmark-org", ID: fmt.Sprintf("job-%02d", index), Strategy: strategy, Resources: domain.ResourceRequest{GPUCount: count, GPUModel: "A100"}}
		placement, err := scheduler.Plan(job, servers, now)
		if err != nil {
			metrics.QueuedJobs++
			continue
		}
		metrics.ScheduledJobs++
		metrics.AllocatedGPUs += len(placement.GPUUUIDs)
		allocate(servers, placement)
	}
	if metrics.AllocatableGPUs > 0 {
		metrics.UtilizationPct = 100 * float64(metrics.AllocatedGPUs) / float64(metrics.AllocatableGPUs)
	}
	for _, server := range servers {
		free := 0
		for _, gpu := range server.GPUs {
			if gpu.State == domain.GPUFree {
				free++
			}
		}
		if free == len(server.GPUs) {
			metrics.CompletelyFreeServers++
		}
		if free > 0 && free < len(server.GPUs) {
			metrics.FragmentedServers++
		}
	}
	return metrics
}

func fixtureServers(now time.Time) []domain.Server {
	freeCounts := []int{3, 4, 2, 1, 4}
	servers := make([]domain.Server, 0, len(freeCounts))
	for serverIndex, freeCount := range freeCounts {
		server := domain.Server{OrganizationID: "benchmark-org", ID: fmt.Sprintf("server-%d", serverIndex), Name: fmt.Sprintf("server-%d", serverIndex), Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now}
		for gpuIndex := 0; gpuIndex < 4; gpuIndex++ {
			state := domain.GPUOccupiedLegacy
			if gpuIndex < freeCount {
				state = domain.GPUFree
			}
			server.GPUs = append(server.GPUs, domain.GPU{UUID: fmt.Sprintf("GPU-%d-%d", serverIndex, gpuIndex), Model: "A100", MemoryTotalMiB: 81920, Healthy: true, State: state})
		}
		servers = append(servers, server)
	}
	return servers
}

func allocate(servers []domain.Server, placement domain.Placement) {
	wanted := make(map[string]bool, len(placement.GPUUUIDs))
	for _, uuid := range placement.GPUUUIDs {
		wanted[uuid] = true
	}
	for serverIndex := range servers {
		if servers[serverIndex].ID != placement.ServerID {
			continue
		}
		for gpuIndex := range servers[serverIndex].GPUs {
			if wanted[servers[serverIndex].GPUs[gpuIndex].UUID] {
				servers[serverIndex].GPUs[gpuIndex].State = domain.GPUAllocated
			}
		}
	}
}
