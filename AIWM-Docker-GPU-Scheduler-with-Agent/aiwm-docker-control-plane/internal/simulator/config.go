package simulator

import (
	"fmt"
	agentconfig "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/config"
	"os"
	"strconv"
	"strings"
)

// Config changes infrastructure only; Agent polling/discovery/commands remain production code.
type Config struct {
	Agent              agentconfig.Config
	RuntimeStateFile   string
	ExternalGPUIndexes []int
	RejectStarts       int
}

// LoadConfig requires NVIDIA's YAML profile, never a custom GPU JSON provider.
func LoadConfig() (Config, error) {
	a, err := agentconfig.Load()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{Agent: a, RuntimeStateFile: os.Getenv("AIWM_SIM_RUNTIME_STATE_FILE")}
	if cfg.RuntimeStateFile == "" {
		cfg.RuntimeStateFile = a.StateFile + ".docker.json"
	}
	if os.Getenv("MOCK_NVML_CONFIG") == "" {
		return Config{}, fmt.Errorf("MOCK_NVML_CONFIG must select an NVIDIA nvml-mock YAML profile")
	}
	for _, part := range strings.Split(os.Getenv("AIWM_SIM_EXTERNAL_GPU_INDEXES"), ",") {
		if strings.TrimSpace(part) == "" {
			continue
		}
		index, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || index < 0 {
			return Config{}, fmt.Errorf("invalid AIWM_SIM_EXTERNAL_GPU_INDEXES")
		}
		cfg.ExternalGPUIndexes = append(cfg.ExternalGPUIndexes, index)
	}
	if raw := os.Getenv("AIWM_SIM_REJECT_STARTS"); raw != "" {
		cfg.RejectStarts, err = strconv.Atoi(raw)
		if err != nil || cfg.RejectStarts < 0 {
			return Config{}, fmt.Errorf("invalid AIWM_SIM_REJECT_STARTS")
		}
	}
	return cfg, nil
}
