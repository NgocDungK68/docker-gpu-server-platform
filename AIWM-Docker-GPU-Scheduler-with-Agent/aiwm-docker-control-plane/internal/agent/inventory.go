package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

type ProcessContainerResolver interface {
	ContainerID(pid int) (string, error)
}

type InventoryCollector struct {
	docker   DockerRuntime
	gpus     GPUReader
	resolver ProcessContainerResolver
	now      func() time.Time
	policy   InventoryPolicy
}

// InventoryPolicy controls conservative accounting for unattributed GPU telemetry.
type InventoryPolicy struct {
	UnknownMemoryMiB      int64
	UnknownUtilizationPct float64
}

// NewInventoryCollector joins Docker grants and NVML processes into a complete snapshot.
func NewInventoryCollector(docker DockerRuntime, gpus GPUReader, resolver ProcessContainerResolver, policies ...InventoryPolicy) *InventoryCollector {
	policy := InventoryPolicy{UnknownMemoryMiB: 256, UnknownUtilizationPct: 5}
	if len(policies) > 0 {
		policy = policies[0]
	}
	return &InventoryCollector{docker: docker, gpus: gpus, resolver: resolver, now: time.Now, policy: policy}
}

func (c *InventoryCollector) Snapshot(ctx context.Context, sequence uint64) (agentv1.InventoryReport, error) {
	if sequence == 0 {
		return agentv1.InventoryReport{}, fmt.Errorf("inventory sequence must be positive")
	}
	containers, err := c.docker.ListContainers(ctx)
	if err != nil {
		return agentv1.InventoryReport{}, fmt.Errorf("list Docker containers: %w", err)
	}
	gpus, processes, err := c.gpus.Snapshot(ctx)
	if err != nil {
		return agentv1.InventoryReport{}, fmt.Errorf("read NVML inventory: %w", err)
	}
	dockerVersion, err := c.docker.Version(ctx)
	if err != nil {
		return agentv1.InventoryReport{}, fmt.Errorf("read Docker version: %w", err)
	}

	allGPUUUIDs := make([]string, 0, len(gpus))
	for _, gpu := range gpus {
		allGPUUUIDs = append(allGPUUUIDs, gpu.UUID)
	}
	byID := make(map[string]int, len(containers))
	wireContainers := make([]agentv1.Container, 0, len(containers))
	for _, container := range containers {
		gpuUUIDs := uniqueStrings(container.GPUUUIDs)
		unbounded := container.UnboundedGPUAccess
		for _, index := range container.GPUIndexes {
			found := false
			for _, gpu := range gpus {
				if gpu.Index == index {
					gpuUUIDs = appendUnique(gpuUUIDs, gpu.UUID)
					found = true
					break
				}
			}
			if !found {
				unbounded = true
			}
		}
		for _, uuid := range gpuUUIDs {
			found := false
			for _, gpu := range gpus {
				if gpu.UUID == uuid {
					found = true
					break
				}
			}
			if !found {
				unbounded = true
			}
		}
		if activeContainerState(container.State) && unbounded {
			// A pre-existing `--gpus all`/Count=-1 grant cannot be placed
			// safely. Conservatively protect every GPU until NVML mapping can
			// prove a narrower set.
			gpuUUIDs = append([]string(nil), allGPUUUIDs...)
		}
		origin, jobID := classifyContainer(container.Labels)
		startedAt := container.StartedAt
		wireContainers = append(wireContainers, agentv1.Container{
			ID: container.ID, Name: container.Name, Image: container.Image, State: container.State,
			Origin: origin, JobID: jobID, GPUUUIDs: gpuUUIDs,
			Labels: cloneMap(container.Labels), StartedAt: startedAt,
			FinishedAt: container.FinishedAt, ExitCode: container.ExitCode,
		})
		byID[container.ID] = len(wireContainers) - 1
	}

	for index := range processes {
		if processes[index].ContainerID == "" && c.resolver != nil {
			containerID, resolveErr := c.resolver.ContainerID(processes[index].PID)
			if resolveErr == nil {
				processes[index].ContainerID = matchContainerID(containerID, byID)
			}
		}
		if containerIndex, ok := byID[processes[index].ContainerID]; ok {
			wireContainers[containerIndex].GPUUUIDs = appendUnique(wireContainers[containerIndex].GPUUUIDs, processes[index].GPUUUID)
		}
	}
	for index := range wireContainers {
		sort.Strings(wireContainers[index].GPUUUIDs)
	}
	for i := range gpus {
		gpu := &gpus[i]
		attributed := false
		for _, container := range wireContainers {
			if !activeContainerState(container.State) {
				continue
			}
			for _, uuid := range container.GPUUUIDs {
				if uuid == gpu.UUID {
					attributed = true
				}
			}
		}
		if !attributed && (gpu.MemoryUsedMiB >= c.policy.UnknownMemoryMiB || gpu.UtilizationPct >= c.policy.UnknownUtilizationPct) {
			gpu.State = "OCCUPIED_UNKNOWN"
		}
	}
	return agentv1.InventoryReport{
		Host:     discoverHost(),
		Sequence: sequence, ObservedAt: c.now().UTC(), DockerVersion: dockerVersion,
		GPUs: gpus, Containers: wireContainers, Processes: processes,
	}, nil
}

func (c *InventoryCollector) GPUsAvailable(ctx context.Context, wanted []string, sameJobID string) error {
	report, err := c.Snapshot(ctx, 1)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(report.GPUs))
	occupied := make(map[string]bool)
	ignoredContainers := make(map[string]bool)
	for _, gpu := range report.GPUs {
		known[gpu.UUID] = gpu.Healthy
		if gpu.State == "OCCUPIED_UNKNOWN" {
			occupied[gpu.UUID] = true
		}
	}
	for _, container := range report.Containers {
		if !activeContainerState(container.State) {
			continue
		}
		if container.Origin == "MANAGED" && container.JobID == sameJobID {
			ignoredContainers[container.ID] = true
			continue
		}
		for _, uuid := range container.GPUUUIDs {
			occupied[uuid] = true
		}
	}
	for _, process := range report.Processes {
		if !ignoredContainers[process.ContainerID] {
			occupied[process.GPUUUID] = true
		}
	}
	for _, uuid := range uniqueStrings(wanted) {
		if !known[uuid] || occupied[uuid] {
			return fmt.Errorf("%w: GPU %s is missing, unhealthy, or occupied", ErrUnsafeGPU, uuid)
		}
	}
	return nil
}

func activeContainerState(state string) bool {
	switch state {
	case "running", "paused", "restarting":
		return true
	default:
		return false
	}
}

func classifyContainer(labels map[string]string) (origin, jobID string) {
	if strings.EqualFold(labels[agentv1.ManagedLabel], "true") && strings.TrimSpace(labels[agentv1.JobIDLabel]) != "" {
		return "MANAGED", labels[agentv1.JobIDLabel]
	}
	return "LEGACY", ""
}

func matchContainerID(id string, containers map[string]int) string {
	if _, ok := containers[id]; ok {
		return id
	}
	for candidate := range containers {
		if strings.HasPrefix(candidate, id) || strings.HasPrefix(id, candidate) {
			return candidate
		}
	}
	return ""
}

func appendUnique(values []string, value string) []string {
	for _, candidate := range values {
		if candidate == value {
			return values
		}
	}
	if value != "" {
		return append(values, value)
	}
	return values
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = appendUnique(result, strings.TrimSpace(value))
	}
	return result
}

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
