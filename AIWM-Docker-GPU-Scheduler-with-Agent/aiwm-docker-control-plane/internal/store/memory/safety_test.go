package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Store, time.Time) {
	t.Helper()
	s := New()
	now := time.Now().UTC()
	_, err := s.UpsertServer(context.Background(), domain.Server{ID: "s", MachineID: "m", Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now,
		GPUs: []domain.GPU{{UUID: "GPU-0", Model: "A100", MemoryTotalMiB: 40000, Healthy: true, State: domain.GPUFree}}})
	if err != nil {
		t.Fatal(err)
	}
	return s, now
}
func queued(t *testing.T, s *Store, id string, priority int, now time.Time) {
	t.Helper()
	if err := s.CreateJob(context.Background(), domain.Job{ID: id, Status: domain.JobQueued, Priority: priority, CreatedAt: now, Resources: domain.ResourceRequest{GPUCount: 1}}); err != nil {
		t.Fatal(err)
	}
}
func assign(s *Store, id string, now time.Time) (domain.Job, error) {
	payload, _ := json.Marshal(map[string]string{"jobId": id})
	return s.CommitAssignment(context.Background(), id, domain.Placement{ServerID: "s", GPUUUIDs: []string{"GPU-0"}},
		domain.Command{ID: "cmd-" + id, AgentID: "s", Type: domain.CommandStartContainer, Status: domain.CommandPending, Payload: payload, CreatedAt: now}, now)
}
func TestConcurrentReservationHasOneWinner(t *testing.T) {
	s, now := fixture(t)
	for i := 0; i < 40; i++ {
		queued(t, s, fmt.Sprint(i), 0, now)
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := assign(s, id, now)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, domain.ErrConflict) {
				t.Errorf("unexpected: %v", err)
			}
		}(fmt.Sprint(i))
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("winners=%d", successes.Load())
	}
}
func TestInvalidReservationDoesNotMutateGPU(t *testing.T) {
	for _, uuids := range [][]string{{"GPU-0", "missing"}, {"GPU-0", "GPU-0"}, {}} {
		s, now := fixture(t)
		queued(t, s, "j", 0, now)
		job := s.jobs["j"]
		job.Resources.GPUCount = len(uuids)
		s.jobs["j"] = job
		_, err := s.CommitAssignment(context.Background(), "j", domain.Placement{ServerID: "s", GPUUUIDs: uuids}, domain.Command{ID: "c", AgentID: "s"}, now)
		if err == nil {
			t.Fatalf("accepted invalid UUIDs %v", uuids)
		}
		server, _ := s.GetServer(context.Background(), "s")
		if server.GPUs[0].State != domain.GPUFree {
			t.Fatal("partial reservation leaked")
		}
	}
}
func TestReservationRechecksServerAndInventory(t *testing.T) {
	for _, kind := range []string{"offline", "drained", "stale-inventory", "missing-inventory"} {
		t.Run(kind, func(t *testing.T) {
			s, now := fixture(t)
			queued(t, s, "j", 0, now)
			server := s.servers["s"]
			switch kind {
			case "offline":
				server.Status = domain.ServerOffline
			case "drained":
				server.Drained = true
			case "stale-inventory":
				server.InventoryReceivedAt = now.Add(-time.Hour)
			case "missing-inventory":
				server.InventoryReceivedAt = time.Time{}
			}
			s.servers["s"] = server
			if _, err := assign(s, "j", now); !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("got %v", err)
			}
		})
	}
}
func TestQueueStorageFIFOAndStableTie(t *testing.T) {
	s, now := fixture(t)
	queued(t, s, "low", 1, now.Add(-time.Hour))
	queued(t, s, "later", 9, now.Add(time.Second))
	queued(t, s, "b", 9, now)
	queued(t, s, "a", 9, now)
	jobs, _ := s.ListQueuedJobs(context.Background())
	for i, want := range []string{"low", "a", "b", "later"} {
		if jobs[i].ID != want {
			t.Fatalf("index %d=%s", i, jobs[i].ID)
		}
	}
}
func TestFailedStartReleasesAndDuplicateACKCannotResurrect(t *testing.T) {
	s, now := fixture(t)
	queued(t, s, "j", 0, now)
	_, _ = assign(s, "j", now)
	_, err := s.AckCommand(context.Background(), "s", "cmd-j", domain.CommandAckRequest{Succeeded: false}, now)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.AckCommand(context.Background(), "s", "cmd-j", domain.CommandAckRequest{Succeeded: true}, now.Add(time.Second))
	job, _ := s.GetJob(context.Background(), "j")
	server, _ := s.GetServer(context.Background(), "s")
	if job.Status != domain.JobFailed || server.GPUs[0].State != domain.GPUFree || job.Assignment.ReservationState != "RELEASED" {
		t.Fatalf("job=%+v gpu=%+v", job, server.GPUs[0])
	}
}
func TestObservedLifecycleReleasesResources(t *testing.T) {
	for _, kind := range []string{"success", "failure", "missing", "stopped"} {
		t.Run(kind, func(t *testing.T) {
			s, now := fixture(t)
			queued(t, s, "j", 0, now)
			_, _ = assign(s, "j", now)
			code := 0
			container := domain.Container{ID: "c", JobID: "j", Origin: domain.ContainerManaged, State: "running", GPUUUIDs: []string{"GPU-0"}}
			report := domain.InventoryReport{Sequence: 1, ObservedAt: now, GPUs: []domain.GPU{{UUID: "GPU-0", Healthy: true}}, Containers: []domain.Container{container}}
			_, err := s.ReplaceInventory(context.Background(), "s", report)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = s.AckCommand(context.Background(), "s", "cmd-j", domain.CommandAckRequest{Succeeded: true}, now)
			running, _ := s.GetJob(context.Background(), "j")
			if running.Status != domain.JobRunning {
				t.Fatal("ACK regressed RUNNING")
			}
			if kind == "stopped" {
				_, _ = s.RequestStop(context.Background(), "j", domain.Command{ID: "stop", Type: domain.CommandStopContainer}, now)
			}
			want := domain.JobSucceeded
			if kind == "failure" {
				code = 1
				want = domain.JobFailed
			}
			container.State, container.ExitCode = "exited", &code
			report.Containers = []domain.Container{container}
			if kind == "missing" {
				report.Containers = nil
				want = domain.JobFailed
			}
			if kind == "stopped" {
				want = domain.JobStopped
			}
			report.Sequence = 2
			report.ObservedAt = now.Add(time.Second)
			_, err = s.ReplaceInventory(context.Background(), "s", report)
			if err != nil {
				t.Fatal(err)
			}
			job, _ := s.GetJob(context.Background(), "j")
			server, _ := s.GetServer(context.Background(), "s")
			if job.Status != want || job.Assignment.ReservationState != "RELEASED" || server.GPUs[0].State != domain.GPUFree {
				t.Fatalf("job=%+v server=%+v", job, server)
			}
		})
	}
}
func TestCancelQueuedJobAndOfflineInventoryPreserved(t *testing.T) {
	s, now := fixture(t)
	queued(t, s, "j", 0, now)
	job, err := s.RequestStop(context.Background(), "j", domain.Command{}, now)
	if err != nil || job.Status != domain.JobCancelled {
		t.Fatalf("cancel: %v %+v", err, job)
	}
	_, _ = s.MarkStaleServers(context.Background(), now.Add(time.Minute))
	server, _ := s.GetServer(context.Background(), "s")
	if server.Status != domain.ServerOffline || len(server.GPUs) != 1 {
		t.Fatal("last-known inventory lost")
	}
	_, _ = s.SetServerDrained(context.Background(), "s", false)
	server, _ = s.GetServer(context.Background(), "s")
	if server.Status != domain.ServerOffline {
		t.Fatal("undrain revived offline agent")
	}
}
