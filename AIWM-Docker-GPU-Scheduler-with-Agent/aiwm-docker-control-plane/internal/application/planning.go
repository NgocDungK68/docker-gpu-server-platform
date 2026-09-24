package application

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

const waitingWindow = "Không đủ GPU an toàn trong khoảng thời gian yêu cầu"

// planReservations is deterministic greedy planning; policy and placement remain replaceable.
// It runs on a repository snapshot and must never call back into the runtime repository.
func (c *ControlPlane) planReservations(ctx context.Context, jobs []domain.Job, servers []domain.Server, enabled map[string]bool, now time.Time) ([]domain.ReservationPlan, error) {
	candidates := []domain.Job{}
	ids := map[string]bool{}
	for _, job := range scopedJobs(ctx, jobs) {
		if job.Replannable(now) {
			candidates = append(candidates, job)
			ids[job.ID] = true
		}
	}
	ordered, err := c.policy.Order(ctx, candidates, now)
	if err != nil {
		return nil, err
	}
	bookings := []domain.Job{}
	for _, job := range jobs {
		if !ids[job.ID] {
			bookings = append(bookings, job)
		}
	}
	result := make([]domain.ReservationPlan, 0, len(ordered))
	for _, job := range ordered {
		entry := domain.ReservationPlan{JobID: job.ID, Policy: job.Policy, Reason: waitingWindow}
		window := job.RequestedWindow()
		switch {
		case !window.Valid():
			entry.Reason = "Yêu cầu cũ thiếu khoảng thời gian hợp lệ; cần tạo workload mới"
		case !now.Before(window.EndAt):
			entry.Reason = "Khoảng thời gian yêu cầu đã kết thúc"
		case c.metadata != nil && !enabled[job.OrganizationID]:
			entry.Reason = "Đơn vị không được phép cấp phát"
		default:
			placement, err := c.scheduler.Plan(job, domain.CalendarServers(job, servers, bookings, now, c.offlineAfter), now)
			if err == nil {
				placement.Policy = job.Policy
				entry.Placement = &placement
				entry.Reason = "Đã giữ tài nguyên theo lịch; chờ đến thời gian chạy"
				job.Assignment = &domain.Assignment{ServerID: placement.ServerID, GPUUUIDs: placement.GPUUUIDs, StartAt: window.StartAt, EndAt: window.EndAt, ReservationState: "PLANNED"}
				bookings = append(bookings, job)
			} else if !errors.Is(err, domain.ErrInsufficientGPU) {
				return nil, err
			}
		}
		result = append(result, entry)
	}
	return result, nil
}

// processReservations is the execution controller, shared by scheduling and reconciliation tickers.
func (c *ControlPlane) processReservations(ctx context.Context) error {
	jobs, err := c.repository.ListJobs(ctx)
	if err != nil {
		return err
	}
	enabled, err := c.enabledOrganizations(ctx)
	if err != nil {
		return err
	}
	jobs = scopedJobs(ctx, jobs)
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	for _, job := range jobs {
		var recoverErr error
		job, recoverErr = c.recoverTrainingUpload(ctx, job, c.now().UTC())
		if recoverErr != nil {
			return recoverErr
		}
		if job.Status.Terminal() {
			continue
		}
		now := c.now().UTC()
		window := job.RequestedWindow()
		if job.Assignment != nil {
			window = job.Assignment.Window()
		}
		if !window.Valid() {
			continue
		} // Legacy in-flight executions are not silently reinterpreted.
		if !now.Before(window.EndAt) {
			if job.Status == domain.JobQueued {
				_, err = c.repository.SetJobStatus(ctx, job.ID, domain.JobFailed, "Khoảng thời gian yêu cầu đã kết thúc mà chưa được cấp GPU", now)
			} else {
				_, err = c.StopJob(ctx, job.ID)
			}
			if err != nil {
				return err
			}
			continue
		}
		if err := c.requestCheckpoint(ctx, job, now); err != nil {
			return err
		}
		if job.Status != domain.JobAssigned || job.Assignment == nil || job.Assignment.ReservationState != "PLANNED" || !window.Contains(now) {
			continue
		}
		if c.metadata != nil && !enabled[job.OrganizationID] {
			continue
		}
		a := job.Assignment
		payload, err := json.Marshal(agentv1.StartContainerPayload{
			JobID: job.ID, Name: "aiwm-" + job.ID, Image: job.Image, Command: job.Command, Environment: c.trainingEnvironment(job), GPUUUIDs: a.GPUUUIDs,
			CPUMilli: job.Resources.CPUMilli, MemoryMiB: job.Resources.MemoryMiB, Labels: map[string]string{"aiwm.managed": "true", "aiwm.job-id": job.ID},
		})
		if err != nil {
			return err
		}
		id, err := newID("cmd")
		if err != nil {
			return err
		}
		p := domain.Placement{ServerID: a.ServerID, GPUUUIDs: a.GPUUUIDs, Strategy: a.Strategy, Score: a.Score, Reason: a.Reason, Policy: job.Policy}
		_, err = c.repository.CommitAssignment(ctx, job.ID, p, domain.Command{ID: id, AgentID: a.ServerID, Type: domain.CommandStartContainer, Status: domain.CommandPending, Payload: payload, CreatedAt: now}, now)
		if errors.Is(err, domain.ErrConflict) {
			// The repository preserves the reservation until a safe retry or its end.
			if _, noteErr := c.repository.SetJobStatus(ctx, job.ID, domain.JobAssigned, "Chờ server/GPU an toàn và inventory mới trước khi chạy", now); noteErr != nil {
				return noteErr
			}
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}
