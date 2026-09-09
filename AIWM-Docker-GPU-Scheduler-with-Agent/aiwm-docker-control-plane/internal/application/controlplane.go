package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/capability"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/policy"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/ports"
)

type ControlPlane struct {
	policy            policy.Evaluator
	catalog           capability.Resolver
	limits            RequestLimits
	repository        ports.Repository
	scheduler         Scheduler
	enrollmentToken   string
	heartbeatInterval time.Duration
	offlineAfter      time.Duration
	commandLease      time.Duration
	now               func() time.Time
}

type Options struct {
	// PlacementPolicy optionally replaces the configured scorer at composition time.
	PlacementPolicy   SchedulingPolicy
	Policy            policy.Evaluator
	Catalog           capability.Resolver
	RequestLimits     RequestLimits
	EnrollmentToken   string
	HeartbeatInterval time.Duration
	OfflineAfter      time.Duration
	CommandLease      time.Duration
	DefaultStrategy   domain.SchedulingStrategy
}

func New(repository ports.Repository, options Options) *ControlPlane {
	if options.Policy == nil {
		options.Policy = policy.New(policy.DevelopmentFacts{QuotaGPUs: 4})
	}
	if options.Catalog == nil {
		options.Catalog = capability.Default()
	}
	if options.RequestLimits.MaxGPUCount == 0 {
		options.RequestLimits.MaxGPUCount = DefaultRequestLimits().MaxGPUCount
	}
	if options.RequestLimits.MaxTTLSeconds == 0 {
		options.RequestLimits.MaxTTLSeconds = DefaultRequestLimits().MaxTTLSeconds
	}
	if options.DefaultStrategy == "" {
		options.DefaultStrategy = domain.StrategyBestFit
	}
	scheduler := NewScheduler(options.DefaultStrategy, options.OfflineAfter)
	if options.PlacementPolicy != nil {
		scheduler = scheduler.WithPolicy(options.DefaultStrategy, options.PlacementPolicy)
	}
	return &ControlPlane{policy: options.Policy, catalog: options.Catalog, limits: options.RequestLimits,
		repository:        repository,
		scheduler:         scheduler,
		enrollmentToken:   options.EnrollmentToken,
		heartbeatInterval: options.HeartbeatInterval,
		offlineAfter:      options.OfflineAfter,
		commandLease:      options.CommandLease,
		now:               time.Now,
	}
}

func (c *ControlPlane) RegisterAgent(ctx context.Context, enrollmentToken string, request agentv1.RegisterRequest) (agentv1.RegisterResponse, error) {
	if enrollmentToken == "" || subtle.ConstantTimeCompare([]byte(enrollmentToken), []byte(c.enrollmentToken)) != 1 {
		return agentv1.RegisterResponse{}, domain.ErrUnauthorized
	}
	if request.ProtocolVersion != agentv1.ProtocolVersion {
		return agentv1.RegisterResponse{}, fmt.Errorf("%w: unsupported agent protocol %q; want %q", domain.ErrInvalidInput, request.ProtocolVersion, agentv1.ProtocolVersion)
	}
	if strings.TrimSpace(request.MachineID) == "" || strings.TrimSpace(request.Name) == "" {
		return agentv1.RegisterResponse{}, fmt.Errorf("%w: machineId and name are required", domain.ErrInvalidInput)
	}
	agentID, err := newID("srv")
	if err != nil {
		return agentv1.RegisterResponse{}, err
	}
	token, err := randomToken(32)
	if err != nil {
		return agentv1.RegisterResponse{}, err
	}
	now := c.now().UTC()
	server, err := c.repository.UpsertServer(ctx, domain.Server{
		ID: agentID, MachineID: request.MachineID, Name: request.Name, Address: request.Address,
		AgentVersion: request.AgentVersion, Labels: request.Labels, Status: domain.ServerOnline,
		LastHeartbeatAt: now, TokenHash: sha256.Sum256([]byte(token)),
	})
	if err != nil {
		return agentv1.RegisterResponse{}, err
	}
	return agentv1.RegisterResponse{
		AgentID: server.ID, AgentToken: token,
		HeartbeatIntervalSeconds: int(c.heartbeatInterval.Seconds()),
	}, nil
}

func (c *ControlPlane) AuthenticateAgent(ctx context.Context, agentID, token string) error {
	return c.repository.AuthenticateAgent(ctx, agentID, token)
}

func (c *ControlPlane) Heartbeat(ctx context.Context, agentID string, request agentv1.HeartbeatRequest) (domain.Server, error) {
	// Connectivity freshness uses server receipt time, never the agent clock.
	return c.repository.Heartbeat(ctx, agentID, c.now().UTC())
}

func (c *ControlPlane) ReportInventory(ctx context.Context, agentID string, report agentv1.InventoryReport) (domain.Server, error) {
	if report.Sequence == 0 {
		return domain.Server{}, fmt.Errorf("%w: inventory sequence must be positive", domain.ErrInvalidInput)
	}
	if report.ObservedAt.IsZero() {
		report.ObservedAt = c.now().UTC()
	}
	domainReport, err := inventoryToDomain(report)
	if err != nil {
		return domain.Server{}, err
	}
	domainReport.ReceivedAt = c.now().UTC()
	return c.repository.ReplaceInventory(ctx, agentID, domainReport)
}

func (c *ControlPlane) ListServers(ctx context.Context) ([]domain.Server, error) {
	servers, err := c.repository.ListServers(ctx)
	for i := range servers {
		servers[i] = c.presentServer(servers[i])
	}
	return servers, err
}

func (c *ControlPlane) GetServer(ctx context.Context, id string) (domain.Server, error) {
	server, err := c.repository.GetServer(ctx, id)
	return c.presentServer(server), err
}

func (c *ControlPlane) SetServerDrained(ctx context.Context, id string, drained bool) (domain.Server, error) {
	server, err := c.repository.SetServerDrained(ctx, id, drained)
	return c.presentServer(server), err
}

// presentServer explains readiness without discarding last-known inventory on a stale host.
func (c *ControlPlane) presentServer(server domain.Server) domain.Server {
	server.SchedulingReady = server.Schedulable(c.now(), c.offlineAfter)
	switch {
	case server.Status == domain.ServerOffline || c.now().Sub(server.LastHeartbeatAt) > c.offlineAfter:
		server.SchedulingReason = "agent offline"
	case server.Drained:
		server.SchedulingReason = "server drained"
	case server.InventoryReceivedAt.IsZero() || c.now().Sub(server.InventoryReceivedAt) > c.offlineAfter:
		server.SchedulingReason = "inventory missing or stale"
	default:
		server.SchedulingReason = "agent online with fresh inventory"
	}
	return server
}

// CreateJob refreshes policy and resources, then joins the shared queue.
// Scheduling re-evaluates all contenders before atomic assignment; submit cannot jump the queue.
func (c *ControlPlane) CreateJob(ctx context.Context, request domain.CreateJobRequest) (domain.Job, error) {
	job, err := c.prepareJob(ctx, request)
	if err != nil {
		return domain.Job{}, err
	}
	match, err := c.matchJob(ctx, job)
	if err != nil {
		return domain.Job{}, err
	}
	job.ID, err = newID("job")
	if err != nil {
		return domain.Job{}, err
	}
	job.StatusReason = match.RecommendationText
	job.Events = []domain.JobEvent{{JobID: job.ID, Status: domain.JobQueued, Reason: job.StatusReason, At: job.CreatedAt}}
	if err := c.repository.CreateJob(ctx, job); err != nil {
		return domain.Job{}, err
	}
	return job, nil
}

func (c *ControlPlane) GetJob(ctx context.Context, id string) (domain.Job, error) {
	return c.repository.GetJob(ctx, id)
}

func (c *ControlPlane) ListJobs(ctx context.Context) ([]domain.Job, error) {
	return c.repository.ListJobs(ctx)
}

func (c *ControlPlane) Queue(ctx context.Context) ([]domain.Job, error) {
	jobs, err := c.repository.ListQueuedJobs(ctx)
	if err != nil {
		return nil, err
	}
	return c.policy.Order(ctx, jobs, c.now().UTC())
}

func (c *ControlPlane) ScheduleOnce(ctx context.Context) (int, error) {
	jobs, err := c.Queue(ctx)
	if err != nil {
		return 0, err
	}
	servers, err := c.repository.ListServers(ctx)
	if err != nil {
		return 0, err
	}
	assigned := 0
	for _, job := range jobs {
		placement, planErr := c.scheduler.Plan(job, servers, c.now().UTC())
		if errors.Is(planErr, domain.ErrInsufficientGPU) {
			_, _ = c.repository.SetJobStatus(
				ctx,
				job.ID,
				domain.JobQueued,
				planErr.Error(),
				c.now().UTC(),
			)
			continue
		}
		if planErr != nil {
			return assigned, planErr
		}
		payload, err := json.Marshal(agentv1.StartContainerPayload{
			JobID: job.ID, Name: "aiwm-" + job.ID, Image: job.Image, Command: job.Command,
			Environment: job.Environment, GPUUUIDs: placement.GPUUUIDs,
			CPUMilli: job.Resources.CPUMilli, MemoryMiB: job.Resources.MemoryMiB,
			Labels: map[string]string{"aiwm.managed": "true", "aiwm.job-id": job.ID},
		})
		if err != nil {
			return assigned, err
		}
		commandID, err := newID("cmd")
		if err != nil {
			return assigned, err
		}
		command := domain.Command{
			ID: commandID, AgentID: placement.ServerID, Type: domain.CommandStartContainer,
			Status: domain.CommandPending, Payload: payload, CreatedAt: c.now().UTC(),
		}
		placement.Policy = job.Policy
		if _, err := c.repository.CommitAssignment(ctx, job.ID, placement, command, c.now().UTC()); err != nil {
			if errors.Is(err, domain.ErrConflict) {
				servers, _ = c.repository.ListServers(ctx)
				continue
			}
			return assigned, err
		}
		assigned++
		servers, _ = c.repository.ListServers(ctx)
	}
	return assigned, nil
}

func (c *ControlPlane) StopJob(ctx context.Context, id string) (domain.Job, error) {
	payload, err := json.Marshal(agentv1.StopContainerPayload{JobID: id, GraceSeconds: 30})
	if err != nil {
		return domain.Job{}, err
	}
	commandID, err := newID("cmd")
	if err != nil {
		return domain.Job{}, err
	}
	return c.repository.RequestStop(ctx, id, domain.Command{ID: commandID, Type: domain.CommandStopContainer, Status: domain.CommandPending, Payload: payload, CreatedAt: c.now().UTC()}, c.now().UTC())
}

func (c *ControlPlane) PollCommands(ctx context.Context, agentID string, limit int) ([]agentv1.Command, error) {
	if limit > 100 {
		limit = 100
	}
	commands, err := c.repository.LeaseCommands(ctx, agentID, c.now().UTC(), c.commandLease, limit)
	if err != nil {
		return nil, err
	}
	result := make([]agentv1.Command, 0, len(commands))
	for _, command := range commands {
		result = append(result, agentv1.Command{ID: command.ID, Type: string(command.Type), Payload: append([]byte(nil), command.Payload...)})
	}
	return result, nil
}

func (c *ControlPlane) AckCommand(ctx context.Context, agentID, commandID string, ack agentv1.CommandAckRequest) (domain.Command, error) {
	return c.repository.AckCommand(ctx, agentID, commandID, domain.CommandAckRequest{
		Succeeded: ack.Succeeded, Message: ack.Message, ContainerID: ack.ContainerID,
	}, c.now().UTC())
}

func (c *ControlPlane) Reconcile(ctx context.Context) (int, error) {
	return c.repository.MarkStaleServers(ctx, c.now().UTC().Add(-c.offlineAfter))
}

func (c *ControlPlane) Summary(ctx context.Context) (domain.ClusterSummary, error) {
	servers, err := c.repository.ListServers(ctx)
	if err != nil {
		return domain.ClusterSummary{}, err
	}
	jobs, err := c.repository.ListJobs(ctx)
	if err != nil {
		return domain.ClusterSummary{}, err
	}
	var summary domain.ClusterSummary
	summary.ServersTotal = len(servers)
	for _, server := range servers {
		if server.Status != domain.ServerOffline && c.now().Sub(server.LastHeartbeatAt) <= c.offlineAfter {
			summary.ServersOnline++
		}
		for _, gpu := range server.GPUs {
			summary.GPUsTotal++
			if gpu.State != domain.GPUFree {
				summary.GPUsOccupied++
			}
			switch gpu.State {
			case domain.GPUFree:
				if server.Schedulable(c.now(), c.offlineAfter) && gpu.Schedulable() {
					summary.GPUsFree++
				}
			case domain.GPUOccupiedLegacy:
				summary.GPUsLegacy++
			case domain.GPUOccupiedUnknown:
				summary.GPUsUnknown++
			}
		}
	}
	for _, job := range jobs {
		switch job.Status {
		case domain.JobQueued:
			summary.JobsQueued++
		case domain.JobRunning:
			summary.JobsRunning++
		}
	}
	summary.RecentEvents = make([]domain.JobEvent, 0)
	for _, job := range jobs {
		summary.RecentEvents = append(summary.RecentEvents, job.Events...)
	}
	sort.Slice(summary.RecentEvents, func(i, j int) bool { return summary.RecentEvents[i].At.After(summary.RecentEvents[j].At) })
	if len(summary.RecentEvents) > 20 {
		summary.RecentEvents = summary.RecentEvents[:20]
	}
	return summary, nil
}

func (c *ControlPlane) GPUs(ctx context.Context) ([]map[string]any, error) {
	servers, err := c.repository.ListServers(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0)
	for _, server := range servers {
		for _, gpu := range server.GPUs {
			result = append(result, map[string]any{"serverId": server.ID, "serverName": server.Name, "serverStatus": server.Status, "gpu": gpu})
		}
	}
	return result, nil
}

func (c *ControlPlane) Containers(ctx context.Context, origin domain.ContainerOrigin) ([]map[string]any, error) {
	servers, err := c.repository.ListServers(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0)
	for _, server := range servers {
		for _, container := range server.Containers {
			if origin != "" && container.Origin != origin {
				continue
			}
			result = append(result, map[string]any{"serverId": server.ID, "serverName": server.Name, "container": container})
		}
	}
	sort.Slice(result, func(i, j int) bool { return fmt.Sprint(result[i]["serverName"]) < fmt.Sprint(result[j]["serverName"]) })
	return result, nil
}

func randomToken(bytesCount int) (string, error) {
	buffer := make([]byte, bytesCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func newID(prefix string) (string, error) {
	token, err := randomToken(8)
	if err != nil {
		return "", err
	}
	return prefix + "_" + token, nil
}
