package application

import (
	"errors"
	"testing"
	"time"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/platform"
)

func fullAgentCapabilities() *platform.Capabilities {
	c := &platform.Capabilities{AgentOperational: true, MachineIDAvailable: true, OS: "linux", Architecture: "amd64", DockerAvailable: true, DockerOS: "linux", DockerAPIVersion: "1.52", NVMLAvailable: true, GPUInventoryAvailable: true, GPUCount: 1, NVIDIAContainerSupport: true}
	c.Normalize()
	return c
}

func TestPartialInventoryPreservesGPUAndRunningJob(t *testing.T) {
	f := newPlanningFixture(t)
	j := f.create(f.now, time.Hour, domain.Necessity2, domain.ImportanceImportant)
	f.cycle()
	for _, c := range f.repo.Export().Commands {
		if c.Type == domain.CommandStartContainer {
			_, err := f.repo.AckCommand(f.ctx, "s", c.ID, domain.CommandAckRequest{Succeeded: true, ContainerID: "managed"}, f.now)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	wire := agentv1.InventoryReport{ObservedAt: f.now, Capabilities: fullAgentCapabilities(), GPUs: []agentv1.GPU{{UUID: "GPU-a", Model: "A100", MemoryTotalMiB: 40960, Healthy: true}}, Containers: []agentv1.Container{{ID: "managed", JobID: j.ID, Origin: "MANAGED", State: "running", GPUUUIDs: []string{"GPU-a"}}}}
	send := func() domain.Server {
		t.Helper()
		f.sequence++
		wire.Sequence = f.sequence
		s, err := f.cp.ReportInventory(f.ctx, "s", wire)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	send()
	if f.get(j.ID).Status != domain.JobRunning {
		t.Fatal("full report did not confirm RUNNING")
	}
	wire.Capabilities.DockerAvailable = false
	wire.Containers = nil
	s := send()
	if len(s.Containers) != 1 || f.get(j.ID).Status != domain.JobRunning || s.GPUs[0].State != domain.GPUAllocated {
		t.Fatalf("Docker failure discarded state: %+v", s)
	}
	wire.Capabilities.DockerAvailable = true
	wire.Capabilities.GPUInventoryAvailable = false
	wire.Capabilities.NVMLAvailable = false
	wire.GPUs = nil
	s = send()
	if len(s.GPUs) != 1 || s.GPUs[0].State != domain.GPUAllocated || s.GPUs[0].AssignedJobID != j.ID || f.get(j.ID).Status != domain.JobRunning {
		t.Fatalf("NVML failure freed assignment: %+v", s)
	}
	if s.Schedulable(f.now, time.Minute) {
		t.Fatal("partial report became schedulable")
	}
	visible, err := f.cp.GetServer(f.ctx, "s")
	if err != nil || visible.Capabilities.OperatingMode != platform.DockerObserveOnly {
		t.Fatalf("capability not presented: %+v %v", visible, err)
	}
	visible.Capabilities.ManagedExecutionReady = true
	stored, _ := f.repo.GetServer(f.ctx, "s")
	if stored.Capabilities.ManagedExecutionReady {
		t.Fatal("capability pointer escaped store")
	}
	wire.Capabilities = fullAgentCapabilities()
	wire.GPUs = []agentv1.GPU{{UUID: "GPU-a", Model: "A100", MemoryTotalMiB: 40960, Healthy: true}}
	wire.Containers = []agentv1.Container{{ID: "managed", JobID: j.ID, Origin: "MANAGED", State: "running", GPUUUIDs: []string{"GPU-a"}}}
	s = send()
	if !s.Schedulable(f.now, time.Minute) || s.Capabilities.OperatingMode != platform.Full || f.get(j.ID).Status != domain.JobRunning {
		t.Fatal("full observation did not recover readiness")
	}
}

func TestExecutionCapabilityGatePrecedesScoringAndCommit(t *testing.T) {
	for _, mode := range []string{"missing", "docker-down", "nvml-down", "no-runtime"} {
		t.Run(mode, func(t *testing.T) {
			f := newPlanningFixture(t)
			j := f.create(f.now, time.Hour, domain.Necessity2, domain.ImportanceImportant)
			s, _ := f.repo.GetServer(f.ctx, "s")
			s.Capabilities = fullAgentCapabilities()
			switch mode {
			case "missing":
				s.Capabilities = nil
			case "docker-down":
				s.Capabilities.DockerAvailable = false
			case "nvml-down":
				s.Capabilities.NVMLAvailable = false
			case "no-runtime":
				s.Capabilities.NVIDIAContainerSupport = false
			}
			if s.Capabilities != nil {
				s.Capabilities.Normalize()
			}
			if _, err := f.repo.UpsertServer(f.ctx, s); err != nil {
				t.Fatal(err)
			}
			s.InventoryReceivedAt = f.now
			scored := false
			scheduler := NewScheduler(domain.StrategyBestFit, time.Minute).WithPolicy(domain.StrategyBestFit,
				scoreFunc(func(domain.Server, []domain.GPU, []domain.GPU) float64 { scored = true; return 0 }))
			if _, err := scheduler.Plan(j, []domain.Server{s}, f.now); err == nil || scored {
				t.Fatal("unready server reached placement scoring")
			}
			f.inventory(nil)
			_, err := f.repo.CommitAssignment(f.ctx, j.ID, domain.Placement{ServerID: "s", GPUUUIDs: []string{"GPU-a"}},
				domain.Command{ID: "unsafe", AgentID: "s", Type: domain.CommandStartContainer}, f.now)
			if !errors.Is(err, domain.ErrConflict) || len(f.repo.Export().Commands) != 0 || f.get(j.ID).Assignment != nil {
				t.Fatal("commit bypassed capability gate", err)
			}
		})
	}
}
