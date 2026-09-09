//go:build !linux

package gpu

import (
	"context"
	"fmt"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

type Reader struct{}

func New() (*Reader, error) {
	return nil, fmt.Errorf("NVML requires Linux with CGO; run aiwm-agent-sim in the supplied Docker Compose environment")
}

func (*Reader) Snapshot(context.Context) ([]agentv1.GPU, []agentv1.GPUProcess, error) {
	return nil, nil, fmt.Errorf("NVML is unsupported on this operating system")
}

func (*Reader) Close() error { return nil }
