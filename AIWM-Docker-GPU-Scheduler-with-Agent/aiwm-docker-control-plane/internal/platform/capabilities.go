// Package platform contains observed host capabilities, independent of adapters and persistence.
package platform

type OperatingMode string

const (
	Full              OperatingMode = "FULL"
	GPUObserveOnly    OperatingMode = "GPU_OBSERVE_ONLY"
	DockerObserveOnly OperatingMode = "DOCKER_OBSERVE_ONLY"
	Degraded          OperatingMode = "DEGRADED"
)

// Capabilities describes which inventory sources were successfully read in one observation.
// Missing reports must never be interpreted as execution readiness.
type Capabilities struct {
	AgentOperational       bool          `json:"agentOperational"`
	OperatingMode          OperatingMode `json:"operatingMode"`
	OS                     string        `json:"os"`
	Architecture           string        `json:"architecture"`
	KernelVersion          string        `json:"kernelVersion,omitempty"`
	CgroupMode             string        `json:"cgroupMode,omitempty"`
	MachineIDAvailable     bool          `json:"machineIdAvailable"`
	DockerAvailable        bool          `json:"dockerAvailable"`
	DockerVersion          string        `json:"dockerVersion,omitempty"`
	DockerAPIVersion       string        `json:"dockerApiVersion,omitempty"`
	DockerOS               string        `json:"dockerOs,omitempty"`
	GPUInventoryAvailable  bool          `json:"gpuInventoryAvailable"`
	NVMLAvailable          bool          `json:"nvmlAvailable"`
	GPUCount               int           `json:"gpuCount"`
	NVIDIADriverVersion    string        `json:"nvidiaDriverVersion,omitempty"`
	CUDADriverVersion      string        `json:"cudaDriverVersion,omitempty"`
	NVIDIAContainerSupport bool          `json:"nvidiaContainerSupport"`
	ManagedExecutionReady  bool          `json:"managedExecutionReady"`
	Reasons                []string      `json:"reasons"`
}

// Normalize derives mode/readiness from observations, not from an advertised ready flag.
func (c *Capabilities) Normalize() {
	c.Reasons = nil
	if !c.AgentOperational {
		c.Reasons = append(c.Reasons, "IDENTITY_UNAVAILABLE")
	}
	if c.OS != "linux" || c.Architecture != "amd64" {
		c.Reasons = append(c.Reasons, "PLATFORM_UNSUPPORTED")
	}
	if !c.DockerAvailable {
		c.Reasons = append(c.Reasons, "DOCKER_UNAVAILABLE")
	}
	if !c.NVMLAvailable || !c.GPUInventoryAvailable {
		c.Reasons = append(c.Reasons, "NVML_UNAVAILABLE")
	}
	if c.GPUInventoryAvailable && c.GPUCount == 0 {
		c.Reasons = append(c.Reasons, "GPU_UNAVAILABLE")
	}
	if !c.NVIDIAContainerSupport {
		c.Reasons = append(c.Reasons, "NVIDIA_CONTAINER_UNAVAILABLE")
	}
	if c.DockerAvailable && (c.DockerOS != "linux" || c.DockerAPIVersion == "") {
		c.Reasons = append(c.Reasons, "DOCKER_EXECUTION_UNSUPPORTED")
	}
	c.ManagedExecutionReady = len(c.Reasons) == 0
	switch {
	case c.ManagedExecutionReady:
		c.OperatingMode = Full
	case c.GPUInventoryAvailable && !c.DockerAvailable:
		c.OperatingMode = GPUObserveOnly
	case c.DockerAvailable && !c.GPUInventoryAvailable:
		c.OperatingMode = DockerObserveOnly
	default:
		c.OperatingMode = Degraded
	}
}

func (c *Capabilities) Clone() *Capabilities {
	if c == nil {
		return nil
	}
	copy := *c
	copy.Reasons = append([]string{}, c.Reasons...)
	return &copy
}
