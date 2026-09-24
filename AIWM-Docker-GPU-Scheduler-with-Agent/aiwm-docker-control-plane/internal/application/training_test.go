package application

import (
	"context"
	"encoding/json"
	"errors"
	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

type trainingObjects struct {
	objects map[string]string
	fail    bool
	before  func()
}

func (s *trainingObjects) Put(ctx context.Context, key string, r io.Reader, size int64) (string, error) {
	if s.before != nil {
		s.before()
	}
	if s.fail {
		return "", errors.New("storage unavailable")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if int64(len(data)) != size {
		return "", errors.New("short upload")
	}
	if s.objects == nil {
		s.objects = map[string]string{}
	}
	uri := "s3://training/" + key
	s.objects[uri] = string(data)
	return uri, nil
}
func (s *trainingObjects) DownloadURL(_ context.Context, uri string, _ time.Duration) (string, error) {
	if _, ok := s.objects[uri]; !ok {
		return "", errors.New("missing object")
	}
	return "https://signed.invalid/download", nil
}
func trainingFixture(t *testing.T) (*planningFixture, *trainingObjects, domain.Job, domain.Container) {
	f := newPlanningFixture(t)
	objects := &trainingObjects{}
	f.cp.objects = objects
	f.cp.training = DefaultTrainingOptions()
	f.cp.training.APIURL = "http://control-plane:8080"
	j := f.create(f.now, time.Hour, domain.Necessity2, domain.ImportanceImportant)
	f.cycle()
	commands, err := f.repo.LeaseCommands(f.ctx, "s", f.now, time.Second, 10)
	if err != nil || len(commands) != 1 {
		t.Fatalf("START lease: %v", err)
	}
	var payload agentv1.StartContainerPayload
	if err = json.Unmarshal(commands[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Environment["AIWM_TRAINING_TOKEN"] == "" || payload.Environment["AIWM_ALLOCATION_END_AT"] != j.RequestedWindow().EndAt.Format(time.RFC3339Nano) {
		t.Fatal("missing training contract")
	}
	_, err = f.repo.AckCommand(f.ctx, "s", commands[0].ID, domain.CommandAckRequest{Succeeded: true, ContainerID: "managed"}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	container := domain.Container{ID: "managed", JobID: j.ID, Origin: domain.ContainerManaged, State: "running", GPUUUIDs: []string{"GPU-a"}}
	f.inventory([]domain.Container{container})
	return f, objects, f.get(j.ID), container
}

func TestTrainingWarningClamp(t *testing.T) {
	o := DefaultTrainingOptions()
	start := time.Now()
	for _, tc := range []struct{ duration, lead time.Duration }{{time.Hour, 6 * time.Minute}, {8 * time.Hour, 30 * time.Minute}, {72 * time.Hour, 30 * time.Minute}, {time.Minute, time.Minute}} {
		w := domain.TimeWindow{StartAt: start, EndAt: start.Add(tc.duration)}
		if got := w.EndAt.Sub(o.WarningAt(w)); got != tc.lead {
			t.Fatalf("%s: %s", tc.duration, got)
		}
	}
}
func TestTrainingTimeLimitResumeOnDifferentServer(t *testing.T) {
	f, objects, j, container := trainingFixture(t)
	f.now = j.Training.CheckpointWarningAt.Add(-time.Nanosecond)
	f.inventory([]domain.Container{container})
	f.cycle()
	if f.get(j.ID).Training.CheckpointStatus != domain.CheckpointNone || f.get(j.ID).Status != domain.JobRunning {
		t.Fatal("early warning or stop")
	}
	f.now = j.Training.CheckpointWarningAt
	f.inventory([]domain.Container{container})
	f.cycle()
	if f.get(j.ID).Training.CheckpointStatus != domain.CheckpointRequested || f.get(j.ID).Status != domain.JobRunning {
		t.Fatal("warning did not request checkpoint")
	}
	saved, err := f.cp.SaveTrainingOutput(f.ctx, j.ID, j.TrainingToken, "checkpoint", 7, 4, strings.NewReader("step"))
	if err != nil || saved.CheckpointStatus != domain.CheckpointAvailable {
		t.Fatalf("checkpoint: %v", err)
	}
	f.cycle()
	if f.get(j.ID).Training.CheckpointStatus != domain.CheckpointAvailable {
		t.Fatal("requested a recent checkpoint again")
	}
	f.now = j.RequestedWindow().EndAt.Add(-time.Nanosecond)
	f.inventory([]domain.Container{container})
	f.cycle()
	if f.get(j.ID).Status != domain.JobRunning {
		t.Fatal("early TIME_LIMIT")
	}
	f.now = j.RequestedWindow().EndAt
	f.inventory([]domain.Container{container})
	f.cycle()
	stopped := f.get(j.ID)
	if stopped.Status != domain.JobStopping || stopped.TerminationReason != domain.TerminationTimeLimit || stopped.Assignment.ReservationState == "RELEASED" {
		t.Fatal("bad expiry/early release")
	}
	code := 0
	container.State = "exited"
	container.ExitCode = &code
	f.inventory([]domain.Container{container})
	stopped = f.get(j.ID)
	if !stopped.Resumable() || stopped.Status != domain.JobStopped || stopped.Assignment.ReservationState != "RELEASED" {
		t.Fatal("not resumable after actual stop")
	}
	server, _ := f.repo.GetServer(f.ctx, "s")
	if server.GPUs[0].State != domain.GPUFree {
		t.Fatal("GPU not released")
	}
	// Original server disappears; continuation must not require its filesystem.
	_, err = f.repo.SetServerDrained(f.ctx, "s", true)
	if err != nil {
		t.Fatal(err)
	}
	server.ID, server.MachineID = "s2", "m2"
	server.GPUs[0].UUID = "GPU-b"
	server.Drained = false
	_, err = f.repo.UpsertServer(f.ctx, server)
	if err != nil {
		t.Fatal(err)
	}
	next, err := f.cp.ContinueTraining(f.ctx, j.ID, ContinueTrainingRequest{NeededAt: f.now.Format(time.RFC3339Nano), TTLSeconds: 3600})
	if err != nil {
		t.Fatal(err)
	}
	f.cycle()
	next = f.get(next.ID)
	if next.ID == j.ID || next.Assignment == nil || next.Assignment.ServerID != "s2" || next.Training.ResumeCheckpointURI != saved.LatestCheckpointURI {
		t.Fatal("bad continuation placement/reference")
	}
	env := f.cp.trainingEnvironment(next)
	if env["AIWM_RESUME_CHECKPOINT_URI"] != saved.LatestCheckpointURI || objects.objects[saved.LatestCheckpointURI] != "step" {
		t.Fatal("checkpoint lost on another server")
	}
	if _, err = f.cp.TrainingSession(f.ctx, next.ID, j.TrainingToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("source token accepted for continuation")
	}
}

func TestTrainingPublicationAndCompletionAreIndependent(t *testing.T) {
	f, objects, j, container := trainingFixture(t)
	f.now = j.Training.CheckpointWarningAt.Add(-time.Second)
	objects.before = func() {
		current := f.get(j.ID)
		if current.Training.ArtifactStatus != domain.ArtifactSaving || current.Training.FinalArtifactURI != "" {
			t.Fatal("READY before PUT")
		}
		f.now = j.Training.CheckpointWarningAt
		f.inventory([]domain.Container{container})
		f.cycle() // warning must not steal an upload revision
	}
	objects.fail = true
	if _, err := f.cp.SaveTrainingOutput(f.ctx, j.ID, j.TrainingToken, "artifact", 0, 3, strings.NewReader("bad")); err == nil {
		t.Fatal("failed upload accepted")
	}
	if f.get(j.ID).Training.ArtifactStatus != domain.ArtifactFailed {
		t.Fatal("missing upload failure")
	}
	objects.before = nil
	code := 0
	container.State = "exited"
	container.ExitCode = &code
	f.inventory([]domain.Container{container})
	current := f.get(j.ID)
	if current.Status != domain.JobSucceeded || current.TerminationReason != domain.TerminationCompleted || current.Training.ArtifactStatus == domain.ArtifactReady {
		t.Fatal("success falsely implies artifact READY")
	}
	objects.fail = false
	saved, err := f.cp.SaveTrainingOutput(f.ctx, j.ID, j.TrainingToken, "artifact", 0, 5, strings.NewReader("model"))
	if err != nil || saved.ArtifactStatus != domain.ArtifactReady {
		t.Fatalf("artifact: %v", err)
	}
	if _, err = f.cp.ArtifactDownload(f.ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	other := domain.WithPrincipal(context.Background(), domain.Principal{User: domain.User{Role: domain.RoleOrganizationUser, OrganizationID: "other"}})
	if _, err = f.cp.ArtifactDownload(other, j.ID); err == nil {
		t.Fatal("cross-org download allowed")
	}
	f.now = f.now.Add(31 * time.Second)
	if _, err = f.cp.TrainingSession(f.ctx, j.ID, j.TrainingToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("terminal token remained valid")
	}
}
func TestTrainingFailedReplacementPreservesCheckpoint(t *testing.T) {
	f, objects, j, _ := trainingFixture(t)
	a, err := f.cp.SaveTrainingOutput(f.ctx, j.ID, j.TrainingToken, "checkpoint", 1, 3, strings.NewReader("old"))
	if err != nil {
		t.Fatal(err)
	}
	objects.fail = true
	b, err := f.cp.SaveTrainingOutput(f.ctx, j.ID, j.TrainingToken, "checkpoint", 2, 3, strings.NewReader("new"))
	if err == nil || b.CheckpointStatus != domain.CheckpointAvailable || b.LatestCheckpointURI != a.LatestCheckpointURI || b.CheckpointStep != 1 {
		t.Fatal("lost prior checkpoint")
	}
	if _, err = f.cp.SaveTrainingOutput(f.ctx, j.ID, "wrong", "checkpoint", 1, 3, strings.NewReader("bad")); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("bad token accepted")
	}
}

func TestTrainingExpiredUploadLeaseRetainsCheckpoint(t *testing.T) {
	f, _, j, _ := trainingFixture(t)
	saved, err := f.cp.SaveTrainingOutput(f.ctx, j.ID, j.TrainingToken, "checkpoint", 3, 3, strings.NewReader("old"))
	if err != nil {
		t.Fatal(err)
	}
	pending := saved
	pending.CheckpointStatus = domain.CheckpointSaving
	pending.UploadID = "crashed"
	pending.UploadKind = "checkpoint"
	pending.UploadStartedAt = f.now
	job, err := f.repo.UpdateTraining(f.ctx, j.ID, saved.Revision, pending, f.now)
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(f.cp.training.UploadTimeout)
	recovered, err := f.cp.recoverTrainingUpload(f.ctx, job, f.now)
	if err != nil || recovered.Training.UploadID != "" || recovered.Training.CheckpointStatus != domain.CheckpointAvailable || recovered.Training.LatestCheckpointURI != saved.LatestCheckpointURI {
		t.Fatal("upload recovery lost prior output")
	}
	if _, err = f.repo.UpdateTraining(f.ctx, j.ID, job.Training.Revision, pending, f.now); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("late upload replaced recovered state")
	}
}
func TestTrainingUserCancellationBoundsPublication(t *testing.T) {
	f, objects, j, _ := trainingFixture(t)
	objects.before = func() {
		if _, err := f.cp.StopJob(f.ctx, j.ID); err != nil {
			t.Fatal(err)
		}
		f.now = f.now.Add(31 * time.Second)
	}
	if _, err := f.cp.SaveTrainingOutput(f.ctx, j.ID, j.TrainingToken, "artifact", 0, 3, strings.NewReader("old")); err == nil {
		t.Fatal("published beyond STOP grace")
	}
	job := f.get(j.ID)
	if job.Training.ArtifactStatus != domain.ArtifactFailed || job.Training.FinalArtifactURI != "" || job.TerminationReason != domain.TerminationUserCancelled {
		t.Fatal("incorrect cancellation/output state")
	}
}
