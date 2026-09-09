//go:build linux

package agent

import (
	"fmt"
	"os"
	"regexp"
)

var containerIDPattern = regexp.MustCompile(`(?i)(?:docker[-/])?([a-f0-9]{12,64})(?:\.scope)?(?:$|/)`)

type ProcCgroupResolver struct{}

func (ProcCgroupResolver) ContainerID(pid int) (string, error) {
	content, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return "", err
	}
	matches := containerIDPattern.FindAllSubmatch(content, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("PID %d is not mapped to a Docker cgroup", pid)
	}
	return string(matches[len(matches)-1][1]), nil
}
