package memory

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/ports"
)

// ReplanReservations atomically replaces only queued/future, undispatched reservations.
func (s *Store) ReplanReservations(_ context.Context, at time.Time, plan ports.ReservationPlanner) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := make([]domain.Job, 0, len(s.jobs))
	servers := make([]domain.Server, 0, len(s.servers))
	for _, job := range s.jobs {
		jobs = append(jobs, cloneJob(job))
	}
	for _, server := range s.servers {
		servers = append(servers, cloneServer(server))
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	sort.Slice(servers, func(i, j int) bool { return servers[i].ID < servers[j].ID })
	plans, err := plan(jobs, servers)
	if err != nil {
		return 0, err
	}
	changed := map[string]domain.Job{}
	for _, entry := range plans {
		job, ok := s.jobs[entry.JobID]
		if !ok || !job.Replannable(at) {
			return 0, domain.ErrConflict
		}
		if _, duplicate := changed[job.ID]; duplicate {
			return 0, domain.ErrConflict
		}
		job = cloneJob(job)
		job.Assignment = nil
		changed[job.ID] = job
	}
	bookings := make([]domain.Job, 0, len(jobs))
	for _, job := range jobs {
		if _, replaced := changed[job.ID]; !replaced {
			bookings = append(bookings, job)
		}
	}
	count := 0
	for _, entry := range plans {
		job := changed[entry.JobID]
		job.Policy = entry.Policy
		if entry.Placement == nil {
			s.transitionLocked(&job, domain.JobQueued, entry.Reason, at)
		} else {
			window := job.RequestedWindow()
			if !window.Valid() || !at.Before(window.EndAt) {
				return 0, domain.ErrConflict
			}
			p := entry.Placement
			valid := false
			for _, server := range domain.CalendarServers(job, servers, bookings, at, s.offlineAfter) {
				if server.ID == p.ServerID && server.Schedulable(at, s.offlineAfter) && placementMatches(job, server, *p) {
					valid = true
					break
				}
			}
			if !valid {
				return 0, domain.ErrConflict
			}
			job.Assignment = &domain.Assignment{ServerID: p.ServerID, GPUUUIDs: append([]string(nil), p.GPUUUIDs...),
				StartAt: window.StartAt, EndAt: window.EndAt, AssignedAt: at, ReservationState: "PLANNED", Strategy: p.Strategy, Score: p.Score, Reason: entry.Reason}
			old := s.jobs[job.ID].Assignment
			if old != nil && old.ServerID == p.ServerID && reflect.DeepEqual(old.GPUUUIDs, p.GPUUUIDs) {
				job.Assignment.AssignedAt = old.AssignedAt
			} else {
				count++
			}
			s.transitionLocked(&job, domain.JobAssigned, entry.Reason, at)
			bookings = append(bookings, job)
		}
		changed[job.ID] = job
	}
	// No mutation was made before every placement was revalidated under the same lock.
	for id, job := range changed {
		s.jobs[id] = cloneJob(job)
	}
	return count, nil
}

func placementMatches(job domain.Job, server domain.Server, p domain.Placement) bool {
	if !domain.SameOrganization(job.OrganizationID, server.OrganizationID) || job.Resources.GPUCount <= 0 || len(p.GPUUUIDs) != job.Resources.GPUCount {
		return false
	}
	wanted := map[string]bool{}
	for _, uuid := range p.GPUUUIDs {
		if uuid == "" || wanted[uuid] {
			return false
		}
		wanted[uuid] = true
	}
	found := 0
	for _, gpu := range server.GPUs {
		if wanted[gpu.UUID] {
			if !job.Resources.Matches(gpu) {
				return false
			}
			found++
		}
	}
	for key, value := range job.ServerSelector {
		if server.Labels[key] != value {
			return false
		}
	}
	return found == len(wanted)
}

// startDeliverableLocked checks again at polling time; a delayed poll cannot start an expired booking.
func (s *Store) startDeliverableLocked(command domain.Command, now time.Time) bool {
	server := s.servers[command.AgentID]
	if !server.Schedulable(now, s.offlineAfter) {
		return false
	}
	var payload struct{ JobID string }
	if json.Unmarshal(command.Payload, &payload) != nil {
		return false
	}
	job, ok := s.jobs[payload.JobID]
	if !ok || job.Status.Terminal() || job.Assignment == nil || job.Assignment.CommandID != command.ID || !domain.SameOrganization(job.OrganizationID, server.OrganizationID) {
		return false
	}
	window := job.Assignment.Window()
	if window.Valid() && !window.Contains(now) {
		return false
	}
	found := 0
	for _, gpu := range server.GPUs {
		if !containsUUID(job.Assignment.GPUUUIDs, gpu.UUID) {
			continue
		}
		if !gpu.Healthy || gpu.AssignedJobID != job.ID || (gpu.State != domain.GPUReserved && gpu.State != domain.GPUAllocated) {
			return false
		}
		found++
	}
	return found == len(job.Assignment.GPUUUIDs)
}
