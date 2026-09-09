package application

import (
	"fmt"
	"strings"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

func inventoryToDomain(report agentv1.InventoryReport) (domain.InventoryReport, error) {
	result := domain.InventoryReport{Sequence: report.Sequence, ObservedAt: report.ObservedAt, DockerVersion: report.DockerVersion}
	result.Host = domain.HostInfo{Hostname: report.Host.Hostname, OS: report.Host.OS, Architecture: report.Host.Architecture, CPUCount: report.Host.CPUCount, MemoryTotalMiB: report.Host.MemoryTotalMiB}
	seenGPU := make(map[string]struct{}, len(report.GPUs))
	for _, gpu := range report.GPUs {
		gpu.UUID = strings.TrimSpace(gpu.UUID)
		if gpu.UUID == "" || gpu.Index < 0 || gpu.MemoryTotalMiB <= 0 || gpu.MemoryUsedMiB < 0 {
			return domain.InventoryReport{}, fmt.Errorf("%w: invalid GPU inventory item", domain.ErrInvalidInput)
		}
		if _, duplicate := seenGPU[gpu.UUID]; duplicate {
			return domain.InventoryReport{}, fmt.Errorf("%w: duplicate GPU UUID %q", domain.ErrInvalidInput, gpu.UUID)
		}
		seenGPU[gpu.UUID] = struct{}{}
		state := domain.GPUFree
		if gpu.State == string(domain.GPUOccupiedUnknown) {
			state = domain.GPUOccupiedUnknown
		}
		result.GPUs = append(result.GPUs, domain.GPU{
			UUID: gpu.UUID, Index: gpu.Index, Model: gpu.Model,
			MemoryTotalMiB: gpu.MemoryTotalMiB, MemoryUsedMiB: gpu.MemoryUsedMiB,
			UtilizationPct: gpu.UtilizationPct, TemperatureC: gpu.TemperatureC,
			Healthy: gpu.Healthy, State: state,
		})
	}
	for _, container := range report.Containers {
		if strings.TrimSpace(container.ID) == "" {
			return domain.InventoryReport{}, fmt.Errorf("%w: container id is required", domain.ErrInvalidInput)
		}
		origin := domain.ContainerUnknown
		switch strings.ToUpper(container.Origin) {
		case string(domain.ContainerManaged):
			origin = domain.ContainerManaged
		case string(domain.ContainerLegacy):
			origin = domain.ContainerLegacy
		}
		result.Containers = append(result.Containers, domain.Container{
			ID: container.ID, Name: container.Name, Image: container.Image, State: container.State,
			Origin: origin, JobID: container.JobID, GPUUUIDs: append([]string(nil), container.GPUUUIDs...),
			Labels: cloneStrings(container.Labels), StartedAt: container.StartedAt,
			FinishedAt: container.FinishedAt, ExitCode: container.ExitCode,
		})
	}
	for _, process := range report.Processes {
		if process.PID <= 0 || strings.TrimSpace(process.GPUUUID) == "" {
			return domain.InventoryReport{}, fmt.Errorf("%w: invalid GPU process inventory item", domain.ErrInvalidInput)
		}
		result.Processes = append(result.Processes, domain.GPUProcess{
			PID: process.PID, GPUUUID: process.GPUUUID, UsedMemoryMiB: process.UsedMemoryMiB,
			ContainerID: process.ContainerID, ProcessName: process.ProcessName,
		})
	}
	return result, nil
}

func cloneStrings(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
