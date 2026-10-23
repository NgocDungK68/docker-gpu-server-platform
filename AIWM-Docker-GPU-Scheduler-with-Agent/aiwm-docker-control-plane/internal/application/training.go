package application

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

type TrainingOptions struct {
	APIURL                         string
	WarningFraction                float64
	MinWarningLead, MaxWarningLead time.Duration
	UploadTimeout                  time.Duration
	MaxUploadBytes                 int64
}

func (o TrainingOptions) withDefaults() TrainingOptions {
	d := DefaultTrainingOptions()
	if o.WarningFraction <= 0 {
		o.WarningFraction = d.WarningFraction
	}
	if o.MinWarningLead <= 0 {
		o.MinWarningLead = d.MinWarningLead
	}
	if o.MaxWarningLead <= 0 {
		o.MaxWarningLead = d.MaxWarningLead
	}
	if o.UploadTimeout <= 0 {
		o.UploadTimeout = d.UploadTimeout
	}
	if o.MaxUploadBytes <= 0 {
		o.MaxUploadBytes = d.MaxUploadBytes
	}
	return o
}

func (c *ControlPlane) TrainingUploadLimits() (int64, time.Duration) {
	return c.training.MaxUploadBytes, c.training.UploadTimeout
}

// The STOP grace is bounded by lifecycle events, not mutable metadata timestamps.
func trainingDeadline(job domain.Job) time.Time {
	deadline := job.RequestedWindow().EndAt.Add(30 * time.Second)
	for _, e := range job.Events {
		if e.Status == domain.JobStopping || e.Status.Terminal() {
			if t := e.At.Add(30 * time.Second); t.Before(deadline) {
				deadline = t
			}
		}
	}
	return deadline
}

func DefaultTrainingOptions() TrainingOptions {
	return TrainingOptions{WarningFraction: .1, MinWarningLead: 5 * time.Minute, MaxWarningLead: 30 * time.Minute, UploadTimeout: 15 * time.Minute, MaxUploadBytes: 5 << 30}
}

// WarningAt never precedes the requested start for short allocations.
func (o TrainingOptions) WarningAt(w domain.TimeWindow) time.Time {
	lead := time.Duration(float64(w.EndAt.Sub(w.StartAt)) * o.WarningFraction)
	if lead < o.MinWarningLead {
		lead = o.MinWarningLead
	}
	if lead > o.MaxWarningLead {
		lead = o.MaxWarningLead
	}
	at := w.EndAt.Add(-lead)
	if at.Before(w.StartAt) {
		at = w.StartAt
	}
	return at
}

func (c *ControlPlane) initTraining(job *domain.Job) error {
	job.Training.CheckpointStatus, job.Training.ArtifactStatus = domain.CheckpointNone, domain.ArtifactNone
	if job.WorkloadType != domain.WorkloadTraining || c.objects == nil {
		return nil
	}
	var err error
	job.TrainingToken, err = randomToken(32)
	job.Training.CheckpointWarningAt = c.training.WarningAt(job.RequestedWindow())
	return err
}

// trainingEnvironment injects only a per-job credential, never object storage keys.
func (c *ControlPlane) trainingEnvironment(job domain.Job) map[string]string {
	env := make(map[string]string, len(job.Environment)+7)
	for k, v := range job.Environment {
		env[k] = v
	}
	if job.WorkloadType == domain.WorkloadTraining && job.TrainingToken != "" && c.objects != nil {
		env["AIWM_JOB_ID"] = job.ID
		env["AIWM_TRAINING_URL"] = strings.TrimRight(c.training.APIURL, "/") + "/api/v1/training/" + url.PathEscape(job.ID)
		env["AIWM_TRAINING_TOKEN"] = job.TrainingToken
		env["AIWM_ALLOCATION_END_AT"] = job.RequestedWindow().EndAt.Format(time.RFC3339Nano)
		env["AIWM_CHECKPOINT_URI"] = job.Training.LatestCheckpointURI
		env["AIWM_ARTIFACT_URI"] = job.Training.FinalArtifactURI
		env["AIWM_RESUME_CHECKPOINT_URI"] = job.Training.ResumeCheckpointURI
	}
	return env
}

func (c *ControlPlane) requestCheckpoint(ctx context.Context, job domain.Job, now time.Time) error {
	t := job.Training
	if c.objects == nil || job.WorkloadType != domain.WorkloadTraining || job.Status != domain.JobRunning ||
		(t.UploadID != "" && now.Sub(t.UploadStartedAt) < c.training.UploadTimeout) || t.CheckpointWarningAt.IsZero() || now.Before(t.CheckpointWarningAt) || !now.Before(job.RequestedWindow().EndAt) ||
		t.CheckpointStatus == domain.CheckpointSaving || t.CheckpointStatus == domain.CheckpointRequested ||
		!t.CheckpointCreatedAt.Before(t.CheckpointWarningAt) {
		return nil
	}
	t.CheckpointStatus = domain.CheckpointRequested
	_, err := c.repository.UpdateTraining(ctx, job.ID, t.Revision, t, now)
	if errors.Is(err, domain.ErrConflict) {
		return nil
	}
	return err
}

// recoverTrainingUpload clears a crashed/expired upload lease without losing the prior checkpoint.
func (c *ControlPlane) recoverTrainingUpload(ctx context.Context, job domain.Job, now time.Time) (domain.Job, error) {
	t := job.Training
	if t.UploadID == "" || (now.Before(t.UploadStartedAt.Add(c.training.UploadTimeout)) && now.Before(trainingDeadline(job))) {
		return job, nil
	}
	if t.UploadKind == "checkpoint" {
		t.CheckpointStatus = domain.CheckpointFailed
		if t.LatestCheckpointURI != "" {
			t.CheckpointStatus = domain.CheckpointAvailable
		}
	} else {
		t.ArtifactStatus = domain.ArtifactFailed
	}
	t.UploadID, t.UploadKind = "", ""
	updated, err := c.repository.UpdateTraining(ctx, job.ID, t.Revision, t, now)
	if errors.Is(err, domain.ErrConflict) {
		return job, nil
	}
	return updated, err
}

// TrainingSession is authenticated only by the credential delivered to this Job.
// Publication may finish within the existing STOP grace, never extending GPU allocation.
func (c *ControlPlane) TrainingSession(ctx context.Context, id, token string) (domain.Job, error) {
	job, err := c.repository.GetJob(ctx, id)
	if err != nil || c.objects == nil || token == "" || job.TrainingToken == "" ||
		subtle.ConstantTimeCompare([]byte(token), []byte(job.TrainingToken)) != 1 {
		return domain.Job{}, domain.ErrUnauthorized
	}
	now := c.now().UTC()
	w := job.RequestedWindow()
	if !w.Valid() || now.Before(w.StartAt) || !now.Before(trainingDeadline(job)) ||
		job.Assignment == nil || job.Assignment.CommandID == "" {
		return domain.Job{}, domain.ErrUnauthorized
	}
	return job, nil
}

type TrainingContract struct {
	StopRequested       bool                 `json:"stopRequested"`
	JobID               string               `json:"jobId"`
	EndAt               time.Time            `json:"endAt"`
	CheckpointRequested bool                 `json:"checkpointRequested"`
	Training            domain.TrainingState `json:"training"`
	ResumeURL           string               `json:"resumeURL,omitempty"`
}

func (c *ControlPlane) TrainingContract(ctx context.Context, id, token string) (TrainingContract, error) {
	job, err := c.TrainingSession(ctx, id, token)
	if err != nil {
		return TrainingContract{}, err
	}
	result := TrainingContract{StopRequested: job.Status == domain.JobStopping, JobID: job.ID, EndAt: job.RequestedWindow().EndAt, Training: job.Training, CheckpointRequested: job.Training.CheckpointStatus == domain.CheckpointRequested}
	if job.Training.ResumeCheckpointURI != "" {
		result.ResumeURL, err = c.objects.DownloadURL(ctx, job.Training.ResumeCheckpointURI, 5*time.Minute)
	}
	return result, err
}

// SaveTrainingOutput streams one application-owned archive; successful Put is the
// publication boundary. A unique key and revision lease preserve prior checkpoints.
func (c *ControlPlane) SaveTrainingOutput(ctx context.Context, id, token, kind string, step, size int64, body io.Reader) (domain.TrainingState, error) {
	job, err := c.TrainingSession(ctx, id, token)
	if err != nil {
		return domain.TrainingState{}, err
	}
	if (kind != "checkpoint" && kind != "artifact") || step < 0 || size <= 0 || size > c.training.MaxUploadBytes {
		return domain.TrainingState{}, fmt.Errorf("%w: output kind, step or size", domain.ErrInvalidInput)
	}
	now := c.now().UTC()
	t := job.Training
	if t.UploadID != "" && now.Sub(t.UploadStartedAt) < c.training.UploadTimeout {
		return t, domain.ErrConflict
	}
	if kind == "artifact" && t.ArtifactStatus == domain.ArtifactReady {
		return t, domain.ErrConflict
	}
	uploadID, err := newID("output")
	if err != nil {
		return t, err
	}
	t.UploadID, t.UploadKind, t.UploadStartedAt = uploadID, kind, now
	if kind == "checkpoint" {
		t.CheckpointStatus = domain.CheckpointSaving
	} else {
		t.ArtifactStatus = domain.ArtifactSaving
	}
	saving, err := c.repository.UpdateTraining(ctx, id, t.Revision, t, now)
	if err != nil {
		return t, err
	}
	t = saving.Training
	key := url.PathEscape(job.OrganizationID) + "/jobs/" + job.ID + "/" + kind + "s/" + uploadID + ".bin"
	// STOP at EndAt is independent of this stream; no scheduler lock is held.
	deadline := now.Add(c.training.UploadTimeout)
	if end := trainingDeadline(job); end.Before(deadline) {
		deadline = end
	}
	uploadCtx, cancel := context.WithTimeout(ctx, deadline.Sub(now))
	defer cancel()
	uri, putErr := c.objects.Put(uploadCtx, key, body, size)
	at := c.now().UTC()
	if latest, e := c.repository.GetJob(ctx, id); e != nil {
		putErr = fmt.Errorf("cannot revalidate publication")
	} else if !at.Before(trainingDeadline(latest)) {
		putErr = fmt.Errorf("publication grace ended")
	}
	if putErr == nil && (uri == "" || uploadCtx.Err() != nil || !at.Before(deadline)) {
		putErr = fmt.Errorf("upload deadline exceeded")
	}
	if putErr == nil {
		if kind == "checkpoint" {
			t.CheckpointStatus, t.LatestCheckpointURI, t.CheckpointCreatedAt, t.CheckpointStep = domain.CheckpointAvailable, uri, at, step
		} else {
			t.ArtifactStatus, t.FinalArtifactURI, t.ArtifactCreatedAt = domain.ArtifactReady, uri, at
		}
	} else if kind == "checkpoint" {
		t.CheckpointStatus = domain.CheckpointFailed
		if t.LatestCheckpointURI != "" {
			t.CheckpointStatus = domain.CheckpointAvailable
		}
	} else {
		t.ArtifactStatus = domain.ArtifactFailed
	}
	t.UploadID, t.UploadKind = "", ""
	// Persist failure even if the client disconnected. A failed commit leaves a
	// bounded lease; retry after timeout creates a fresh key without losing old output.
	commitCtx, commitCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer commitCancel()
	stored, commitErr := c.repository.UpdateTraining(commitCtx, id, t.Revision, t, at)
	if commitErr != nil {
		return domain.TrainingState{}, commitErr
	}
	if putErr != nil {
		return stored.Training, fmt.Errorf("không lưu được tệp đầu ra")
	}
	return stored.Training, nil
}

func (c *ControlPlane) ArtifactDownload(ctx context.Context, id string) (string, error) {
	job, err := c.GetJob(ctx, id)
	if err != nil {
		return "", err
	}
	if c.objects == nil || job.Training.ArtifactStatus != domain.ArtifactReady || job.Training.FinalArtifactURI == "" {
		return "", domain.ErrConflict
	}
	return c.objects.DownloadURL(ctx, job.Training.FinalArtifactURI, 5*time.Minute)
}

type ContinueTrainingRequest struct {
	NeededAt   string `json:"neededAt"`
	TTLSeconds int64  `json:"ttlSeconds"`
}

func (c *ControlPlane) ContinueTraining(ctx context.Context, id string, r ContinueTrainingRequest) (domain.Job, error) {
	source, err := c.GetJob(ctx, id)
	if err != nil {
		return domain.Job{}, err
	}
	if !source.Resumable() || c.objects == nil {
		return domain.Job{}, domain.ErrConflict
	}
	intent := source.AllocationIntent
	intent.NeededAt, intent.TTLSeconds = r.NeededAt, r.TTLSeconds
	fp8 := source.Resources.FP8Required
	return c.CreateJob(ctx, domain.CreateJobRequest{AllocationIntent: intent, Name: source.Name, Image: source.Image, Backend: source.Backend, Command: source.Command, Environment: source.Environment,
		Resources: domain.AllocationResources{GPUCount: source.Resources.GPUCount, MinVRAMMiB: source.Resources.MinVRAMMiB, PerformanceProfile: source.Resources.PerformanceProfile, FP8Required: &fp8, CPUMilli: source.Resources.CPUMilli, MemoryMiB: source.Resources.MemoryMiB}, ResumeFromJobID: source.ID})
}
