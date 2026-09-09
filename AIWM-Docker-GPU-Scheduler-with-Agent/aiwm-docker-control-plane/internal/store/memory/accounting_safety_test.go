package memory

import (
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"testing"
)

func TestKnownContainerDoesNotHideUnexpectedGPUProcess(t *testing.T) {
	actual := normalizeGPUState("s", []domain.GPU{{UUID: "GPU-1", Healthy: true}},
		[]domain.Container{{ID: "c", State: "running", Origin: domain.ContainerLegacy, GPUUUIDs: []string{"GPU-0"}}},
		[]domain.GPUProcess{{PID: 42, ContainerID: "c", GPUUUID: "GPU-1"}}, nil)
	if actual[0].State != domain.GPUOccupiedUnknown {
		t.Fatalf("unexpected process was lost: %#v", actual)
	}
}
