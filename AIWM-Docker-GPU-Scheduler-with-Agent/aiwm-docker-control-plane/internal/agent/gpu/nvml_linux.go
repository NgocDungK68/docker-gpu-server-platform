//go:build linux

package gpu

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

const bytesPerMiB = 1024 * 1024

type Reader struct {
	initialized bool
}

func New() (*Reader, error) {
	if result := nvml.Init(); result != nvml.SUCCESS {
		return nil, fmt.Errorf("initialize NVML: %s", nvml.ErrorString(result))
	}
	return &Reader{initialized: true}, nil
}

func (r *Reader) Snapshot(ctx context.Context) ([]agentv1.GPU, []agentv1.GPUProcess, error) {
	count, result := nvml.DeviceGetCount()
	if result != nvml.SUCCESS {
		return nil, nil, fmt.Errorf("get GPU count: %s", nvml.ErrorString(result))
	}
	gpus := make([]agentv1.GPU, 0, count)
	processes := make([]agentv1.GPUProcess, 0)
	seenProcesses := make(map[string]bool)
	for index := 0; index < count; index++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		device, result := nvml.DeviceGetHandleByIndex(index)
		if result != nvml.SUCCESS {
			return nil, nil, fmt.Errorf("get GPU %d handle: %s", index, nvml.ErrorString(result))
		}
		uuid, result := device.GetUUID()
		if result != nvml.SUCCESS {
			return nil, nil, fmt.Errorf("get GPU %d UUID: %s", index, nvml.ErrorString(result))
		}
		name, result := device.GetName()
		if result != nvml.SUCCESS {
			return nil, nil, fmt.Errorf("get GPU %s model: %s", uuid, nvml.ErrorString(result))
		}
		memory, result := device.GetMemoryInfo()
		if result != nvml.SUCCESS {
			return nil, nil, fmt.Errorf("get GPU %s memory: %s", uuid, nvml.ErrorString(result))
		}
		gpu := agentv1.GPU{
			UUID: uuid, Index: index, Model: name, MemoryTotalMiB: int64(memory.Total / bytesPerMiB),
			MemoryUsedMiB: int64(memory.Used / bytesPerMiB), Healthy: true, State: "FREE",
		}
		if utilization, result := device.GetUtilizationRates(); result == nvml.SUCCESS {
			gpu.UtilizationPct = float64(utilization.Gpu)
		} else {
			gpu.Healthy = false
		}
		if current, _, ret := device.GetMigMode(); ret == nvml.SUCCESS && current != nvml.DEVICE_MIG_DISABLE {
			gpu.Healthy = false
		}
		if count, ret := device.GetTotalEccErrors(nvml.MEMORY_ERROR_TYPE_UNCORRECTED, nvml.VOLATILE_ECC); (ret == nvml.SUCCESS && count > 0) || (ret != nvml.SUCCESS && ret != nvml.ERROR_NOT_SUPPORTED) {
			gpu.Healthy = false
		}
		if temperature, result := device.GetTemperature(nvml.TEMPERATURE_GPU); result == nvml.SUCCESS {
			gpu.TemperatureC = float64(temperature)
		}
		compute, result := device.GetComputeRunningProcesses()
		if result != nvml.SUCCESS {
			return nil, nil, fmt.Errorf("get GPU %s compute processes: %s", uuid, nvml.ErrorString(result))
		}
		graphics, result := device.GetGraphicsRunningProcesses()
		if result != nvml.SUCCESS && result != nvml.ERROR_NOT_SUPPORTED {
			return nil, nil, fmt.Errorf("get GPU %s graphics processes: %s", uuid, nvml.ErrorString(result))
		}
		for _, process := range append(compute, graphics...) {
			key := uuid + ":" + strconv.FormatUint(uint64(process.Pid), 10)
			if seenProcesses[key] {
				continue
			}
			seenProcesses[key] = true
			usedMiB := int64(0)
			if process.UsedGpuMemory != math.MaxUint64 {
				usedMiB = int64(process.UsedGpuMemory / bytesPerMiB)
			}
			processes = append(processes, agentv1.GPUProcess{
				PID: int(process.Pid), GPUUUID: uuid, UsedMemoryMiB: usedMiB,
				ProcessName: processName(int(process.Pid)),
			})
		}
		gpus = append(gpus, gpu)
	}
	return gpus, processes, nil
}

func (r *Reader) Close() error {
	if !r.initialized {
		return nil
	}
	r.initialized = false
	if result := nvml.Shutdown(); result != nvml.SUCCESS {
		return fmt.Errorf("shutdown NVML: %s", nvml.ErrorString(result))
	}
	return nil
}

func processName(pid int) string {
	content, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}
