package durable

import "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/platform"

func fullAgentCapabilities() *platform.Capabilities {
	c := &platform.Capabilities{AgentOperational: true, MachineIDAvailable: true, OS: "linux", Architecture: "amd64", DockerAvailable: true, DockerOS: "linux", DockerAPIVersion: "1.52", NVMLAvailable: true, GPUInventoryAvailable: true, GPUCount: 1, NVIDIAContainerSupport: true}
	c.Normalize()
	return c
}
