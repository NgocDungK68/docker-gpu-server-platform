package application

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/distribution/reference"
)

var environmentKey = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")

// validateJobRequest limits the public API to safe, bounded whole-GPU Docker requests.
func validateJobRequest(r domain.CreateJobRequest) error {
	invalid := func(message string) error { return fmt.Errorf("%w: %s", domain.ErrInvalidInput, message) }
	if len(strings.TrimSpace(r.Name)) < 3 || len(r.Name) > 128 {
		return invalid("name must contain 3 to 128 characters")
	}
	if len(r.Image) > 512 {
		return invalid("Docker image reference too long")
	}
	if _, err := reference.ParseNormalizedNamed(r.Image); err != nil {
		return invalid("invalid Docker image reference")
	}
	if r.Backend != domain.BackendDocker || r.Resources.AllowSharedGPU {
		return invalid("only standalone Docker with whole physical GPUs is supported")
	}
	if !domain.ValidStrategy(r.Strategy) {
		return invalid("unsupported scheduler strategy")
	}
	if r.Resources.GPUCount < 1 || r.Resources.GPUCount > 64 || r.Priority < 0 || r.Priority > 1000 {
		return invalid("GPU count must be 1..64 and priority 0..1000")
	}
	if r.Resources.MinVRAMMiB < 0 || r.Resources.CPUMilli < 0 || r.Resources.MemoryMiB < 0 || r.Resources.CPUMilli > math.MaxInt64/1_000_000 || r.Resources.MemoryMiB > math.MaxInt64/(1024*1024) {
		return invalid("invalid CPU, RAM or VRAM limit")
	}
	if len(r.Environment) > 128 || len(r.Command) > 256 || len(r.ServerSelector) > 64 {
		return invalid("too many environment variables, arguments or selectors")
	}
	for key, value := range r.Environment {
		if !environmentKey.MatchString(key) || len(value) > 8192 || strings.ContainsRune(value, 0) {
			return invalid("invalid environment key or value")
		}
		if strings.HasPrefix(key, "NVIDIA_") || key == "CUDA_VISIBLE_DEVICES" {
			return invalid("GPU visibility environment is controlled by the agent")
		}
	}
	for _, arg := range r.Command {
		if len(arg) > 8192 || strings.ContainsRune(arg, 0) {
			return invalid("invalid command argument")
		}
	}
	for key, value := range r.ServerSelector {
		if strings.TrimSpace(key) == "" || len(key) > 128 || len(value) > 256 {
			return invalid("invalid server selector")
		}
	}
	return nil
}
