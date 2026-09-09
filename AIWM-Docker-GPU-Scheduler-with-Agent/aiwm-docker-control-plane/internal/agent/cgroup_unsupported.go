//go:build !linux

package agent

import "fmt"

type ProcCgroupResolver struct{}

func (ProcCgroupResolver) ContainerID(pid int) (string, error) {
	return "", fmt.Errorf("Docker cgroup PID mapping is only supported on Linux (pid %d)", pid)
}
