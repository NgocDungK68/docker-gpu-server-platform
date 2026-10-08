package agent

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/platform"
)

type CompatibilityStatus string

const (
	PlatformSupported   CompatibilityStatus = "SUPPORTED"
	PlatformDegraded    CompatibilityStatus = "DEGRADED"
	PlatformUnsupported CompatibilityStatus = "UNSUPPORTED"
)

type RuntimeCompatibility struct {
	Version       string
	APIVersion    string
	OS            string
	NVIDIARuntime bool
}

// RuntimeProbe reads runtime configuration without starting a container.
type RuntimeProbe interface {
	Compatibility(context.Context) (RuntimeCompatibility, error)
}

type CompatibilityReport struct {
	platform.Capabilities
	Status      CompatibilityStatus `json:"status"`
	Schedulable bool                `json:"schedulable"`
}

func discoverPlatform() platform.Capabilities {
	r := platform.Capabilities{OS: runtime.GOOS, Architecture: runtime.GOARCH, CgroupMode: "UNKNOWN"}
	if content, err := os.ReadFile("/etc/machine-id"); err == nil && strings.TrimSpace(string(content)) != "" {
		r.MachineIDAvailable = true
	}
	if content, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		r.KernelVersion = strings.TrimSpace(string(content))
	}
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		r.CgroupMode = "v2"
	} else if _, err := os.Stat("/sys/fs/cgroup"); err == nil {
		r.CgroupMode = "v1"
	}
	r.AgentOperational = r.MachineIDAvailable
	return r
}

// Preflight is diagnostic; degraded execution readiness is not a daemon startup error.
func Preflight(ctx context.Context, docker DockerRuntime, gpus GPUReader, _ error, machineID ...string) CompatibilityReport {
	base := discoverPlatform()
	if len(machineID) > 0 && strings.TrimSpace(machineID[0]) != "" {
		base.MachineIDAvailable = true
		base.AgentOperational = true
	}
	snapshot := collectSources(ctx, docker, gpus, base)
	report := CompatibilityReport{Capabilities: snapshot.capabilities, Status: PlatformDegraded}
	if report.ManagedExecutionReady {
		report.Status = PlatformSupported
	} else if report.OS != "linux" || report.Architecture != "amd64" {
		report.Status = PlatformUnsupported
	}
	report.Schedulable = report.ManagedExecutionReady
	return report
}

type sourceSnapshot struct {
	capabilities platform.Capabilities
	gpus         []agentv1.GPU
	processes    []agentv1.GPUProcess
	containers   []RuntimeContainer
}

// Each source gets its own bounded request. Failure never becomes an authoritative empty source.
func collectSources(ctx context.Context, docker DockerRuntime, gpus GPUReader, base platform.Capabilities) sourceSnapshot {
	result := sourceSnapshot{capabilities: base}
	r := &result.capabilities
	if gpus != nil {
		readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		devices, processes, err := gpus.Snapshot(readCtx)
		if err == nil {
			result.gpus, result.processes = devices, processes
			r.NVMLAvailable, r.GPUInventoryAvailable, r.GPUCount = true, true, len(devices)
			if source, ok := gpus.(interface {
				DriverInfo(context.Context) (string, string)
			}); ok {
				r.NVIDIADriverVersion, r.CUDADriverVersion = source.DriverInfo(readCtx)
			}
		}
		cancel()
	}
	if docker != nil {
		readCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		containers, err := docker.ListContainers(readCtx)
		if err == nil {
			result.containers = containers
			r.DockerAvailable = true
		}
		cancel()
		// Missing runtime support only blocks execution, not usable Docker inventory.
		if r.DockerAvailable {
			probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if probe, ok := docker.(RuntimeProbe); ok {
				if info, err := probe.Compatibility(probeCtx); err == nil {
					r.DockerVersion, r.DockerAPIVersion, r.DockerOS, r.NVIDIAContainerSupport = info.Version, info.APIVersion, info.OS, info.NVIDIARuntime
				}
			} else {
				r.DockerVersion, _ = docker.Version(probeCtx)
			}
			cancel()
		}
	}
	r.Normalize()
	return result
}
