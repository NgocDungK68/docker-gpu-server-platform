package application

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/store/memory"
)

type planningFixture struct {
	t        *testing.T
	cp       *ControlPlane
	repo     *memory.Store
	now      time.Time
	ctx      context.Context
	sequence uint64
}

func newPlanningFixture(t *testing.T) *planningFixture {
	t.Helper()
	f := &planningFixture{t: t, now: time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC), ctx: allocationContext(), repo: memory.New(time.Minute)}
	f.cp = New(f.repo, Options{OfflineAfter: time.Minute, CommandLease: time.Second})
	f.cp.now = func() time.Time { return f.now }
	_, err := f.repo.UpsertServer(f.ctx, domain.Server{ID: "s", MachineID: "m", OrganizationID: "test-org", Status: domain.ServerOnline, LastHeartbeatAt: f.now})
	if err != nil {
		t.Fatal(err)
	}
	f.inventory(nil)
	return f
}
func (f *planningFixture) inventory(containers []domain.Container) {
	f.t.Helper()
	f.sequence++
	_, err := f.repo.ReplaceInventory(f.ctx, "s", domain.InventoryReport{Sequence: f.sequence, ObservedAt: f.now, ReceivedAt: f.now,
		GPUs: []domain.GPU{{UUID: "GPU-a", Model: "A100", MemoryTotalMiB: 40960, Healthy: true, State: domain.GPUFree}}, Containers: containers})
	if err != nil {
		f.t.Fatal(err)
	}
}
func (f *planningFixture) create(start time.Time, duration time.Duration, level domain.NecessityLevel, importance domain.SystemImportance) domain.Job {
	f.t.Helper()
	r := validAllocation()
	r.NeededAt = start.Format(time.RFC3339)
	r.TTLSeconds = int64(duration / time.Second)
	r.NecessityLevel, r.NecessityReason, r.NecessityExplanation = level, "CUSTOM", "Planning test"
	r.SystemImportance = importance
	j, err := f.cp.CreateJob(f.ctx, r)
	if err != nil {
		f.t.Fatal(err)
	}
	return j
}
func (f *planningFixture) cycle() {
	f.t.Helper()
	if _, err := f.cp.ScheduleOnce(f.ctx); err != nil {
		f.t.Fatal(err)
	}
}
func (f *planningFixture) get(id string) domain.Job {
	f.t.Helper()
	j, err := f.repo.GetJob(f.ctx, id)
	if err != nil {
		f.t.Fatal(err)
	}
	return j
}
func (f *planningFixture) starts() int {
	n := 0
	for _, c := range f.repo.Export().Commands {
		if c.Type == domain.CommandStartContainer {
			n++
		}
	}
	return n
}

func TestFutureReservationDoesNotExecuteEarly(t *testing.T) {
	f := newPlanningFixture(t)
	start := f.now.Add(6 * time.Hour)
	j := f.create(start, 2*time.Hour, domain.Necessity2, domain.ImportanceImportant)
	f.cycle()
	planned := f.get(j.ID)
	if planned.Status != domain.JobAssigned || planned.Assignment == nil || planned.Assignment.ReservationState != "PLANNED" || !planned.Assignment.StartAt.Equal(start) || !planned.Assignment.EndAt.Equal(start.Add(2*time.Hour)) {
		t.Fatalf("bad reservation: %+v", planned)
	}
	if f.starts() != 0 {
		t.Fatal("early START created")
	}
	s, _ := f.repo.GetServer(f.ctx, "s")
	if s.GPUs[0].State != domain.GPUFree {
		t.Fatal("future calendar changed current physical state")
	}
	f.now = start.Add(-time.Nanosecond)
	f.inventory(nil)
	f.cycle()
	if f.starts() != 0 {
		t.Fatal("START before exact endpoint")
	}
	f.now = start
	f.inventory(nil)
	f.cycle()
	if f.starts() != 1 || f.get(j.ID).Status != domain.JobStarting {
		t.Fatal("did not dispatch at start")
	}
	// An inventory racing ahead of command completion must not release STARTING.
	f.inventory(nil)
	if f.get(j.ID).Status != domain.JobStarting {
		t.Fatal("premature missing-container failure")
	}
	f.cycle()
	if f.starts() != 1 {
		t.Fatal("duplicate START")
	}
}

func TestReservationIntervalsAndPolicyReplanning(t *testing.T) {
	for _, tc := range []struct {
		name             string
		secondOffset     time.Duration
		level            domain.NecessityLevel
		importance       domain.SystemImportance
		both, secondWins bool
	}{
		{"touching", 2 * time.Hour, domain.Necessity2, domain.ImportanceImportant, true, false},
		{"overlap-fifo", time.Hour, domain.Necessity2, domain.ImportanceImportant, false, false},
		{"necessity-wins", 0, domain.Necessity1, domain.ImportanceNormal, false, true},
		{"auxiliary-same-necessity", 0, domain.Necessity2, domain.ImportanceCritical, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPlanningFixture(t)
			start := f.now.Add(6 * time.Hour)
			a := f.create(start, 2*time.Hour, domain.Necessity2, domain.ImportanceImportant)
			f.cycle()
			f.now = f.now.Add(time.Second)
			b := f.create(start.Add(tc.secondOffset), 2*time.Hour, tc.level, tc.importance)
			f.cycle()
			aa, bb := f.get(a.ID), f.get(b.ID)
			if (aa.Assignment != nil) != (tc.both || !tc.secondWins) || (bb.Assignment != nil) != (tc.both || tc.secondWins) {
				t.Fatalf("wrong winner: %s/%s", aa.Status, bb.Status)
			}
			if tc.both && aa.Assignment.GPUUUIDs[0] != bb.Assignment.GPUUUIDs[0] {
				t.Fatal("should reuse same GPU in non-overlap")
			}
			if tc.secondWins && aa.StatusReason != waitingWindow {
				t.Fatal("missing conflict reason")
			}
			if f.starts() != 0 {
				t.Fatal("planning dispatched a future workload")
			}
			before := f.repo.Export()
			f.cycle()
			after := f.repo.Export()
			if !reflect.DeepEqual(before.Jobs, after.Jobs) || len(after.Commands) != 0 {
				t.Fatal("same snapshot/clock did not plan deterministically")
			}
		})
	}
}

func TestPlanningOrganizationIsolation(t *testing.T) {
	f := newPlanningFixture(t)
	_, err := f.repo.SetServerDrained(f.ctx, "s", true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.repo.UpsertServer(f.ctx, domain.Server{ID: "vds", MachineID: "vds", OrganizationID: "vds", Status: domain.ServerOnline, LastHeartbeatAt: f.now, InventoryReceivedAt: f.now,
		GPUs: []domain.GPU{{UUID: "GPU-vds", Model: "A100", MemoryTotalMiB: 40960, Healthy: true, State: domain.GPUFree}}})
	if err != nil {
		t.Fatal(err)
	}
	j := f.create(f.now.Add(time.Hour), time.Hour, domain.Necessity1, domain.ImportanceCritical)
	f.cycle()
	if f.get(j.ID).Assignment != nil || f.starts() != 0 {
		t.Fatal("cross-organization reservation")
	}
}

func TestExecutionRevalidatesOfflineAndExternalOccupancy(t *testing.T) {
	for _, kind := range []string{"offline", "legacy", "unknown", "unhealthy"} {
		t.Run(kind, func(t *testing.T) {
			f := newPlanningFixture(t)
			start := f.now.Add(time.Hour)
			j := f.create(start, time.Hour, domain.Necessity2, domain.ImportanceImportant)
			f.cycle()
			f.now = start
			switch kind {
			case "legacy":
				f.inventory([]domain.Container{{ID: "external", Origin: domain.ContainerLegacy, State: "running", GPUUUIDs: []string{"GPU-a"}}})
			case "unknown", "unhealthy":
				f.inventory(nil)
				snap := f.repo.Export()
				s := snap.Servers["s"]
				if kind == "unknown" {
					s.GPUs[0].State = domain.GPUOccupiedUnknown
				} else {
					s.GPUs[0].Healthy = false
				}
				snap.Servers["s"] = s
				f.repo.Restore(snap)
			}
			f.cycle()
			if f.starts() != 0 || f.get(j.ID).Status != domain.JobAssigned {
				t.Fatal("unsafe execution")
			}
			if f.get(j.ID).StatusReason == "" {
				t.Fatal("missing retry reason")
			}
			// A fresh, safe report allows retry inside the original interval, without moving its end.
			f.now = f.now.Add(time.Minute)
			f.inventory(nil)
			f.cycle()
			if f.starts() != 1 || !f.get(j.ID).Assignment.EndAt.Equal(start.Add(time.Hour)) {
				t.Fatal("safe retry failed or extended duration")
			}
		})
	}
}

func TestReservationEndStopsManagedAndReleasesOnlyOnObservation(t *testing.T) {
	f := newPlanningFixture(t)
	start := f.now.Add(time.Hour)
	j := f.create(start, time.Hour, domain.Necessity2, domain.ImportanceImportant)
	f.cycle()
	f.now = start
	f.inventory(nil)
	f.cycle()
	commands, err := f.repo.LeaseCommands(f.ctx, "s", f.now, time.Second, 10)
	if err != nil || len(commands) != 1 {
		t.Fatalf("lease: %v %v", commands, err)
	}
	_, err = f.repo.AckCommand(f.ctx, "s", commands[0].ID, domain.CommandAckRequest{Succeeded: true, ContainerID: "managed"}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	container := domain.Container{ID: "managed", JobID: j.ID, Origin: domain.ContainerManaged, State: "running", GPUUUIDs: []string{"GPU-a"}}
	f.inventory([]domain.Container{container})
	if f.get(j.ID).Status != domain.JobRunning {
		t.Fatal("not running")
	}
	// Higher-priority demand cannot evict an execution that has started.
	f.create(f.now, 2*time.Hour, domain.Necessity1, domain.ImportanceCritical)
	f.cycle()
	if f.get(j.ID).Status != domain.JobRunning {
		t.Fatal("running workload preempted")
	}
	f.now = start.Add(time.Hour)
	f.inventory([]domain.Container{container})
	f.cycle()
	if f.get(j.ID).Status != domain.JobStopping || f.get(j.ID).Assignment.ReservationState == "RELEASED" {
		t.Fatal("released without observed stop")
	}
	commands, err = f.repo.LeaseCommands(f.ctx, "s", f.now, time.Second, 10)
	if err != nil || len(commands) != 1 || commands[0].Type != domain.CommandStopContainer {
		t.Fatalf("missing graceful STOP: %v %v", commands, err)
	}
	_, err = f.repo.AckCommand(f.ctx, "s", commands[0].ID, domain.CommandAckRequest{Succeeded: true}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if f.get(j.ID).Assignment.ReservationState == "RELEASED" {
		t.Fatal("ACK alone released resources")
	}
	code := 0
	container.State = "exited"
	container.ExitCode = &code
	f.inventory([]domain.Container{container})
	s, _ := f.repo.GetServer(f.ctx, "s")
	if f.get(j.ID).Status != domain.JobStopped || f.get(j.ID).Assignment.ReservationState != "RELEASED" || s.GPUs[0].State != domain.GPUFree {
		t.Fatal("stop/release failed")
	}
}

func TestDelayedPollCannotStartExpiredOrUnsafeReservation(t *testing.T) {
	for _, kind := range []string{"expired", "external"} {
		t.Run(kind, func(t *testing.T) {
			f := newPlanningFixture(t)
			j := f.create(f.now, time.Minute, domain.Necessity2, domain.ImportanceImportant)
			f.cycle()
			if f.starts() != 1 {
				t.Fatal("missing current START")
			}
			if kind == "expired" {
				f.now = f.now.Add(time.Minute)
				f.inventory(nil)
			} else {
				f.inventory([]domain.Container{{ID: "legacy", Origin: domain.ContainerLegacy, State: "running", GPUUUIDs: []string{"GPU-a"}}})
			}
			cmds, err := f.repo.LeaseCommands(f.ctx, "s", f.now, time.Second, 10)
			if err != nil || len(cmds) != 0 {
				t.Fatal("unsafe delayed START delivered")
			}
			if kind == "expired" {
				f.cycle()
				if !f.get(j.ID).Status.Terminal() || f.get(j.ID).Assignment.ReservationState != "RELEASED" {
					t.Fatal("undelivered expiration leaked reservation")
				}
			}
		})
	}
}

func TestConcurrentPlanningKeepsSingleOverlappingReservation(t *testing.T) {
	f := newPlanningFixture(t)
	for i := 0; i < 4; i++ {
		f.create(f.now.Add(time.Hour), time.Hour, domain.Necessity2, domain.ImportanceImportant)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.cp.ScheduleOnce(f.ctx); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	for _, j := range f.repo.Export().Jobs {
		if j.Assignment != nil {
			n++
		}
	}
	if n != 1 || f.starts() != 0 {
		t.Fatal("non-atomic planning")
	}
}

func TestTimeAwarePreviewIsReadOnlyAndPolicyAware(t *testing.T) {
	f := newPlanningFixture(t)
	start := f.now.Add(time.Hour)
	f.create(start, time.Hour, domain.Necessity2, domain.ImportanceImportant)
	f.cycle()
	f.now = f.now.Add(time.Second)
	r := validAllocation()
	r.NeededAt = start.Format(time.RFC3339)
	r.NecessityLevel = domain.Necessity1
	r.NecessityReason = "EXECUTIVE_DIRECTION"
	before := f.repo.Export()
	p, err := f.cp.PreviewJob(f.ctx, r)
	if err != nil || !p.ResourceMatch.Satisfiable || p.PlanningStatus != "AVAILABLE" || !p.StartAt.Equal(start) {
		t.Fatalf("%+v %v", p, err)
	}
	if !reflect.DeepEqual(before, f.repo.Export()) {
		t.Fatal("preview reserved or replanned")
	}
	r.NecessityLevel = domain.Necessity4
	r.NecessityReason = "MODEL_EXPERIMENT"
	p, err = f.cp.PreviewJob(f.ctx, r)
	if err != nil || p.ResourceMatch.Satisfiable || p.PlanningStatus != "CONFLICT" {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestExpiredDeliveredStartWithLostACKCanStillStop(t *testing.T) {
	f := newPlanningFixture(t)
	j := f.create(f.now, time.Minute, domain.Necessity2, domain.ImportanceImportant)
	f.cycle()
	cmds, err := f.repo.LeaseCommands(f.ctx, "s", f.now, time.Second, 10)
	if err != nil || len(cmds) != 1 {
		t.Fatal(cmds, err)
	}
	// Agent might have executed START and lost its ACK; CP cannot assume the GPU is free.
	f.now = f.now.Add(time.Minute)
	f.inventory(nil)
	f.cycle()
	if f.get(j.ID).Status != domain.JobStopping {
		t.Fatal("unresolved execution must retain ownership")
	}
	cmds, err = f.repo.LeaseCommands(f.ctx, "s", f.now, time.Second, 10)
	if err != nil || len(cmds) != 1 || cmds[0].Type != domain.CommandStopContainer {
		t.Fatal("STOP blocked behind expired START", cmds, err)
	}
	_, err = f.repo.AckCommand(f.ctx, "s", cmds[0].ID, domain.CommandAckRequest{Succeeded: true}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	f.inventory(nil)
	if f.get(j.ID).Status != domain.JobStopped || f.get(j.ID).Assignment.ReservationState != "RELEASED" {
		t.Fatal("confirmed STOP did not release")
	}
}

func TestPlanningAfterRunningReservationAndCancellation(t *testing.T) {
	f := newPlanningFixture(t)
	a := f.create(f.now, time.Hour, domain.Necessity2, domain.ImportanceImportant)
	f.cycle()
	b := f.create(f.now.Add(time.Hour), time.Hour, domain.Necessity2, domain.ImportanceImportant)
	f.cycle()
	if f.get(b.ID).Assignment == nil || f.starts() != 1 {
		t.Fatal("could not plan after current allocation")
	}
	_, err := f.cp.StopJob(f.ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if f.get(b.ID).Status != domain.JobCancelled || f.get(b.ID).Assignment.ReservationState != "RELEASED" {
		t.Fatal("future cancel failed")
	}
	s, _ := f.repo.GetServer(f.ctx, "s")
	if s.GPUs[0].AssignedJobID != a.ID || s.GPUs[0].State != domain.GPUReserved {
		t.Fatal("future cancellation freed current owner's GPU")
	}
}

func TestExpiredBlockedReservationNeverStopsExternalWorkload(t *testing.T) {
	f := newPlanningFixture(t)
	start := f.now.Add(time.Hour)
	j := f.create(start, time.Minute, domain.Necessity2, domain.ImportanceImportant)
	f.cycle()
	f.now = start
	external := domain.Container{ID: "external", Origin: domain.ContainerLegacy, State: "running", GPUUUIDs: []string{"GPU-a"}}
	f.inventory([]domain.Container{external})
	f.cycle()
	f.now = start.Add(time.Minute)
	f.inventory([]domain.Container{external})
	f.cycle()
	s, _ := f.repo.GetServer(f.ctx, "s")
	if f.get(j.ID).Status != domain.JobCancelled || len(f.repo.Export().Commands) != 0 || s.GPUs[0].State != domain.GPUOccupiedLegacy || s.Containers[0].State != "running" {
		t.Fatal("expiration affected an external workload")
	}
}

func TestRequestedTimeValidation(t *testing.T) {
	f := newPlanningFixture(t)
	for _, tc := range []struct {
		offset   time.Duration
		duration int64
		valid    bool
	}{
		{-61 * time.Second, 3600, false}, {-30 * time.Second, 10, false}, {-30 * time.Second, 3600, true}, {time.Hour, 0, false}, {time.Hour, 2592001, false},
	} {
		r := validAllocation()
		r.NeededAt = f.now.Add(tc.offset).Format(time.RFC3339Nano)
		r.TTLSeconds = tc.duration
		_, err := f.cp.PreviewJob(f.ctx, r)
		if (err == nil) != tc.valid {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}

func TestStaleQueuedExpirationCannotReleaseDispatchedJob(t *testing.T) {
	f := newPlanningFixture(t)
	j := f.create(f.now, time.Minute, domain.Necessity2, domain.ImportanceImportant)
	f.cycle()
	commands, err := f.repo.LeaseCommands(f.ctx, "s", f.now, time.Second, 10)
	if err != nil || len(commands) != 1 {
		t.Fatal(commands, err)
	}
	f.now = f.now.Add(time.Minute)
	// Emulate a controller still holding the QUEUED snapshot from before activation.
	_, err = f.repo.SetJobStatus(f.ctx, j.ID, domain.JobFailed, "stale queue expiration", f.now)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := f.repo.GetServer(f.ctx, "s")
	if f.get(j.ID).Status != domain.JobStarting || s.GPUs[0].State != domain.GPUReserved || f.get(j.ID).Assignment.ReservationState == "RELEASED" {
		t.Fatal("stale queue snapshot released a dispatched execution")
	}
	f.cycle()
	if f.get(j.ID).Status != domain.JobStopping {
		t.Fatal("next cycle did not request safe STOP")
	}
}
