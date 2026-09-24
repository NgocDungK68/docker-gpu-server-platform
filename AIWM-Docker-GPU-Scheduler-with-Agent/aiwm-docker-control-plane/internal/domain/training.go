package domain

import "time"

type CheckpointStatus string

const (
	CheckpointNone      CheckpointStatus = "NONE"
	CheckpointRequested CheckpointStatus = "REQUESTED"
	CheckpointSaving    CheckpointStatus = "SAVING"
	CheckpointAvailable CheckpointStatus = "AVAILABLE"
	CheckpointFailed    CheckpointStatus = "FAILED"
)

type ArtifactStatus string

const (
	ArtifactNone   ArtifactStatus = "NONE"
	ArtifactSaving ArtifactStatus = "SAVING"
	ArtifactReady  ArtifactStatus = "READY"
	ArtifactFailed ArtifactStatus = "FAILED"
)

type TerminationReason string

const (
	TerminationCompleted      TerminationReason = "COMPLETED"
	TerminationTimeLimit      TerminationReason = "TIME_LIMIT"
	TerminationUserCancelled  TerminationReason = "USER_CANCELLED"
	TerminationExecutionError TerminationReason = "EXECUTION_ERROR"
	TerminationSystemError    TerminationReason = "SYSTEM_ERROR"
)

// TrainingState stores references and publication state, never model bytes.
type TrainingState struct {
	CheckpointStatus    CheckpointStatus `json:"checkpointStatus"`
	LatestCheckpointURI string           `json:"latestCheckpointURI,omitempty"`
	CheckpointCreatedAt time.Time        `json:"checkpointCreatedAt,omitempty"`
	CheckpointStep      int64            `json:"checkpointStep,omitempty"`
	CheckpointWarningAt time.Time        `json:"checkpointWarningAt,omitempty"`
	ArtifactStatus      ArtifactStatus   `json:"artifactStatus"`
	FinalArtifactURI    string           `json:"finalArtifactURI,omitempty"`
	ArtifactCreatedAt   time.Time        `json:"artifactCreatedAt,omitempty"`
	ResumeCheckpointURI string           `json:"resumeCheckpointURI,omitempty"`
	ResumeFromJobID     string           `json:"resumeFromJobId,omitempty"`
	// Private publication lease prevents late uploads from overwriting newer metadata.
	Revision        uint64    `json:"-"`
	UploadID        string    `json:"-"`
	UploadKind      string    `json:"-"`
	UploadStartedAt time.Time `json:"-"`
}

func (j Job) Resumable() bool {
	return j.WorkloadType == WorkloadTraining && j.Status == JobStopped &&
		j.TerminationReason == TerminationTimeLimit && j.Training.CheckpointStatus == CheckpointAvailable && j.Training.LatestCheckpointURI != ""
}
