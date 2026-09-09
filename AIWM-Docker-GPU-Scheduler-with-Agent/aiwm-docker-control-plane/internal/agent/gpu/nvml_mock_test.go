//go:build linux && nvmlmock

package gpu

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestNVIDIAMockProfiles exercises the production CGO/NVML adapter against NVIDIA's shared library.
func TestNVIDIAMockProfiles(t *testing.T) {
	if os.Getenv("AIWM_TEST_NVML_MOCK") != "1" {
		t.Skip("explicit NVIDIA mock library environment required")
	}
	reader, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	gpus, processes, err := reader.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Base(os.Getenv("MOCK_NVML_CONFIG"))
	count := 4
	if profile == "t4-empty.yaml" {
		count = 2
	}
	if len(gpus) != count {
		t.Fatalf("got %d GPUs, want %d", len(gpus), count)
	}
	for _, gpu := range gpus {
		if gpu.UUID == "" || gpu.Model == "" || gpu.MemoryTotalMiB <= 0 {
			t.Fatalf("invalid NVML GPU: %+v", gpu)
		}
		if profile != "a100-unhealthy.yaml" && !gpu.Healthy {
			t.Fatalf("unexpected unhealthy GPU %+v", gpu)
		}
	}
	switch profile {
	case "a100-external.yaml":
		if len(processes) != 1 || processes[0].PID != 10000 {
			t.Fatalf("external process not discovered: %+v", processes)
		}
	case "a100-unknown.yaml":
		if len(processes) != 1 || processes[0].PID != 424242 {
			t.Fatalf("unknown process not discovered: %+v", processes)
		}
	case "a100-unhealthy.yaml":
		if gpus[0].Healthy {
			t.Fatal("uncorrectable ECC did not make GPU unhealthy")
		}
	case "t4-empty.yaml":
		if len(processes) != 0 || gpus[0].MemoryTotalMiB != 16384 {
			t.Fatal("T4 profile mismatch")
		}
	default:
		t.Fatalf("unexpected test profile %s", profile)
	}
}
