package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

type CommandExecutor struct {
	mu        sync.Mutex
	docker    DockerRuntime
	inventory *InventoryCollector
}

func NewCommandExecutor(docker DockerRuntime, inventory *InventoryCollector) *CommandExecutor {
	return &CommandExecutor{docker: docker, inventory: inventory}
}

func (e *CommandExecutor) Execute(ctx context.Context, command agentv1.Command) agentv1.CommandAckRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch command.Type {
	case "START_CONTAINER":
		var payload agentv1.StartContainerPayload
		if err := strictUnmarshal(command.Payload, &payload); err != nil {
			return failedAck("invalid start payload: " + err.Error())
		}
		if strings.TrimSpace(payload.JobID) == "" || strings.TrimSpace(payload.Image) == "" || len(payload.GPUUUIDs) == 0 {
			return failedAck("start payload requires jobId, image and gpuUuids")
		}
		if err := e.inventory.GPUsAvailable(ctx, payload.GPUUUIDs, payload.JobID); err != nil {
			return failedAck(err.Error())
		}
		containerID, err := e.docker.StartManagedContainer(ctx, payload)
		if err != nil {
			return failedAck("start managed container: " + err.Error())
		}
		return agentv1.CommandAckRequest{Succeeded: true, ContainerID: containerID, Message: "managed container started"}
	case "STOP_CONTAINER":
		var payload agentv1.StopContainerPayload
		if err := strictUnmarshal(command.Payload, &payload); err != nil {
			return failedAck("invalid stop payload: " + err.Error())
		}
		if strings.TrimSpace(payload.JobID) == "" {
			return failedAck("stop payload requires jobId")
		}
		containerID, err := e.docker.StopManagedJob(ctx, payload)
		if err != nil {
			return failedAck("stop managed container: " + err.Error())
		}
		return agentv1.CommandAckRequest{Succeeded: true, ContainerID: containerID, Message: "managed container stopped or already absent"}
	default:
		return failedAck(fmt.Sprintf("unsupported command type %q", command.Type))
	}
}

func strictUnmarshal(payload []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("payload must contain one JSON object")
	}
	return nil
}

func failedAck(message string) agentv1.CommandAckRequest {
	return agentv1.CommandAckRequest{Succeeded: false, Message: message}
}
