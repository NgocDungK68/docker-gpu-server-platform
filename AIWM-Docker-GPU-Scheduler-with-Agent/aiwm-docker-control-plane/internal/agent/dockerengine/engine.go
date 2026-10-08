package dockerengine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

const userAgent = "aiwm-agent/0.2"

// New restricts the production adapter to the configured local Docker socket.
func New(endpoint string) (*Engine, error) {
	if !strings.HasPrefix(endpoint, "unix://") && !strings.HasPrefix(endpoint, "npipe://") {
		return nil, fmt.Errorf("Docker endpoint must be local")
	}
	c, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation(), client.WithUserAgent(userAgent))
	if err != nil {
		return nil, err
	}
	return &Engine{client: c}, nil
}

type Engine struct {
	client *client.Client
}

func NewFromEnvironment() (*Engine, error) {
	dockerClient, err := client.New(client.FromEnv, client.WithAPIVersionNegotiation(), client.WithUserAgent(userAgent))
	if err != nil {
		return nil, fmt.Errorf("create Docker Engine client: %w", err)
	}
	return &Engine{client: dockerClient}, nil
}

// NewWithClient is primarily useful for adapter tests against a fake Docker
// Engine. Production composition uses New with the validated local endpoint.
func NewWithClient(dockerClient *client.Client) *Engine { return &Engine{client: dockerClient} }

func (e *Engine) Ping(ctx context.Context) error {
	_, err := e.client.Ping(ctx, client.PingOptions{NegotiateAPIVersion: true})
	return err
}

func (e *Engine) Version(ctx context.Context) (string, error) {
	result, err := e.client.ServerVersion(ctx, client.ServerVersionOptions{})
	if err != nil {
		return "", err
	}
	return result.Version, nil
}

func (e *Engine) Compatibility(ctx context.Context) (agent.RuntimeCompatibility,error) {
	v,err:=e.client.ServerVersion(ctx,client.ServerVersionOptions{})
	if err!=nil { return agent.RuntimeCompatibility{},err }
	i,err:=e.client.Info(ctx,client.InfoOptions{})
	if err!=nil { return agent.RuntimeCompatibility{},err }
	_,nvidia:=i.Info.Runtimes["nvidia"]
	return agent.RuntimeCompatibility{Version:v.Version,APIVersion:v.APIVersion,OS:v.Os,NVIDIARuntime:nvidia},nil
}

func (e *Engine) ListContainers(ctx context.Context) ([]agent.RuntimeContainer, error) {
	result, err := e.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return nil, err
	}
	containers := make([]agent.RuntimeContainer, 0, len(result.Items))
	for _, summary := range result.Items {
		item := agent.RuntimeContainer{
			ID: summary.ID, Name: firstName(summary.Names), Image: summary.Image,
			State: string(summary.State), Labels: cloneMap(summary.Labels),
		}
		managed := strings.EqualFold(summary.Labels[agentv1.ManagedLabel], "true")
		if activeState(item.State) || managed {
			inspect, inspectErr := e.client.ContainerInspect(ctx, summary.ID, client.ContainerInspectOptions{})
			if inspectErr != nil {
				// Never publish a potentially incomplete view of running or AIWM
				// containers: retaining the previous inventory is safer than
				// accidentally declaring their GPUs free.
				return nil, fmt.Errorf("inspect container %s: %w", summary.ID, inspectErr)
			}
			item = fromInspect(inspect.Container, item)
		}
		containers = append(containers, item)
	}
	sort.Slice(containers, func(i, j int) bool { return containers[i].ID < containers[j].ID })
	return containers, nil
}

func (e *Engine) StartManagedContainer(ctx context.Context, payload agentv1.StartContainerPayload) (string, error) {
	existing, err := e.ListContainers(ctx)
	if err != nil {
		return "", err
	}
	for _, item := range existing {
		if strings.EqualFold(item.Labels[agentv1.ManagedLabel], "true") && item.Labels[agentv1.JobIDLabel] == payload.JobID {
			if item.State == "created" {
				if _, err := e.client.ContainerStart(ctx, item.ID, client.ContainerStartOptions{}); err != nil {
					return "", fmt.Errorf("restart existing managed container %s: %w", item.ID, err)
				}
			}
			return item.ID, nil
		}
	}
	if _, err := e.client.ImageInspect(ctx, payload.Image); err != nil {
		if !errdefs.IsNotFound(err) {
			return "", fmt.Errorf("inspect image %q: %w", payload.Image, err)
		}
		pull, pullErr := e.client.ImagePull(ctx, payload.Image, client.ImagePullOptions{})
		if pullErr != nil {
			return "", fmt.Errorf("pull image %q: %w", payload.Image, pullErr)
		}
		defer pull.Close()
		if err := pull.Wait(ctx); err != nil {
			return "", fmt.Errorf("pull image %q: %w", payload.Image, err)
		}
	}
	labels := cloneMap(payload.Labels)
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[agentv1.ManagedLabel] = "true"
	labels[agentv1.JobIDLabel] = payload.JobID
	hostConfig := &container.HostConfig{Resources: container.Resources{
		NanoCPUs: payload.CPUMilli * 1_000_000,
		Memory:   payload.MemoryMiB * 1024 * 1024,
		DeviceRequests: []container.DeviceRequest{{
			Driver: "nvidia", DeviceIDs: append([]string(nil), payload.GPUUUIDs...),
			Capabilities: [][]string{{"gpu"}},
		}},
	}}
	created, err := e.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: payload.Name,
		Config: &container.Config{
			Image: payload.Image, Cmd: append([]string(nil), payload.Command...),
			Env: environment(payload.Environment), Labels: labels,
		},
		HostConfig: hostConfig,
	})
	if err != nil {
		return "", err
	}
	if _, err := e.client.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, cleanupErr := e.client.ContainerRemove(cleanupCtx, created.ID, client.ContainerRemoveOptions{Force: true})
		return "", errors.Join(err, cleanupErr)
	}
	return created.ID, nil
}

func (e *Engine) StopManagedJob(ctx context.Context, payload agentv1.StopContainerPayload) (string, error) {
	containers, err := e.ListContainers(ctx)
	if err != nil {
		return "", err
	}
	matched := ""
	foundRequestedID := false
	for _, item := range containers {
		if payload.ContainerID != "" && item.ID == payload.ContainerID {
			foundRequestedID = true
		}
		if payload.ContainerID != "" && item.ID != payload.ContainerID {
			continue
		}
		if !strings.EqualFold(item.Labels[agentv1.ManagedLabel], "true") || item.Labels[agentv1.JobIDLabel] != payload.JobID {
			continue
		}
		if matched == "" {
			matched = item.ID
		}
		if activeState(item.State) {
			timeout := payload.GraceSeconds
			if timeout < 0 {
				timeout = 0
			}
			if _, err := e.client.ContainerStop(ctx, item.ID, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
				return matched, err
			}
		}
	}
	if payload.ContainerID != "" && foundRequestedID && matched == "" {
		return "", agent.ErrLegacyTarget
	}
	return matched, nil
}

func (e *Engine) WatchContainerEvents(ctx context.Context) (<-chan struct{}, <-chan error) {
	events := e.client.Events(ctx, client.EventsListOptions{Filters: make(client.Filters).Add("type", "container")})
	changes := make(chan struct{}, 1)
	errorsChannel := make(chan error, 1)
	go func() {
		defer close(changes)
		defer close(errorsChannel)
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-events.Messages:
				if !ok {
					return
				}
				select {
				case changes <- struct{}{}:
				default:
				}
			case err, ok := <-events.Err:
				if ok && err != nil {
					errorsChannel <- err
				}
				return
			}
		}
	}()
	return changes, errorsChannel
}

func (e *Engine) Close() error { return e.client.Close() }

func fromInspect(inspect container.InspectResponse, fallback agent.RuntimeContainer) agent.RuntimeContainer {
	result := fallback
	result.ID = inspect.ID
	result.Name = strings.TrimPrefix(inspect.Name, "/")
	if inspect.Config != nil {
		result.Image = inspect.Config.Image
		result.Labels = cloneMap(inspect.Config.Labels)
	}
	if inspect.State != nil {
		result.State = string(inspect.State.Status)
		result.InitPID = inspect.State.Pid
		if result.State == "exited" || result.State == "dead" {
			code := inspect.State.ExitCode
			result.ExitCode = &code
		}
		if finishedAt, err := time.Parse(time.RFC3339Nano, inspect.State.FinishedAt); err == nil && !finishedAt.IsZero() {
			result.FinishedAt = &finishedAt
		}
		if startedAt, err := time.Parse(time.RFC3339Nano, inspect.State.StartedAt); err == nil && !startedAt.IsZero() {
			result.StartedAt = &startedAt
		}
	}
	if inspect.HostConfig != nil {
		for _, request := range inspect.HostConfig.DeviceRequests {
			if request.Driver != "" && request.Driver != "nvidia" {
				continue
			}
			if len(request.DeviceIDs) > 0 {
				for _, deviceID := range request.DeviceIDs {
					if strings.HasPrefix(deviceID, "GPU-") || strings.HasPrefix(deviceID, "MIG-") {
						result.GPUUUIDs = append(result.GPUUUIDs, deviceID)
					} else if index, err := strconv.Atoi(deviceID); err == nil && index >= 0 {
						result.GPUIndexes = append(result.GPUIndexes, index)
					} else {
						// Numeric indexes and vendor-specific aliases are not stable
						// enough for central allocation. Protect the full server.
						result.UnboundedGPUAccess = true
					}
				}
			} else if request.Count != 0 {
				result.UnboundedGPUAccess = true
			}
		}
	}
	if inspect.Config != nil {
		for _, entry := range inspect.Config.Env {
			key, value, ok := strings.Cut(entry, "=")
			if !ok || key != "NVIDIA_VISIBLE_DEVICES" {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "", "none", "void":
			case "all":
				if len(result.GPUUUIDs) == 0 && len(result.GPUIndexes) == 0 {
					result.UnboundedGPUAccess = true
				}
			default:
				for _, device := range strings.Split(value, ",") {
					device = strings.TrimSpace(device)
					if strings.HasPrefix(device, "GPU-") || strings.HasPrefix(device, "MIG-") {
						result.GPUUUIDs = append(result.GPUUUIDs, device)
					} else if index, err := strconv.Atoi(device); err == nil && index >= 0 {
						result.GPUIndexes = append(result.GPUIndexes, index)
					} else {
						result.UnboundedGPUAccess = true
					}
				}
			}
		}
	}
	result.GPUUUIDs = unique(result.GPUUUIDs)
	return result
}

func environment(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result
}

func firstName(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimPrefix(values[0], "/")
}

func activeState(state string) bool {
	switch state {
	case "running", "paused", "restarting":
		return true
	default:
		return false
	}
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

func unique(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
