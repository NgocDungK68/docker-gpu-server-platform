package agent

import (
	"context"
	"encoding/json"
	"errors"
	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

type failingState struct {
	value   PersistentState
	saves   int
	failAt  int
	loadErr error
}

func (s *failingState) Load() (PersistentState, error) { return s.value, s.loadErr }
func (s *failingState) Save(p PersistentState) error {
	s.saves++
	if s.saves >= s.failAt {
		return errors.New("disk unavailable")
	}
	// Keep the persisted value independent of in-memory command map mutations.
	data, _ := json.Marshal(p)
	return json.Unmarshal(data, &s.value)
}

type durabilityCP struct {
	fakeControlPlane
	reports atomic.Int32
}

func (*durabilityCP) Register(context.Context, agentv1.RegisterRequest, string) (agentv1.RegisterResponse, error) {
	return agentv1.RegisterResponse{AgentID: "a", AgentToken: "test-token"}, nil
}
func (c *durabilityCP) ReportInventory(context.Context, string, string, agentv1.InventoryReport) error {
	c.reports.Add(1)
	return nil
}

func TestLocalStateFailureStopsStartupAndWorkers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failAt  int
		reports int32
	}{
		{"identity", 2, 0}, {"initial sequence", 3, 0}, {"periodic sequence", 4, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := &probeRuntime{unavailable: true}
			collector := newTestCollector(runtime, fakeGPUReader{gpus: []agentv1.GPU{testGPU("g")}}, nil)
			api := &durabilityCP{}
			store := &failingState{failAt: tc.failAt}
			runner := NewRunner(RunnerConfig{MachineID: "m", HeartbeatEvery: time.Hour, InventoryEvery: time.Millisecond, CommandPollEvery: time.Hour},
				api, collector, NewCommandExecutor(runtime, collector), store, runtime, slog.New(slog.NewTextHandler(io.Discard, nil)))
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := runner.Run(ctx)
			if !errors.Is(err, errLocalState) || api.reports.Load() != tc.reports {
				t.Fatalf("failure swallowed or unpersisted inventory sent: %v reports=%d", err, api.reports.Load())
			}
			if runtime.starts != 0 || runtime.stops != 0 {
				t.Fatal("fatal state error disrupted containers")
			}
		})
	}
}

func TestUnpersistedCommandCannotBeAcknowledgedOrReplayedInMemory(t *testing.T) {
	runtime := &fakeRuntime{}
	collector := newTestCollector(runtime, fakeGPUReader{gpus: []agentv1.GPU{testGPU("g")}}, nil)
	payload, _ := json.Marshal(agentv1.StartContainerPayload{JobID: "job", Name: "job", Image: "test", GPUUUIDs: []string{"g"}})
	api := &fakeControlPlane{commands: []agentv1.Command{{ID: "start", Type: "START_CONTAINER", Payload: payload}}}
	store := &failingState{failAt: 1}
	runner := NewRunner(RunnerConfig{MachineID: "m"}, api, collector, NewCommandExecutor(runtime, collector), store, runtime, slog.New(slog.NewTextHandler(io.Discard, nil)))
	runner.persistent = PersistentState{AgentID: "a", AgentToken: "test-token", MachineID: "m", ProcessedCommands: map[string]CommandResult{}}
	for i := 0; i < 2; i++ {
		if err := runner.pollAndExecute(context.Background()); !errors.Is(err, errLocalState) {
			t.Fatal(err)
		}
	}
	if api.acks != 0 || runtime.starts != 1 || runtime.stops != 0 {
		t.Fatal("unsafe ACK, retry or stop", api.acks, runtime.starts, runtime.stops)
	}
	if len(store.value.ProcessedCommands) != 0 {
		t.Fatal("failed save altered durable state")
	}
}

func TestInvalidPersistedIdentityFailsBeforeAnySubsystemAccess(t *testing.T) {
	for _, p := range []PersistentState{
		{MachineID: "other"}, {AgentID: "a", AgentToken: "token"}, {MachineID: "m", AgentID: "a"},
	} {
		store := &failingState{value: p, failAt: 10}
		runner := NewRunner(RunnerConfig{MachineID: "m"}, nil, nil, nil, store, nil, nil)
		if err := runner.Run(context.Background()); err == nil || store.saves != 0 {
			t.Fatal("invalid state used")
		}
	}
	store := &failingState{loadErr: errors.New("corrupt state"), failAt: 10}
	runner := NewRunner(RunnerConfig{MachineID: "m"}, nil, nil, nil, store, nil, nil)
	if err := runner.Run(context.Background()); err == nil || store.saves != 0 {
		t.Fatal("load error overwritten")
	}
}
