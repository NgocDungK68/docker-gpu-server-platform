package agent

import (
	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// discoverHost reads hardware metadata without invoking host commands or changing configuration.
func discoverHost() agentv1.HostInfo {
	hostname, _ := os.Hostname()
	info := agentv1.HostInfo{Hostname: hostname, OS: runtime.GOOS, Architecture: runtime.GOARCH, CPUCount: runtime.NumCPU()}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "MemTotal:" {
				kib, _ := strconv.ParseInt(fields[1], 10, 64)
				info.MemoryTotalMiB = kib / 1024
				break
			}
		}
	}
	return info
}
