package integration_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
	agentcp "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/controlplane"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/state"
	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/application"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/httpapi"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/store/memory"
)

func TestBrownfieldEndToEndLegacyWorkloadSurvives(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repository := memory.New()
	controlPlane := application.New(repository, application.Options{
		EnrollmentToken: "enroll", HeartbeatInterval: 20 * time.Millisecond,
		OfflineAfter: time.Second, CommandLease: 100 * time.Millisecond,
		DefaultStrategy: domain.StrategyBestFit,
	})
	httpServer := httptest.NewServer(httpapi.New(controlPlane, logger, nil).Handler())
	defer httpServer.Close()
	client, err := agentcp.New(httpServer.URL, time.Second, agentcp.TLSConfig{})
	if err != nil {
		t.Fatal(err)
	}

	runtime := &testRuntime{containers: []agent.RuntimeContainer{{
		ID: "legacy-container", Name: "business-training", Image: "old/image", State: "running",
		GPUUUIDs: []string{"GPU-0"}, Labels: map[string]string{"owner": "business-unit"},
	}}}
	gpuReader := testGPUReader{gpus: []agentv1.GPU{
		{UUID: "GPU-0", Index: 0, Model: "A100", MemoryTotalMiB: 81920, Healthy: true},
		{UUID: "GPU-1", Index: 1, Model: "A100", MemoryTotalMiB: 81920, Healthy: true},
	}}
	inventory := agent.NewInventoryCollector(runtime, gpuReader, nil)
	runner := agent.NewRunner(agent.RunnerConfig{
		MachineID: "machine-1", Name: "docker-gpu-1", AgentVersion: "test",
		EnrollmentToken: "enroll", HeartbeatEvery: 20 * time.Millisecond,
		InventoryEvery: 20 * time.Millisecond, CommandPollEvery: 10 * time.Millisecond,
	}, client, inventory, agent.NewCommandExecutor(runtime, inventory), state.NewFileStore(t.TempDir()+"/state.json"), runtime, logger)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()
	defer func() { cancel(); <-done }()

	waitFor(t, time.Second, func() bool {
		servers, _ := controlPlane.ListServers(context.Background())
		return len(servers) == 1 && len(servers[0].GPUs) == 2 && servers[0].GPUs[0].State == domain.GPUOccupiedLegacy
	})
	fp8 := false
	job, err := controlPlane.CreateJob(context.Background(), domain.CreateJobRequest{
		AllocationIntent: domain.AllocationIntent{WorkloadType: domain.WorkloadTraining, NecessityLevel: domain.Necessity2, NecessityReason: "GO_LIVE_90_DAYS", SystemImportance: domain.ImportanceImportant, NeededAt: time.Now().UTC().Format(time.RFC3339), TTLSeconds: 3600},
		Name:             "new-job", Image: "busybox", Resources: domain.AllocationResources{GPUCount: 1, MinVRAMMiB: 1024, PerformanceProfile: "a100-equivalent", FP8Required: &fp8},
	})
	if err != nil {
		t.Fatal(err)
	}
	if assigned, err := controlPlane.ScheduleOnce(context.Background()); err != nil || assigned != 1 {
		t.Fatalf("ScheduleOnce() = %d, %v", assigned, err)
	}
	waitFor(t, time.Second, func() bool {
		current, _ := controlPlane.GetJob(context.Background(), job.ID)
		return current.Status == domain.JobRunning
	})
	current, _ := controlPlane.GetJob(context.Background(), job.ID)
	if current.Assignment == nil || len(current.Assignment.GPUUUIDs) != 1 || current.Assignment.GPUUUIDs[0] != "GPU-1" {
		t.Fatalf("new job assignment = %#v, want free GPU-1", current.Assignment)
	}
	if !runtime.hasContainer("legacy-container") {
		t.Fatal("legacy workload was changed while starting a managed job")
	}
	if _, err := controlPlane.StopJob(context.Background(), job.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, func() bool {
		stopped, _ := controlPlane.GetJob(context.Background(), job.ID)
		return stopped.Status == domain.JobStopped
	})
	if !runtime.hasContainer("legacy-container") {
		t.Fatal("legacy workload was changed while stopping a managed job")
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

type testRuntime struct {
	mu         sync.Mutex
	containers []agent.RuntimeContainer
}

func (*testRuntime) Ping(context.Context) error              { return nil }
func (*testRuntime) Version(context.Context) (string, error) { return "29-test", nil }
func (r *testRuntime) ListContainers(context.Context) ([]agent.RuntimeContainer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]agent.RuntimeContainer(nil), r.containers...), nil
}
func (r *testRuntime) StartManagedContainer(_ context.Context, payload agentv1.StartContainerPayload) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := "managed-" + payload.JobID
	r.containers = append(r.containers, agent.RuntimeContainer{ID: id, Name: payload.Name, Image: payload.Image, State: "running", GPUUUIDs: append([]string(nil), payload.GPUUUIDs...), Labels: map[string]string{agentv1.ManagedLabel: "true", agentv1.JobIDLabel: payload.JobID}})
	return id, nil
}
func (r *testRuntime) StopManagedJob(_ context.Context, payload agentv1.StopContainerPayload) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, item := range r.containers {
		if item.Labels[agentv1.ManagedLabel] == "true" && item.Labels[agentv1.JobIDLabel] == payload.JobID {
			r.containers = append(r.containers[:index], r.containers[index+1:]...)
			return item.ID, nil
		}
	}
	return "", nil
}
func (*testRuntime) WatchContainerEvents(context.Context) (<-chan struct{}, <-chan error) {
	return make(chan struct{}), make(chan error)
}
func (*testRuntime) Close() error { return nil }
func (r *testRuntime) hasContainer(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range r.containers {
		if item.ID == id {
			return true
		}
	}
	return false
}

type testGPUReader struct{ gpus []agentv1.GPU }

func (r testGPUReader) Snapshot(context.Context) ([]agentv1.GPU, []agentv1.GPUProcess, error) {
	return append([]agentv1.GPU(nil), r.gpus...), nil, nil
}
func (testGPUReader) Close() error { return nil }
