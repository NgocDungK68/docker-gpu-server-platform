package memory

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

func TestCommitAssignmentIsAtomic(t *testing.T) {
	ctx := context.Background()
	store := New()
	now := time.Now().UTC()
	_, err := store.UpsertServer(ctx, domain.Server{
		ID: "server-1", MachineID: "machine-1", Name: "server-1", Status: domain.ServerOnline,
		LastHeartbeatAt: now, InventoryReceivedAt: now,
		GPUs: []domain.GPU{{UUID: "gpu-1", Healthy: true, State: domain.GPUFree}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"job-1", "job-2"} {
		if err := store.CreateJob(ctx, domain.Job{ID: id, Status: domain.JobQueued, Resources: domain.ResourceRequest{GPUCount: 1}, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	payload, _ := json.Marshal(agentv1.StartContainerPayload{JobID: "job-1"})
	placement := domain.Placement{ServerID: "server-1", GPUUUIDs: []string{"gpu-1"}}
	_, err = store.CommitAssignment(ctx, "job-1", placement, domain.Command{ID: "cmd-1", AgentID: "server-1", Payload: payload}, now)
	if err != nil {
		t.Fatalf("first CommitAssignment() error = %v", err)
	}
	_, err = store.CommitAssignment(ctx, "job-2", placement, domain.Command{ID: "cmd-2", AgentID: "server-1", Payload: payload}, now)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second CommitAssignment() error = %v, want ErrConflict", err)
	}
}

func TestInventoryClassifiesLegacyAndUnknownConsumers(t *testing.T) {
	ctx := context.Background()
	store := New()
	now := time.Now().UTC()
	_, err := store.UpsertServer(ctx, domain.Server{ID: "server-1", MachineID: "m-1", Name: "s-1", LastHeartbeatAt: now, InventoryReceivedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	server, err := store.ReplaceInventory(ctx, "server-1", domain.InventoryReport{
		Sequence: 1, ObservedAt: now,
		GPUs: []domain.GPU{
			{UUID: "gpu-0", Healthy: true}, {UUID: "gpu-1", Healthy: true}, {UUID: "gpu-2", Healthy: true},
		},
		Containers: []domain.Container{{ID: "old", State: "running", Origin: domain.ContainerLegacy, GPUUUIDs: []string{"gpu-0"}}},
		Processes:  []domain.GPUProcess{{PID: 42, GPUUUID: "gpu-1", UsedMemoryMiB: 1000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if server.GPUs[0].State != domain.GPUOccupiedLegacy {
		t.Fatalf("gpu-0 state = %s", server.GPUs[0].State)
	}
	if server.GPUs[1].State != domain.GPUOccupiedUnknown {
		t.Fatalf("gpu-1 state = %s", server.GPUs[1].State)
	}
	if server.GPUs[2].State != domain.GPUFree {
		t.Fatalf("gpu-2 state = %s", server.GPUs[2].State)
	}
}

func TestStoppedLegacyContainerDoesNotOccupyGPU(t *testing.T) {
	ctx := context.Background()
	store := New()
	now := time.Now().UTC()
	_, _ = store.UpsertServer(ctx, domain.Server{ID: "server-1", MachineID: "m-1", Name: "s-1", LastHeartbeatAt: now, InventoryReceivedAt: now})
	server, err := store.ReplaceInventory(ctx, "server-1", domain.InventoryReport{
		Sequence:   1,
		ObservedAt: now,
		GPUs:       []domain.GPU{{UUID: "gpu-0", Healthy: true}},
		Containers: []domain.Container{{ID: "old", State: "exited", Origin: domain.ContainerLegacy, GPUUUIDs: []string{"gpu-0"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if server.GPUs[0].State != domain.GPUFree {
		t.Fatalf("stopped legacy container left GPU state %s", server.GPUs[0].State)
	}
}
