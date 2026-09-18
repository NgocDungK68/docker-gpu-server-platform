package memory

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

type Store struct {
	mu           sync.RWMutex
	servers      map[string]domain.Server
	machines     map[string]string
	jobs         map[string]domain.Job
	commands     map[string]domain.Command
	offlineAfter time.Duration
}

func New(timeouts ...time.Duration) *Store {
	timeout := time.Minute
	if len(timeouts) > 0 {
		timeout = timeouts[0]
	}
	return &Store{
		offlineAfter: timeout,
		servers:      make(map[string]domain.Server),
		machines:     make(map[string]string),
		jobs:         make(map[string]domain.Job),
		commands:     make(map[string]domain.Command),
	}
}

func (s *Store) UpsertServer(_ context.Context, incoming domain.Server) (domain.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, ok := s.machines[incoming.MachineID]; ok {
		existing := s.servers[id]
		if existing.OrganizationID != incoming.OrganizationID || (incoming.OrganizationID!="" && incoming.ID!=existing.ID) {
			return domain.Server{},domain.ErrConflict
		}
		existing.Name = incoming.Name
		existing.Address = incoming.Address
		existing.AgentVersion = incoming.AgentVersion
		existing.Labels = cloneMap(incoming.Labels)
		existing.Status = domain.ServerOnline
		existing.InventoryReceivedAt = time.Time{}
		existing.InventoryVersion = 0
		if existing.Drained {
			existing.Status = domain.ServerDraining
		}
		existing.LastHeartbeatAt = incoming.LastHeartbeatAt
		existing.TokenHash = incoming.TokenHash
		s.servers[id] = cloneServer(existing)
		return cloneServer(existing), nil
	}

	s.machines[incoming.MachineID] = incoming.ID
	s.servers[incoming.ID] = cloneServer(incoming)
	return cloneServer(incoming), nil
}

func (s *Store) AuthenticateAgent(_ context.Context, agentID, token string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	server, ok := s.servers[agentID]
	if !ok {
		return domain.ErrUnauthorized
	}
	want := server.TokenHash
	got := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(want[:], got[:]) != 1 {
		return domain.ErrUnauthorized
	}
	return nil
}

func (s *Store) Heartbeat(_ context.Context, agentID string, at time.Time) (domain.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	server, ok := s.servers[agentID]
	if !ok {
		return domain.Server{}, domain.ErrNotFound
	}
	if server.Status==domain.ServerOffline || at.Sub(server.LastHeartbeatAt)>s.offlineAfter {
		server.InventoryReceivedAt=time.Time{}
	}
	server.LastHeartbeatAt = at
	if server.Drained {
		server.Status = domain.ServerDraining
	} else {
		server.Status = domain.ServerOnline
	}
	s.servers[agentID] = cloneServer(server)
	return cloneServer(server), nil
}

func (s *Store) ReplaceInventory(_ context.Context, agentID string, report domain.InventoryReport) (domain.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	server, ok := s.servers[agentID]
	if !ok {
		return domain.Server{}, domain.ErrNotFound
	}
	if report.Sequence <= server.InventoryVersion {
		return domain.Server{}, domain.ErrStaleInventory
	}
	for otherID, other := range s.servers {
		if otherID == agentID {
			continue
		}
		for _, gpu := range report.GPUs {
			for _, known := range other.GPUs {
				if gpu.UUID == known.UUID {
					return domain.Server{}, domain.ErrConflict
				}
			}
		}
	}

	server.InventoryVersion = report.Sequence
	server.LastInventoryAt = report.ObservedAt
	if report.ReceivedAt.IsZero() {
		report.ReceivedAt = report.ObservedAt
	}
	server.LastHeartbeatAt = report.ReceivedAt
	server.InventoryReceivedAt = report.ReceivedAt
	server.DockerVersion = report.DockerVersion
	server.Host = report.Host
	s.reconcileObservedLocked(agentID, report)
	server.GPUs = normalizeGPUState(agentID, report.GPUs, report.Containers, report.Processes, s.jobs)
	server.Containers = cloneContainers(report.Containers)
	if server.Drained {
		server.Status = domain.ServerDraining
	} else {
		server.Status = domain.ServerOnline
	}
	s.servers[agentID] = cloneServer(server)

	return cloneServer(server), nil
}

func (s *Store) SetServerDrained(_ context.Context, serverID string, drained bool) (domain.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	server, ok := s.servers[serverID]
	if !ok {
		return domain.Server{}, domain.ErrNotFound
	}
	server.Drained = drained
	if server.Status == domain.ServerOffline {
		// Preserve connectivity independently from drain configuration.
	} else if drained {
		server.Status = domain.ServerDraining
	} else {
		server.Status = domain.ServerOnline
	}
	s.servers[serverID] = cloneServer(server)
	return cloneServer(server), nil
}

func (s *Store) MarkStaleServers(_ context.Context, staleBefore time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := 0
	for id, server := range s.servers {
		if server.LastHeartbeatAt.Before(staleBefore) && server.Status != domain.ServerOffline {
			server.Status = domain.ServerOffline
			s.servers[id] = cloneServer(server)
			changed++
		}
	}
	return changed, nil
}

func (s *Store) GetServer(_ context.Context, id string) (domain.Server, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	server, ok := s.servers[id]
	if !ok {
		return domain.Server{}, domain.ErrNotFound
	}
	return cloneServer(server), nil
}

func (s *Store) ListServers(_ context.Context) ([]domain.Server, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Server, 0, len(s.servers))
	for _, server := range s.servers {
		result = append(result, cloneServer(server))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s *Store) CreateJob(_ context.Context, job domain.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return domain.ErrConflict
	}
	s.jobs[job.ID] = cloneJob(job)
	return nil
}

func (s *Store) GetJob(_ context.Context, id string) (domain.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return domain.Job{}, domain.ErrNotFound
	}
	return cloneJob(job), nil
}

func (s *Store) ListJobs(_ context.Context) ([]domain.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		result = append(result, cloneJob(job))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (s *Store) ListQueuedJobs(_ context.Context) ([]domain.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]domain.Job, 0)
	for _, job := range s.jobs {
		if job.Status == domain.JobQueued {
			result = append(result, cloneJob(job))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.Before(result[j].CreatedAt)
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func (s *Store) SetJobStatus(_ context.Context, id string, status domain.JobStatus, reason string, at time.Time) (domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return domain.Job{}, domain.ErrNotFound
	}
	if job.Status.Terminal() || (status == domain.JobQueued && job.Status != domain.JobQueued) {
		return cloneJob(job), nil
	}
	s.transitionLocked(&job, status, reason, at)
	if status.Terminal() {
		s.releaseLocked(&job, at)
	}
	s.jobs[id] = cloneJob(job)
	return cloneJob(job), nil
}

func (s *Store) CommitAssignment(_ context.Context, jobID string, placement domain.Placement, command domain.Command, at time.Time) (domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[jobID]
	if !ok {
		return domain.Job{}, domain.ErrNotFound
	}
	if job.Status != domain.JobQueued {
		return domain.Job{}, domain.ErrConflict
	}
	server, ok := s.servers[placement.ServerID]
	if !ok {
		return domain.Job{}, domain.ErrNotFound
	}
	if !domain.SameOrganization(job.OrganizationID,server.OrganizationID) { return domain.Job{},domain.ErrConflict }
	if !server.Schedulable(at, s.offlineAfter) || job.Resources.GPUCount <= 0 || len(placement.GPUUUIDs) != job.Resources.GPUCount || command.AgentID != server.ID {
		return domain.Job{}, domain.ErrConflict
	}
	if _, exists := s.commands[command.ID]; exists {
		return domain.Job{}, domain.ErrConflict
	}
	wanted := make(map[string]bool, len(placement.GPUUUIDs))
	for _, uuid := range placement.GPUUUIDs {
		if uuid == "" || wanted[uuid] {
			return domain.Job{}, domain.ErrConflict
		}
		wanted[uuid] = true
	}
	found := 0
	for _, gpu := range server.GPUs {
		if !wanted[gpu.UUID] {
			continue
		}
		if !job.Resources.Matches(gpu) {
			return domain.Job{}, domain.ErrConflict
		}
		found++
	}
	if found != len(wanted) {
		return domain.Job{}, domain.ErrConflict
	}
	for key, value := range job.ServerSelector {
		if server.Labels[key] != value {
			return domain.Job{}, domain.ErrConflict
		}
	}
	// Validate all UUIDs before copying/mutating slices; failures leave the snapshot untouched.
	server = cloneServer(server)
	for i := range server.GPUs {
		if wanted[server.GPUs[i].UUID] {
			server.GPUs[i].State = domain.GPUReserved
			server.GPUs[i].AssignedJobID = jobID
		}
	}

	s.transitionLocked(&job, domain.JobAssigned, placement.Reason, at)
	job.Assignment = &domain.Assignment{
		ServerID: placement.ServerID, GPUUUIDs: append([]string(nil), placement.GPUUUIDs...),
		CommandID: command.ID, AssignedAt: at,
		ReservationState: "RESERVED", Strategy: placement.Strategy, Score: placement.Score, Reason: placement.Reason,
	}
	job.Policy = placement.Policy
	job.UpdatedAt = at
	s.servers[server.ID] = cloneServer(server)
	s.jobs[job.ID] = cloneJob(job)
	s.commands[command.ID] = cloneCommand(command)
	return cloneJob(job), nil
}

func (s *Store) EnqueueCommand(_ context.Context, command domain.Command) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.commands[command.ID]; exists {
		return domain.ErrConflict
	}
	if _, exists := s.servers[command.AgentID]; !exists {
		return domain.ErrNotFound
	}
	s.commands[command.ID] = cloneCommand(command)
	return nil
}

func (s *Store) LeaseCommands(_ context.Context, agentID string, now time.Time, lease time.Duration, limit int) ([]domain.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.servers[agentID]; !ok {
		return nil, domain.ErrNotFound
	}
	if limit <= 0 {
		limit = 10
	}
	eligible := make([]domain.Command, 0)
	for _, command := range s.commands {
		if command.AgentID != agentID {
			continue
		}
		if command.Type==domain.CommandStartContainer && !s.servers[agentID].Schedulable(now,s.offlineAfter) { continue }
		if command.Type == domain.CommandStopContainer {
			var stop struct{ JobID string }
			_ = json.Unmarshal(command.Payload, &stop)
			job := s.jobs[stop.JobID]
			if job.Assignment != nil {
				start := s.commands[job.Assignment.CommandID]
				if start.Status == domain.CommandPending || start.Status == domain.CommandDelivered {
					continue
				}
			}
		}
		if command.Status == domain.CommandPending || (command.Status == domain.CommandDelivered && command.LeaseUntil != nil && command.LeaseUntil.Before(now)) {
			eligible = append(eligible, cloneCommand(command))
		}
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].CreatedAt.Equal(eligible[j].CreatedAt) {
			return eligible[i].ID < eligible[j].ID
		}
		return eligible[i].CreatedAt.Before(eligible[j].CreatedAt)
	})
	if len(eligible) > limit {
		eligible = eligible[:limit]
	}
	leaseUntil := now.Add(lease)
	for i := range eligible {
		command := eligible[i]
		command.Status = domain.CommandDelivered
		command.Attempts++
		command.DeliveredAt = timePtr(now)
		command.LeaseUntil = timePtr(leaseUntil)
		s.commands[command.ID] = cloneCommand(command)
		eligible[i] = cloneCommand(command)
	}
	return eligible, nil
}

func (s *Store) AckCommand(_ context.Context, agentID, commandID string, ack domain.CommandAckRequest, at time.Time) (domain.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	command, ok := s.commands[commandID]
	if !ok || command.AgentID != agentID {
		return domain.Command{}, domain.ErrNotFound
	}
	if command.Status == domain.CommandSucceeded || command.Status == domain.CommandFailed {
		return cloneCommand(command), nil
	}
	command.CompletedAt = timePtr(at)
	command.LeaseUntil = nil
	if ack.Succeeded {
		command.Status = domain.CommandSucceeded
	} else {
		command.Status = domain.CommandFailed
		command.Error = ack.Message
	}
	s.commands[command.ID] = cloneCommand(command)
	s.applyAckLocked(command, ack, at)
	return cloneCommand(command), nil
}

func cloneServer(value domain.Server) domain.Server {
	value.Labels = cloneMap(value.Labels)
	value.GPUs = cloneGPUs(value.GPUs)
	value.Containers = cloneContainers(value.Containers)
	return value
}

func cloneJob(value domain.Job) domain.Job {
	value.Events = append([]domain.JobEvent{}, value.Events...)
	value.Command = append([]string(nil), value.Command...)
	value.Environment = cloneMap(value.Environment)
	value.ServerSelector = cloneMap(value.ServerSelector)
	value.Resources.ResolvedModels = append([]string(nil), value.Resources.ResolvedModels...)
	if value.Assignment != nil {
		assignment := *value.Assignment
		assignment.GPUUUIDs = append([]string(nil), value.Assignment.GPUUUIDs...)
		value.Assignment = &assignment
	}
	return value
}

func cloneCommand(value domain.Command) domain.Command {
	value.Payload = append([]byte(nil), value.Payload...)
	return value
}

func cloneGPUs(values []domain.GPU) []domain.GPU {
	result := append([]domain.GPU{}, values...)
	for i := range result {
		result[i].ObservedConsumers = append([]string(nil), result[i].ObservedConsumers...)
	}
	return result
}

func cloneContainers(values []domain.Container) []domain.Container {
	result := append([]domain.Container{}, values...)
	for i := range result {
		result[i].GPUUUIDs = append([]string(nil), result[i].GPUUUIDs...)
		result[i].Labels = cloneMap(result[i].Labels)
	}
	return result
}

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func timePtr(value time.Time) *time.Time { return &value }
