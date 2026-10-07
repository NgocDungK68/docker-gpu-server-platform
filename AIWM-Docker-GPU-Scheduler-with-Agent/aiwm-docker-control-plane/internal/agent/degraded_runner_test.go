package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

type observingControlPlane struct {
	fakeControlPlane
	registrations chan agentv1.RegisterRequest
	inventories   chan agentv1.InventoryReport
	heartbeats    chan struct{}
	attempts      atomic.Int32
}

func (c *observingControlPlane) Register(_ context.Context, r agentv1.RegisterRequest, _ string) (agentv1.RegisterResponse, error) {
	if c.attempts.Add(1) == 1 {
		return agentv1.RegisterResponse{}, errors.New("CP temporarily unavailable")
	}
	c.registrations <- r
	return agentv1.RegisterResponse{AgentID: "a", AgentToken: "test"}, nil
}
func (c *observingControlPlane) ReportInventory(_ context.Context, _, _ string, r agentv1.InventoryReport) error {
	select {
	case c.inventories <- r:
	default:
	}
	return nil
}
func (c *observingControlPlane) Heartbeat(context.Context, string, string, agentv1.HeartbeatRequest) error {
	select {
	case c.heartbeats <- struct{}{}:
	default:
	}
	return errors.New("temporary disconnect")
}

func TestDegradedRunnerRegistersReportsAndHeartbeatsDespiteOutage(t *testing.T) {
	for _, gpuDown := range []bool{false, true} {
		t.Run(map[bool]string{false: "docker unavailable", true: "nvml unavailable"}[gpuDown], func(t *testing.T) {
			docker := &probeRuntime{unavailable: !gpuDown, nvidia: true}
			reader := &probeGPU{unavailable: gpuDown, fakeGPUReader: fakeGPUReader{gpus: []agentv1.GPU{testGPU("g")}}}
			collector := newTestCollector(docker, reader, nil)
			api := &observingControlPlane{registrations: make(chan agentv1.RegisterRequest, 2), inventories: make(chan agentv1.InventoryReport, 8), heartbeats: make(chan struct{}, 8)}
			r := NewRunner(RunnerConfig{MachineID: "m", Name: "test", HeartbeatEvery: 20 * time.Millisecond, InventoryEvery: 20 * time.Millisecond, CommandPollEvery: 20 * time.Millisecond}, api, collector, NewCommandExecutor(docker, collector), &memoryStateStore{}, docker, slog.New(slog.NewTextHandler(io.Discard, nil)))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- r.Run(ctx) }()
			select {
			case registered := <-api.registrations:
				if registered.CapabilityReport == nil || registered.CapabilityReport.ManagedExecutionReady || contains(registered.Capabilities, "managed-container-v1") {
					t.Fatal("false execution advertisement")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("did not register")
			}
			select {
			case report := <-api.inventories:
				if report.Capabilities == nil || report.Capabilities.GPUInventoryAvailable == gpuDown {
					t.Fatal("wrong source validity")
				}
			case <-time.After(time.Second):
				t.Fatal("no partial inventory")
			}
			select {
			case <-api.heartbeats:
			case <-time.After(time.Second):
				t.Fatal("no heartbeat")
			}
			select {
			case err := <-done:
				t.Fatalf("Agent exited: %v", err)
			default:
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("Agent did not shut down")
			}
			if docker.starts != 0 || docker.stops != 0 {
				t.Fatal("degraded state disrupted containers")
			}
		})
	}
}
