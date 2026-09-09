package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

type RunnerConfig struct {
	MachineID         string
	Name              string
	AgentVersion      string
	Labels            map[string]string
	EnrollmentToken   string
	HeartbeatEvery    time.Duration
	InventoryEvery    time.Duration
	CommandPollEvery  time.Duration
	CommandLimit      int
	MaxCommandHistory int
}

type Runner struct {
	config    RunnerConfig
	client    ControlPlaneClient
	inventory InventorySource
	executor  *CommandExecutor
	state     StateStore
	docker    DockerRuntime
	logger    *slog.Logger

	mu          sync.Mutex
	persistent  PersistentState
	registerMu  sync.Mutex
	inventoryMu sync.Mutex
}

func NewRunner(config RunnerConfig, client ControlPlaneClient, inventory InventorySource, executor *CommandExecutor, state StateStore, docker DockerRuntime, logger *slog.Logger) *Runner {
	if config.CommandLimit <= 0 {
		config.CommandLimit = 10
	}
	if config.MaxCommandHistory <= 0 {
		config.MaxCommandHistory = 1000
	}
	return &Runner{config: config, client: client, inventory: inventory, executor: executor, state: state, docker: docker, logger: logger}
}

func (r *Runner) Run(ctx context.Context) error {
	loaded, err := r.state.Load()
	if err != nil {
		return fmt.Errorf("load agent state: %w", err)
	}
	if loaded.MachineID != "" && loaded.MachineID != r.config.MachineID {
		return fmt.Errorf("state belongs to machine %q, current machine is %q", loaded.MachineID, r.config.MachineID)
	}
	if loaded.ProcessedCommands == nil {
		loaded.ProcessedCommands = make(map[string]CommandResult)
	}
	loaded.MachineID = r.config.MachineID
	r.mu.Lock()
	r.persistent = loaded
	r.mu.Unlock()

	for {
		err := r.docker.Ping(ctx)
		if err == nil {
			err = r.ensureRegistered(ctx, "")
		}
		if err == nil {
			err = r.reportInventory(ctx)
		}
		if err == nil {
			break
		}
		r.logger.Warn("agent startup awaiting Docker/control-plane/inventory", "error", err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(r.config.HeartbeatEvery):
		}
	}

	r.logger.Info("AIWM agent started", "agent_id", r.identity().AgentID, "machine_id", r.config.MachineID)
	var workers sync.WaitGroup
	workers.Add(3)
	go func() { defer workers.Done(); r.heartbeatLoop(ctx) }()
	go func() { defer workers.Done(); r.inventoryLoop(ctx) }()
	go func() { defer workers.Done(); r.commandLoop(ctx) }()
	<-ctx.Done()
	workers.Wait()
	return nil
}

func (r *Runner) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(r.config.HeartbeatEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			identity := r.identity()
			err := r.client.Heartbeat(ctx, identity.AgentID, identity.AgentToken, agentv1.HeartbeatRequest{ObservedAt: time.Now().UTC()})
			if errors.Is(err, ErrUnauthorized) {
				err = r.ensureRegistered(ctx, identity.AgentID)
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				r.logger.Warn("heartbeat failed", "error", err)
			}
		}
	}
}

func (r *Runner) inventoryLoop(ctx context.Context) {
	ticker := time.NewTicker(r.config.InventoryEvery)
	defer ticker.Stop()
	events, eventErrors := r.docker.WatchContainerEvents(ctx)
	var debounce <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.tryInventory(ctx)
		case _, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			// Collapse Docker event bursts (create/start/die/destroy) into one
			// inventory refresh instead of hammering Docker Engine.
			debounce = time.After(500 * time.Millisecond)
		case <-debounce:
			debounce = nil
			r.tryInventory(ctx)
		case err, ok := <-eventErrors:
			if ok && err != nil && !errors.Is(err, context.Canceled) {
				r.logger.Warn("Docker event stream stopped; periodic inventory remains active", "error", err)
			}
			eventErrors = nil
		}
	}
}

func (r *Runner) commandLoop(ctx context.Context) {
	ticker := time.NewTicker(r.config.CommandPollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.pollAndExecute(ctx); err != nil && !errors.Is(err, context.Canceled) {
				r.logger.Warn("command poll failed", "error", err)
			}
		}
	}
}

func (r *Runner) tryInventory(ctx context.Context) {
	if err := r.reportInventory(ctx); err != nil && !errors.Is(err, context.Canceled) {
		r.logger.Warn("inventory report failed; previous Control Plane state is retained", "error", err)
	}
}

func (r *Runner) reportInventory(ctx context.Context) error {
	r.inventoryMu.Lock()
	defer r.inventoryMu.Unlock()
	r.mu.Lock()
	r.persistent.InventorySequence++
	sequence := r.persistent.InventorySequence
	if err := r.saveLocked(); err != nil {
		r.persistent.InventorySequence--
		r.mu.Unlock()
		return err
	}
	r.mu.Unlock()

	report, err := r.inventory.Snapshot(ctx, sequence)
	if err != nil {
		return err
	}
	identity := r.identity()
	err = r.client.ReportInventory(ctx, identity.AgentID, identity.AgentToken, report)
	if errors.Is(err, ErrUnauthorized) {
		if registerErr := r.ensureRegistered(ctx, identity.AgentID); registerErr != nil {
			return registerErr
		}
		identity = r.identity()
		return r.client.ReportInventory(ctx, identity.AgentID, identity.AgentToken, report)
	}
	return err
}

func (r *Runner) pollAndExecute(ctx context.Context) error {
	identity := r.identity()
	commands, err := r.client.PollCommands(ctx, identity.AgentID, identity.AgentToken, r.config.CommandLimit)
	if errors.Is(err, ErrUnauthorized) {
		return r.ensureRegistered(ctx, identity.AgentID)
	}
	if err != nil {
		return err
	}
	for _, command := range commands {
		ack, processed := r.processed(command.ID)
		if !processed {
			ack = r.executor.Execute(ctx, command)
			if err := r.remember(command.ID, ack); err != nil {
				return fmt.Errorf("persist command result before ACK: %w", err)
			}
		}
		identity = r.identity()
		if err := r.client.AckCommand(ctx, identity.AgentID, identity.AgentToken, command.ID, ack); err != nil {
			return err
		}
		r.logger.Info("agent command completed", "command_id", command.ID, "type", command.Type, "succeeded", ack.Succeeded)
		// Publish the resulting actual state immediately. Periodic and
		// Docker-event inventory remain safety nets if this request fails.
		if err := r.reportInventory(ctx); err != nil {
			r.logger.Warn("post-command inventory failed", "command_id", command.ID, "error", err)
		}
	}
	return nil
}

func (r *Runner) ensureRegistered(ctx context.Context, failedAgentID string) error {
	r.registerMu.Lock()
	defer r.registerMu.Unlock()
	current := r.identity()
	if current.AgentID != "" && failedAgentID != "" && current.AgentID != failedAgentID {
		return nil
	}
	if current.AgentID != "" && failedAgentID == "" {
		return nil
	}
	response, err := r.client.Register(ctx, agentv1.RegisterRequest{
		ProtocolVersion: agentv1.ProtocolVersion, MachineID: r.config.MachineID, Name: r.config.Name,
		AgentVersion: r.config.AgentVersion, Labels: cloneMap(r.config.Labels),
		Capabilities: []string{"docker-inventory", "nvml-inventory", "managed-container-v1"},
	}, r.config.EnrollmentToken)
	if err != nil {
		return fmt.Errorf("register agent: %w", err)
	}
	r.mu.Lock()
	r.persistent.AgentID = response.AgentID
	r.persistent.AgentToken = response.AgentToken
	err = r.saveLocked()
	r.mu.Unlock()
	if err != nil {
		return fmt.Errorf("persist agent identity: %w", err)
	}
	r.logger.Info("agent registered", "agent_id", response.AgentID, "protocol", agentv1.ProtocolVersion)
	return nil
}

func (r *Runner) identity() PersistentState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return PersistentState{AgentID: r.persistent.AgentID, AgentToken: r.persistent.AgentToken}
}

func (r *Runner) processed(commandID string) (agentv1.CommandAckRequest, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result, ok := r.persistent.ProcessedCommands[commandID]
	return result.Ack, ok
}

func (r *Runner) remember(commandID string, ack agentv1.CommandAckRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.persistent.ProcessedCommands[commandID] = CommandResult{Ack: ack, CompletedAt: time.Now().UTC()}
	for len(r.persistent.ProcessedCommands) > r.config.MaxCommandHistory {
		oldestID := ""
		var oldest time.Time
		for id, result := range r.persistent.ProcessedCommands {
			if oldestID == "" || result.CompletedAt.Before(oldest) {
				oldestID, oldest = id, result.CompletedAt
			}
		}
		delete(r.persistent.ProcessedCommands, oldestID)
	}
	return r.saveLocked()
}

func (r *Runner) saveLocked() error {
	return r.state.Save(r.persistent)
}
