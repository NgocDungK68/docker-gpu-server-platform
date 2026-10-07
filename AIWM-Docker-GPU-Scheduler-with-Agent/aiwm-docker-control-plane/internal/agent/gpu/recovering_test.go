package gpu

import (
	"context"
	"errors"
	"testing"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

type testReader struct {
	fail   bool
	closed int
}

func (r *testReader) Snapshot(context.Context) ([]agentv1.GPU, []agentv1.GPUProcess, error) {
	if r.fail {
		return nil, nil, errors.New("driver temporarily unavailable")
	}
	return []agentv1.GPU{{UUID: "g"}}, nil, nil
}
func (r *testReader) Close() error { r.closed++; return nil }

func TestReaderReopensAfterInitializationAndSnapshotFailures(t *testing.T) {
	attempt := 0
	source := &testReader{}
	r := &RecoveringReader{open: func() (agent.GPUReader, error) {
		attempt++
		if attempt == 1 {
			return nil, errors.New("NVML not loaded")
		}
		return source, nil
	}}
	ctx := context.Background()
	if _, _, err := r.Snapshot(ctx); err == nil {
		t.Fatal("missing failure")
	}
	if g, _, err := r.Snapshot(ctx); err != nil || len(g) != 1 {
		t.Fatalf("recovery: %v", err)
	}
	source.fail = true
	if _, _, err := r.Snapshot(ctx); err == nil || source.closed != 1 {
		t.Fatal("failed reader not closed")
	}
	source.fail = false
	if g, _, err := r.Snapshot(ctx); err != nil || len(g) != 1 || attempt != 3 {
		t.Fatalf("reopen: %v", err)
	}
	_ = r.Close()
}
