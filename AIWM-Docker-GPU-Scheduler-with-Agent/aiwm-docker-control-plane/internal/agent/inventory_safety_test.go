package agent

import (
	"context"
	"errors"
	"testing"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

func TestUnattributedTelemetryBlocksPlacement(t *testing.T) {
	for _, metric := range []string{"memory", "utilization"} {
		t.Run(metric, func(t *testing.T) {
			g := testGPU("GPU-0")
			if metric == "memory" {
				g.MemoryUsedMiB = 256
			} else {
				g.UtilizationPct = 5
			}
			collector := NewInventoryCollector(&fakeRuntime{}, fakeGPUReader{gpus: []agentv1.GPU{g}}, nil)
			if err := collector.GPUsAvailable(context.Background(), []string{g.UUID}, "new"); !errors.Is(err, ErrUnsafeGPU) {
				t.Fatalf("unattributed %s must block placement: %v", metric, err)
			}
		})
	}
}

func TestDockerNumericAndUnknownDeviceMapping(t *testing.T) {
	for _, tc := range []struct {
		name    string
		indexes []int
		uuids   []string
		want    int
	}{
		{"exact index", []int{1}, nil, 1},
		{"unknown index", []int{9}, nil, 2},
		{"unknown UUID", nil, []string{"MIG-not-a-physical-GPU"}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g0, g1 := testGPU("GPU-0"), testGPU("GPU-1")
			g1.Index = 1
			runtime := &fakeRuntime{containers: []RuntimeContainer{{ID: "external", State: "running", GPUIndexes: tc.indexes, GPUUUIDs: tc.uuids}}}
			collector := NewInventoryCollector(runtime, fakeGPUReader{gpus: []agentv1.GPU{g0, g1}}, nil)
			report, err := collector.Snapshot(context.Background(), 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Containers[0].GPUUUIDs) != tc.want {
				t.Fatalf("unsafe mapping: %#v", report.Containers[0])
			}
			if err := collector.GPUsAvailable(context.Background(), []string{"GPU-1"}, "new"); !errors.Is(err, ErrUnsafeGPU) {
				t.Fatal("external GPU accepted")
			}
			if tc.want == 1 {
				if err := collector.GPUsAvailable(context.Background(), []string{"GPU-0"}, "new"); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
