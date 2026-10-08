package httpapi

import (
	"encoding/json"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/platform"
	"testing"
)

func fullAgentCapabilities() *platform.Capabilities {
	c := &platform.Capabilities{AgentOperational: true, MachineIDAvailable: true, OS: "linux", Architecture: "amd64", DockerAvailable: true, DockerOS: "linux", DockerAPIVersion: "1.52", NVMLAvailable: true, GPUInventoryAvailable: true, GPUCount: 1, NVIDIAContainerSupport: true}
	c.Normalize()
	return c
}

func TestServerCapabilityDetailIsOrganizationScoped(t *testing.T) {
	h, _ := identityHandler(t)
	for _, token := range []string{"user-session", "admin-session"} {
		w := identityRequest(h, "GET", "/api/v1/servers/server-own", token, nil)
		var result struct{ Data domain.Server }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Data.Capabilities == nil ||
			result.Data.Capabilities.OperatingMode != platform.Full || !result.Data.Capabilities.ManagedExecutionReady {
			t.Fatalf("missing capabilities: %d %s", w.Code, w.Body)
		}
	}
	if w := identityRequest(h, "GET", "/api/v1/servers/server-other", "user-session", nil); w.Code != 404 {
		t.Fatal("cross-org detail accessible")
	}
	if w := identityRequest(h, "GET", "/api/v1/servers/server-own", "", nil); w.Code != 401 {
		t.Fatal("unauthenticated detail accessible")
	}
}
