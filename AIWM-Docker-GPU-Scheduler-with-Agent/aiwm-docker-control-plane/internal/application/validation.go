package application

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/capability"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/policy"
	"github.com/distribution/reference"
)

var environmentKey = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")

type RequestLimits struct {
	MaxGPUCount   int   `json:"maxGpuCount"`
	MaxTTLSeconds int64 `json:"maxTtlSeconds"`
}

func DefaultRequestLimits() RequestLimits {
	return RequestLimits{MaxGPUCount: 64, MaxTTLSeconds: 30 * 24 * 3600}
}

// validateJobRequest bounds execution input independently of physical placement.
func validateJobRequest(r domain.CreateJobRequest, limits RequestLimits, catalog capability.Resolver) error {
	fields := map[string]string{}
	if len(strings.TrimSpace(r.Name)) < 3 || len(r.Name) > 128 {
		fields["name"] = "Tên cần từ 3 đến 128 byte"
	}
	if _, err := reference.ParseNormalizedNamed(r.Image); err != nil || len(r.Image) > 512 {
		fields["image"] = "Docker image reference không hợp lệ"
	}
	if r.Backend != domain.BackendDocker {
		fields["backend"] = "Chỉ hỗ trợ Docker"
	}
	if r.Resources.AllowSharedGPU {
		fields["resources.allowSharedGpu"] = "Chỉ cấp phát nguyên GPU vật lý"
	}
	if r.Resources.GPUCount < 1 || r.Resources.GPUCount > limits.MaxGPUCount {
		fields["resources.gpuCount"] = fmt.Sprintf("Số GPU phải là số nguyên từ 1 đến %d", limits.MaxGPUCount)
	}
	if r.Resources.MinVRAMMiB <= 0 {
		fields["resources.minVramMiB"] = "VRAM mỗi GPU phải lớn hơn 0 MiB"
	}
	if r.Resources.CPUMilli < 0 || r.Resources.CPUMilli > math.MaxInt64/1_000_000 {
		fields["resources.cpuMilli"] = "Giới hạn CPU không hợp lệ"
	}
	if r.Resources.MemoryMiB < 0 || r.Resources.MemoryMiB > math.MaxInt64/(1024*1024) {
		fields["resources.memoryMiB"] = "Giới hạn RAM không hợp lệ"
	}
	if r.Resources.FP8Required == nil {
		fields["resources.fp8Required"] = "Chọn có hoặc không yêu cầu FP8"
	}
	if _, err := catalog.Resolve(r.Resources.PerformanceProfile, false); err != nil {
		fields["resources.performanceProfile"] = "Chọn profile hiệu năng được hỗ trợ"
	}
	policy.Validate(r.AllocationIntent, fields)
	if _, err := time.Parse(time.RFC3339, r.NeededAt); err != nil {
		fields["neededAt"] = "Thời điểm cần phải là RFC3339 có múi giờ"
	}
	if r.TTLSeconds <= 0 || r.TTLSeconds > limits.MaxTTLSeconds {
		fields["ttlSeconds"] = fmt.Sprintf("Thời lượng phải từ 1 đến %d giây", limits.MaxTTLSeconds)
	}
	if len(r.Environment) > 128 {
		fields["environment"] = "Tối đa 128 biến môi trường"
	}
	if len(r.Command) > 256 {
		fields["command"] = "Tối đa 256 đối số"
	}
	for key, value := range r.Environment {
		if !environmentKey.MatchString(key) || len(value) > 8192 || strings.ContainsRune(value, 0) ||
			strings.HasPrefix(key, "AIWM_") || strings.HasPrefix(key, "NVIDIA_") || key == "CUDA_VISIBLE_DEVICES" {
			fields["environment"] = "Biến môi trường không hợp lệ; GPU visibility do Agent quản lý"
		}
	}
	for _, arg := range r.Command {
		if len(arg) > 8192 || strings.ContainsRune(arg, 0) {
			fields["command"] = "Đối số không hợp lệ hoặc quá dài"
		}
	}
	if len(fields) > 0 {
		return &domain.ValidationError{Fields: fields}
	}
	return nil
}
