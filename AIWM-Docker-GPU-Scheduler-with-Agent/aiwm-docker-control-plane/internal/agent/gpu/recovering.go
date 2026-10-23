package gpu

import (
	"context"
	"sync"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

// RecoveringReader retries NVML initialization on the next inventory cycle after a failure.
type RecoveringReader struct {
	mu     sync.Mutex
	reader agent.GPUReader
	open   func() (agent.GPUReader, error)
}

func NewRecovering() *RecoveringReader {
	return &RecoveringReader{open: func() (agent.GPUReader, error) { return New() }}
}

func (r *RecoveringReader) Snapshot(ctx context.Context) ([]agentv1.GPU, []agentv1.GPUProcess, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if r.reader == nil {
		reader, err := r.open()
		if err != nil {
			return nil, nil, err
		}
		r.reader = reader
	}
	gpus, processes, err := r.reader.Snapshot(ctx)
	if err != nil {
		_ = r.reader.Close()
		r.reader = nil
	}
	return gpus, processes, err
}

func (r *RecoveringReader) DriverInfo(ctx context.Context) (string, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if source, ok := r.reader.(interface {
		DriverInfo(context.Context) (string, string)
	}); ok {
		return source.DriverInfo(ctx)
	}
	return "", ""
}

func (r *RecoveringReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reader == nil {
		return nil
	}
	err := r.reader.Close()
	r.reader = nil
	return err
}
