package agent

import (
	"context"
	"errors"
	"testing"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/platform"
)

type probeRuntime struct {
	fakeRuntime
	unavailable bool
	nvidia      bool
}

func (r *probeRuntime) ListContainers(ctx context.Context) ([]RuntimeContainer, error) {
	if r.unavailable {
		return nil, errors.New("Docker unavailable")
	}
	return r.fakeRuntime.ListContainers(ctx)
}
func (r *probeRuntime) Compatibility(context.Context) (RuntimeCompatibility, error) {
	return RuntimeCompatibility{OS: "linux", Version: "29", APIVersion: "1.52", NVIDIARuntime: r.nvidia}, nil
}

type probeGPU struct {
	fakeGPUReader
	unavailable bool
}

func (r *probeGPU) Snapshot(ctx context.Context) ([]agentv1.GPU, []agentv1.GPUProcess, error) {
	if r.unavailable {
		return nil, nil, errors.New("NVML unavailable")
	}
	return r.fakeGPUReader.Snapshot(ctx)
}
func testPlatform() platform.Capabilities {
	return platform.Capabilities{OS: "linux", Architecture: "amd64", MachineIDAvailable: true, AgentOperational: true}
}

func TestCapabilityModesAndRecovery(t *testing.T) {
	docker := &probeRuntime{nvidia: true}
	reader := &probeGPU{fakeGPUReader: fakeGPUReader{gpus: []agentv1.GPU{testGPU("g")}}}
	for _, tc := range []struct {
		name                         string
		dockerDown, nvmlDown, nvidia bool
		mode                         platform.OperatingMode
		ready                        bool
	}{
		{"full", false, false, true, platform.Full, true},
		{"docker down", true, false, true, platform.GPUObserveOnly, false},
		{"docker recovery", false, false, true, platform.Full, true},
		{"missing nvidia", false, false, false, platform.Degraded, false},
		{"nvml down", false, true, true, platform.DockerObserveOnly, false},
		{"nvml recovery", false, false, true, platform.Full, true},
		{"both down", true, true, false, platform.Degraded, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			docker.unavailable, docker.nvidia, reader.unavailable = tc.dockerDown, tc.nvidia, tc.nvmlDown
			got := collectSources(context.Background(), docker, reader, testPlatform()).capabilities
			if got.OperatingMode != tc.mode || got.ManagedExecutionReady != tc.ready || !got.AgentOperational {
				t.Fatalf("%+v", got)
			}
			if got.GPUInventoryAvailable == tc.nvmlDown || got.DockerAvailable == tc.dockerDown {
				t.Fatalf("source validity: %+v", got)
			}
		})
	}
}

func TestUnsupportedPlatformStillProbesSources(t *testing.T) {
	base := testPlatform()
	base.OS = "windows"
	got := collectSources(context.Background(), &probeRuntime{nvidia: true}, &probeGPU{fakeGPUReader: fakeGPUReader{gpus: []agentv1.GPU{testGPU("g")}}}, base).capabilities
	if got.ManagedExecutionReady || !got.DockerAvailable || !got.GPUInventoryAvailable {
		t.Fatalf("%+v", got)
	}
}

func newTestCollector(docker DockerRuntime, reader GPUReader, resolver ProcessContainerResolver, policies ...InventoryPolicy) *InventoryCollector {
	c := NewInventoryCollector(docker, reader, resolver, policies...)
	c.platform = testPlatform
	return c
}
func (r *fakeRuntime) Compatibility(context.Context) (RuntimeCompatibility, error) {
	return RuntimeCompatibility{OS: "linux", Version: "fake-29", APIVersion: "1.52", NVIDIARuntime: true}, nil
}

func TestOptionalInventorySourcesKeepUsefulDataAndBlockExecution(t *testing.T) {
	docker := &probeRuntime{nvidia: true}
	docker.containers = []RuntimeContainer{{ID: "external", State: "running", GPUUUIDs: []string{"g"}}}
	reader := &probeGPU{fakeGPUReader: fakeGPUReader{
		gpus:      []agentv1.GPU{testGPU("g")},
		processes: []agentv1.GPUProcess{{PID: 42, GPUUUID: "g", ContainerID: "external"}},
	}}
	collector := newTestCollector(docker, reader, nil)
	ctx := context.Background()
	docker.unavailable = true
	report, err := collector.Snapshot(ctx, 1)
	if err != nil || len(report.GPUs) != 1 || len(report.Processes) != 1 || report.Capabilities.DockerAvailable || report.Processes[0].ContainerID != "" {
		t.Fatalf("GPU-only: %+v %v", report, err)
	}
	if err := collector.GPUsAvailable(ctx, []string{"g"}, "job"); err == nil {
		t.Fatal("degraded START allowed")
	}
	reader.unavailable = true
	docker.unavailable = false
	report, err = collector.Snapshot(ctx, 2)
	if err != nil || len(report.Containers) != 1 || report.Capabilities.GPUInventoryAvailable || report.Capabilities.ManagedExecutionReady || len(report.GPUs) != 0 {
		t.Fatalf("Docker-only: %+v %v", report, err)
	}
	reader.unavailable = false
	report, err = collector.Snapshot(ctx, 3)
	if err != nil || !report.Capabilities.ManagedExecutionReady {
		t.Fatalf("recovery: %+v %v", report, err)
	}
}
