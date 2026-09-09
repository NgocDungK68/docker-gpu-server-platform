package memory

import (
	"fmt"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

// normalizeGPUState derives availability from actual consumers and active reservations.
func normalizeGPUState(serverID string, gpus []domain.GPU, containers []domain.Container, processes []domain.GPUProcess, jobs map[string]domain.Job) []domain.GPU {
	result := cloneGPUs(gpus)
	legacy, unknown := make(map[string][]string), make(map[string][]string)
	managed, reserved := make(map[string]string), make(map[string]string)
	knownContainers := make(map[string][]string)
	for _, job := range jobs {
		if job.Assignment == nil || job.Assignment.ServerID != serverID || job.Status.Terminal() {
			continue
		}
		for _, uuid := range job.Assignment.GPUUUIDs {
			reserved[uuid] = job.ID
		}
	}
	for _, c := range containers {
		if !containerConsumesGPU(c.State) {
			continue
		}
		knownContainers[c.ID] = c.GPUUUIDs
		for _, uuid := range c.GPUUUIDs {
			job, known := jobs[c.JobID]
			if c.Origin == domain.ContainerManaged && known && job.Assignment != nil && job.Assignment.ServerID == serverID && containsUUID(job.Assignment.GPUUUIDs, uuid) {
				if other := managed[uuid]; other != "" && other != job.ID {
					unknown[uuid] = append(unknown[uuid], "multiple managed consumers")
				}
				managed[uuid] = job.ID
			} else {
				legacy[uuid] = append(legacy[uuid], c.ID)
			}
		}
	}
	for _, p := range processes {
		if !containsUUID(knownContainers[p.ContainerID], p.GPUUUID) {
			unknown[p.GPUUUID] = append(unknown[p.GPUUUID], fmt.Sprintf("pid:%d", p.PID))
		}
	}
	for i := range result {
		g := &result[i]
		observedUnknown := g.State == domain.GPUOccupiedUnknown
		g.AssignedJobID, g.StateReason = "", ""
		g.ObservedConsumers = []string{}
		switch {
		case !g.Healthy:
			g.State, g.StateReason = domain.GPUUnhealthy, "NVML device unhealthy or required telemetry unavailable"
		case len(legacy[g.UUID]) > 0:
			g.State, g.StateReason = domain.GPUOccupiedLegacy, "external container has GPU access"
			g.ObservedConsumers = legacy[g.UUID]
		case len(unknown[g.UUID]) > 0:
			g.State, g.StateReason = domain.GPUOccupiedUnknown, "unattributed GPU process"
			g.ObservedConsumers = unknown[g.UUID]
		case managed[g.UUID] != "":
			g.State, g.AssignedJobID, g.StateReason = domain.GPUAllocated, managed[g.UUID], "managed container observed active"
		case observedUnknown:
			g.State, g.StateReason = domain.GPUOccupiedUnknown, "GPU usage has no confirmed owner"
		case reserved[g.UUID] != "":
			g.State, g.AssignedJobID, g.StateReason = domain.GPUReserved, reserved[g.UUID], "atomic reservation awaiting actual state"
		default:
			g.State, g.StateReason = domain.GPUFree, "healthy GPU without consumers or reservation"
		}
	}
	return result
}

func containsUUID(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func containerConsumesGPU(state string) bool {
	return state == "running" || state == "paused" || state == "restarting"
}
