// Package simulator supplies only a Docker runtime adapter. GPU discovery always uses NVML.
package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

// Runtime persists simulated Docker containers independently of Agent identity/state.
type Runtime struct {
	mu           sync.Mutex
	path         string
	containers   map[string]agent.RuntimeContainer
	events       chan struct{}
	rejectStarts int
}

// NewRuntime seeds external containers before registration from NVML-discovered UUIDs.
func NewRuntime(path string, gpus []agentv1.GPU, external []int, rejectStarts int) (*Runtime, error) {
	r := &Runtime{path: path, containers: map[string]agent.RuntimeContainer{}, events: make(chan struct{}, 1), rejectStarts: rejectStarts}
	data, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(data, &r.containers); err != nil {
			return nil, err
		}
		if r.containers == nil {
			return nil, fmt.Errorf("invalid Docker runtime state")
		}
		return r, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	for _, index := range external {
		uuid := ""
		for _, g := range gpus {
			if g.Index == index {
				uuid = g.UUID
				break
			}
		}
		if uuid == "" {
			return nil, fmt.Errorf("external GPU index %d is absent from NVML profile", index)
		}
		id := fmt.Sprintf("external-%d", index)
		now := time.Now().UTC()
		r.containers[id] = agent.RuntimeContainer{ID: id, Name: "existing-inference-" + id, Image: "external/workload:existing",
			State: "running", GPUUUIDs: []string{uuid}, InitPID: 10000 + index, StartedAt: &now}
	}
	if err := r.saveLocked(); err != nil {
		return nil, err
	}
	return r, nil
}
func (r *Runtime) Ping(context.Context) error              { return nil }
func (r *Runtime) Version(context.Context) (string, error) { return "simulated-docker/nvml-mock", nil }
func (r *Runtime) Close() error                            { return nil }
func (r *Runtime) ListContainers(context.Context) ([]agent.RuntimeContainer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]agent.RuntimeContainer, 0, len(r.containers))
	for _, c := range r.containers {
		c.GPUUUIDs = append([]string(nil), c.GPUUUIDs...)
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// ContainerID correlates deterministic simulated host PIDs with Docker inventory.
func (r *Runtime) ContainerID(pid int) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, c := range r.containers {
		if c.InitPID == pid {
			return id, nil
		}
	}
	return "", fmt.Errorf("unknown PID")
}

// StartManagedContainer mimics idempotent Docker creation while keeping external containers untouched.
func (r *Runtime) StartManagedContainer(_ context.Context, p agentv1.StartContainerPayload) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, c := range r.containers {
		if c.Labels[agentv1.ManagedLabel] == "true" && c.Labels[agentv1.JobIDLabel] == p.JobID {
			return id, nil
		}
	}
	if r.rejectStarts > 0 {
		r.rejectStarts--
		return "", fmt.Errorf("simulated Docker launch failure")
	}
	id := "managed-" + p.JobID
	now := time.Now().UTC()
	r.containers[id] = agent.RuntimeContainer{ID: id, Name: p.Name, Image: p.Image, State: "running", GPUUUIDs: append([]string(nil), p.GPUUUIDs...),
		Labels: map[string]string{agentv1.ManagedLabel: "true", agentv1.JobIDLabel: p.JobID}, StartedAt: &now}
	if err := r.saveLocked(); err != nil {
		delete(r.containers, id)
		return "", err
	}
	r.notify()
	return id, nil
}

// StopManagedJob accepts ownership labels and job identity, never arbitrary external targets.
func (r *Runtime) StopManagedJob(_ context.Context, p agentv1.StopContainerPayload) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, c := range r.containers {
		if p.ContainerID != "" && p.ContainerID != id {
			continue
		}
		if c.Labels[agentv1.ManagedLabel] != "true" || c.Labels[agentv1.JobIDLabel] != p.JobID {
			if p.ContainerID == id {
				return "", agent.ErrLegacyTarget
			}
			continue
		}
		before := c
		now, code := time.Now().UTC(), 0
		c.State, c.FinishedAt, c.ExitCode = "exited", &now, &code
		r.containers[id] = c
		if err := r.saveLocked(); err != nil {
			r.containers[id] = before
			return "", err
		}
		r.notify()
		return id, nil
	}
	return "", nil
}

func (r *Runtime) WatchContainerEvents(ctx context.Context) (<-chan struct{}, <-chan error) {
	errors := make(chan error)
	go func() { <-ctx.Done(); close(errors) }()
	return r.events, errors
}
func (r *Runtime) notify() {
	select {
	case r.events <- struct{}{}:
	default:
	}
}
func (r *Runtime) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(r.containers)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(r.path), ".docker-state-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, r.path)
}
