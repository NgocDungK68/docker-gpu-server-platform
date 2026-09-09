package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

func TestInventoryClassifiesLegacyAndMapsNVMLProcess(t *testing.T) {
	runtime := &fakeRuntime{containers: []RuntimeContainer{
		{ID: "legacy-container-0001", Name: "old-training", State: "running", GPUUUIDs: []string{"GPU-0"}},
		{ID: "managed-container-2", Name: "new-training", State: "running", Labels: map[string]string{agentv1.ManagedLabel: "true", agentv1.JobIDLabel: "job-2"}},
	}}
	reader := fakeGPUReader{
		gpus:      []agentv1.GPU{testGPU("GPU-0"), testGPU("GPU-1"), testGPU("GPU-2")},
		processes: []agentv1.GPUProcess{{PID: 42, GPUUUID: "GPU-1", UsedMemoryMiB: 1024}},
	}
	collector := NewInventoryCollector(runtime, reader, fakeResolver{id: "managed-container-2"})
	report, err := collector.Snapshot(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if report.Sequence != 7 || report.Containers[0].Origin != "LEGACY" || report.Containers[1].Origin != "MANAGED" {
		t.Fatalf("unexpected inventory: %#v", report)
	}
	if report.Processes[0].ContainerID != "managed-container-2" {
		t.Fatalf("process mapped to %q", report.Processes[0].ContainerID)
	}
	if !contains(report.Containers[1].GPUUUIDs, "GPU-1") {
		t.Fatal("NVML GPU process was not joined to its Docker container")
	}
}

func TestUnboundedLegacyGPUGrantProtectsWholeServer(t *testing.T) {
	runtime := &fakeRuntime{containers: []RuntimeContainer{{ID: "old", State: "running", UnboundedGPUAccess: true}}}
	reader := fakeGPUReader{gpus: []agentv1.GPU{testGPU("GPU-0"), testGPU("GPU-1")}}
	report, err := NewInventoryCollector(runtime, reader, nil).Snapshot(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Containers[0].GPUUUIDs) != 2 {
		t.Fatalf("unbounded legacy grant protected %d GPUs, want 2", len(report.Containers[0].GPUUUIDs))
	}
}

func TestGPUAvailabilityRejectsLegacyAndUnmappedProcesses(t *testing.T) {
	runtime := &fakeRuntime{containers: []RuntimeContainer{{ID: "legacy", State: "running", GPUUUIDs: []string{"GPU-0"}}}}
	reader := fakeGPUReader{
		gpus:      []agentv1.GPU{testGPU("GPU-0"), testGPU("GPU-1"), testGPU("GPU-2")},
		processes: []agentv1.GPUProcess{{PID: 77, GPUUUID: "GPU-1"}},
	}
	collector := NewInventoryCollector(runtime, reader, fakeResolver{err: errors.New("not in Docker")})
	for _, uuid := range []string{"GPU-0", "GPU-1"} {
		if err := collector.GPUsAvailable(context.Background(), []string{uuid}, "job-new"); !errors.Is(err, ErrUnsafeGPU) {
			t.Fatalf("GPUsAvailable(%s) error = %v", uuid, err)
		}
	}
	if err := collector.GPUsAvailable(context.Background(), []string{"GPU-2"}, "job-new"); err != nil {
		t.Fatalf("free GPU rejected: %v", err)
	}
}

type fakeRuntime struct {
	containers []RuntimeContainer
	starts     int
	stops      int
}

func (f *fakeRuntime) Ping(context.Context) error              { return nil }
func (f *fakeRuntime) Version(context.Context) (string, error) { return "fake-29", nil }
func (f *fakeRuntime) ListContainers(context.Context) ([]RuntimeContainer, error) {
	return append([]RuntimeContainer(nil), f.containers...), nil
}
func (f *fakeRuntime) StartManagedContainer(_ context.Context, payload agentv1.StartContainerPayload) (string, error) {
	f.starts++
	id := "managed-" + payload.JobID
	now := time.Now().UTC()
	f.containers = append(f.containers, RuntimeContainer{ID: id, State: "running", GPUUUIDs: payload.GPUUUIDs, Labels: map[string]string{agentv1.ManagedLabel: "true", agentv1.JobIDLabel: payload.JobID}, StartedAt: &now})
	return id, nil
}
func (f *fakeRuntime) StopManagedJob(context.Context, agentv1.StopContainerPayload) (string, error) {
	f.stops++
	return "managed", nil
}
func (f *fakeRuntime) WatchContainerEvents(context.Context) (<-chan struct{}, <-chan error) {
	return make(chan struct{}), make(chan error)
}
func (f *fakeRuntime) Close() error { return nil }

type fakeGPUReader struct {
	gpus      []agentv1.GPU
	processes []agentv1.GPUProcess
}

func (f fakeGPUReader) Snapshot(context.Context) ([]agentv1.GPU, []agentv1.GPUProcess, error) {
	return append([]agentv1.GPU(nil), f.gpus...), append([]agentv1.GPUProcess(nil), f.processes...), nil
}
func (fakeGPUReader) Close() error { return nil }

type fakeResolver struct {
	id  string
	err error
}

func (f fakeResolver) ContainerID(int) (string, error) { return f.id, f.err }

func testGPU(uuid string) agentv1.GPU {
	return agentv1.GPU{UUID: uuid, Model: "A100", MemoryTotalMiB: 81920, Healthy: true, State: "FREE"}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
