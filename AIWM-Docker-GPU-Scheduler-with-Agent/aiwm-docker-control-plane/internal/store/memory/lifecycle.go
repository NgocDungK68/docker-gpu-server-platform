package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

// RequestStop atomically cancels queued/unleased jobs or appends one managed stop command.
func (s *Store) RequestStop(_ context.Context, jobID string, command domain.Command, at time.Time) (domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return domain.Job{}, domain.ErrNotFound
	}
	if job.Status.Terminal() || job.Status == domain.JobStopping {
		return cloneJob(job), nil
	}
	if job.Status == domain.JobQueued {
		s.transitionLocked(&job, domain.JobCancelled, "cancelled before placement", at)
	} else if job.Assignment != nil {
		start := s.commands[job.Assignment.CommandID]
		if job.Assignment.CommandID == "" {
			s.transitionLocked(&job, domain.JobCancelled, "Đã hủy lịch trước khi thực thi", at)
			s.releaseLocked(&job, at)
		} else if start.Status == domain.CommandPending {
			start.Status = domain.CommandFailed
			start.Error = "cancelled before dispatch"
			start.CompletedAt = timePtr(at)
			s.commands[start.ID] = start
			s.transitionLocked(&job, domain.JobCancelled, "cancelled before command delivery", at)
			s.releaseLocked(&job, at)
		} else {
			if _, exists := s.commands[command.ID]; exists {
				return domain.Job{}, domain.ErrConflict
			}
			command.AgentID = job.Assignment.ServerID
			s.commands[command.ID] = cloneCommand(command)
			s.transitionLocked(&job, domain.JobStopping, "stop requested; awaiting agent and observed state", at)
		}
	} else {
		return domain.Job{}, domain.ErrJobNotStoppable
	}
	s.jobs[job.ID] = cloneJob(job)
	return cloneJob(job), nil
}

// transitionLocked records meaningful lifecycle changes while preserving terminal states.
func (s *Store) transitionLocked(job *domain.Job, status domain.JobStatus, reason string, at time.Time) {
	if job.Status.Terminal() || (job.Status == status && job.StatusReason == reason) {
		return
	}
	job.Status, job.StatusReason, job.UpdatedAt = status, reason, at
	job.Events = append(job.Events, domain.JobEvent{JobID: job.ID, Status: status, Reason: reason, At: at})
}

// releaseLocked releases reservation in the same transaction as terminal job state.
// Actual active Docker consumers still protect their GPUs until fresh inventory says otherwise.
func (s *Store) releaseLocked(job *domain.Job, at time.Time) {
	if job.Assignment == nil {
		return
	}
	job.Assignment.ReservationState = "RELEASED"
	job.Assignment.ReleasedAt = timePtr(at)
	s.jobs[job.ID] = cloneJob(*job)
	server, ok := s.servers[job.Assignment.ServerID]
	if !ok {
		return
	}
	server.GPUs = normalizeGPUState(server.ID, server.GPUs, server.Containers, nil, s.jobs)
	s.servers[server.ID] = cloneServer(server)
}

// applyAckLocked applies the first ACK only, atomically with its job transition.
func (s *Store) applyAckLocked(command domain.Command, ack domain.CommandAckRequest, at time.Time) {
	var payload struct{ JobID string }
	if json.Unmarshal(command.Payload, &payload) != nil {
		return
	}
	job, ok := s.jobs[payload.JobID]
	if !ok || job.Status.Terminal() || job.Assignment == nil || job.Assignment.ServerID != command.AgentID {
		return
	}
	switch command.Type {
	case domain.CommandStartContainer:
		if ack.Succeeded {
			job.ContainerID = ack.ContainerID
			if job.Status == domain.JobAssigned {
				s.transitionLocked(&job, domain.JobStarting, "start acknowledged; awaiting observed container", at)
			}
		} else if job.Status == domain.JobAssigned || job.Status == domain.JobStarting || job.Status == domain.JobStopping {
			s.transitionLocked(&job, domain.JobFailed, "agent could not launch managed container", at)
			s.releaseLocked(&job, at)
		}
	case domain.CommandStopContainer:
		if !ack.Succeeded && job.Status == domain.JobStopping {
			s.transitionLocked(&job, domain.JobRunning, "agent could not stop managed container", at)
		}
	}
	s.jobs[job.ID] = cloneJob(job)
}

// reconcileObservedLocked treats complete inventory as actual state, never as permission to adopt.
func (s *Store) reconcileObservedLocked(serverID string, report domain.InventoryReport) {
	byJob := make(map[string]domain.Container)
	for _, c := range report.Containers {
		if c.Origin == domain.ContainerManaged && c.JobID != "" {
			previous, ok := byJob[c.JobID]
			if !ok || (!containerConsumesGPU(previous.State) && containerConsumesGPU(c.State)) {
				byJob[c.JobID] = c
			}
		}
	}
	for id, job := range s.jobs {
		if job.Assignment == nil || job.Assignment.ServerID != serverID || job.Status.Terminal() {
			continue
		}
		// A future reservation does not authorize adopting a container from inventory.
		if job.Assignment.CommandID == "" {
			continue
		}
		start := s.commands[job.Assignment.CommandID]
		observed, exists := byJob[id]
		if exists {
			job.ContainerID = observed.ID
			job.LastObservedAt = timePtr(report.ObservedAt)
			switch observed.State {
			case "running", "paused", "restarting":
				job.Assignment.ReservationState = "ALLOCATED"
				if job.Status == domain.JobAssigned || job.Status == domain.JobStarting {
					s.transitionLocked(&job, domain.JobRunning, "managed container observed active", report.ReceivedAt)
				}
			case "exited", "dead":
				status := domain.JobFailed
				reason := "managed container terminated without a successful exit"
				if job.Status == domain.JobStopping {
					status, reason = domain.JobStopped, "managed container observed stopped"
				} else if observed.ExitCode != nil && *observed.ExitCode == 0 {
					status, reason = domain.JobSucceeded, "managed container exited successfully"
				} else if observed.ExitCode != nil {
					reason = fmt.Sprintf("managed container exited with code %d", *observed.ExitCode)
				}
				s.transitionLocked(&job, status, reason, report.ReceivedAt)
				s.releaseLocked(&job, report.ReceivedAt)
			}
		} else if (job.Status == domain.JobRunning || job.Status == domain.JobStopping || job.Status == domain.JobStarting) &&
			((start.Status == domain.CommandSucceeded && start.CompletedAt != nil && !report.ObservedAt.Before(*start.CompletedAt)) || s.stopConfirmedLocked(job, report.ObservedAt)) {
			status, reason := domain.JobFailed, "managed container missing from complete agent inventory"
			if job.Status == domain.JobStopping {
				status, reason = domain.JobStopped, "managed container is absent after stop request"
			}
			s.transitionLocked(&job, status, reason, report.ReceivedAt)
			s.releaseLocked(&job, report.ReceivedAt)
		}
		s.jobs[id] = cloneJob(job)
	}
}

// stopConfirmedLocked also resolves an expired START with a lost ACK, after the
// serial Agent executor acknowledged STOP and a later inventory confirms absence.
func (s *Store) stopConfirmedLocked(job domain.Job, observedAt time.Time) bool {
	if job.Status != domain.JobStopping || job.Assignment == nil {
		return false
	}
	for _, command := range s.commands {
		if command.AgentID != job.Assignment.ServerID || command.Type != domain.CommandStopContainer || command.Status != domain.CommandSucceeded || command.CompletedAt == nil || observedAt.Before(*command.CompletedAt) {
			continue
		}
		var payload struct{ JobID string }
		if json.Unmarshal(command.Payload, &payload) == nil && payload.JobID == job.ID {
			return true
		}
	}
	return false
}
