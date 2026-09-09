package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

func TestRunnerExecutesRedeliveredCommandOnlyOnce(t *testing.T) {
	runtime := &fakeRuntime{}
	reader := fakeGPUReader{gpus: []agentv1.GPU{testGPU("GPU-0")}}
	inventory := NewInventoryCollector(runtime, reader, nil)
	payload, _ := json.Marshal(agentv1.StartContainerPayload{JobID: "job-1", Name: "job-1", Image: "busybox", GPUUUIDs: []string{"GPU-0"}})
	api := &fakeControlPlane{commands: []agentv1.Command{{ID: "cmd-1", Type: "START_CONTAINER", Payload: payload}}}
	store := &memoryStateStore{}
	runner := NewRunner(RunnerConfig{MachineID: "m1", MaxCommandHistory: 10}, api, inventory, NewCommandExecutor(runtime, inventory), store, runtime, slog.New(slog.NewTextHandler(io.Discard, nil)))
	runner.persistent = PersistentState{AgentID: "a1", AgentToken: "token", MachineID: "m1", ProcessedCommands: make(map[string]CommandResult)}

	if err := runner.pollAndExecute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runner.pollAndExecute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runtime.starts != 1 {
		t.Fatalf("container starts = %d, want 1", runtime.starts)
	}
	if api.acks != 2 {
		t.Fatalf("ACK count = %d, want 2 for redelivery", api.acks)
	}
}

type fakeControlPlane struct {
	commands []agentv1.Command
	acks     int
}

func (*fakeControlPlane) Register(context.Context, agentv1.RegisterRequest, string) (agentv1.RegisterResponse, error) {
	return agentv1.RegisterResponse{}, nil
}
func (*fakeControlPlane) Heartbeat(context.Context, string, string, agentv1.HeartbeatRequest) error {
	return nil
}
func (*fakeControlPlane) ReportInventory(context.Context, string, string, agentv1.InventoryReport) error {
	return nil
}
func (f *fakeControlPlane) PollCommands(context.Context, string, string, int) ([]agentv1.Command, error) {
	return f.commands, nil
}
func (f *fakeControlPlane) AckCommand(context.Context, string, string, string, agentv1.CommandAckRequest) error {
	f.acks++
	return nil
}

type memoryStateStore struct{ value PersistentState }

func (m *memoryStateStore) Load() (PersistentState, error)   { return m.value, nil }
func (m *memoryStateStore) Save(value PersistentState) error { m.value = value; return nil }
