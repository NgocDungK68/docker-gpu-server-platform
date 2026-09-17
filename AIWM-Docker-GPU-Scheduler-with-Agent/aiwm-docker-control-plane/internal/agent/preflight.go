package agent

import (
	"context"
	"os"
	"runtime"
	"strings"
)

type CompatibilityStatus string

const (
	PlatformSupported CompatibilityStatus = "SUPPORTED"
	PlatformDegraded CompatibilityStatus = "DEGRADED"
	PlatformUnsupported CompatibilityStatus = "UNSUPPORTED"
)

type RuntimeCompatibility struct {
	Version string `json:"version"`
	APIVersion string `json:"apiVersion"`
	OS string `json:"os"`
	NVIDIARuntime bool `json:"nvidiaRuntime"`
}

// RuntimeProbe là boundary đọc capability; không pull/start container trong preflight.
type RuntimeProbe interface { Compatibility(context.Context) (RuntimeCompatibility,error) }

type CompatibilityReport struct {
	Status CompatibilityStatus `json:"status"`
	Schedulable bool `json:"schedulable"`
	OS string `json:"os"`
	Architecture string `json:"architecture"`
	KernelVersion string `json:"kernelVersion"`
	CgroupMode string `json:"cgroupMode"`
	Docker RuntimeCompatibility `json:"docker"`
	DockerReachable bool `json:"dockerReachable"`
	NVMLAvailable bool `json:"nvmlAvailable"`
	GPUCount int `json:"gpuCount"`
	Issues []string `json:"issues"`
}

// Preflight kiểm tra điều kiện nền tảng; inventory/health/occupancy vẫn quyết định placement.
func Preflight(ctx context.Context,docker DockerRuntime,gpus GPUReader,nvmlError error) CompatibilityReport {
	r:=CompatibilityReport{Status:PlatformSupported,OS:runtime.GOOS,Architecture:runtime.GOARCH,CgroupMode:"UNKNOWN",Issues:[]string{}}
	if runtime.GOOS!="linux" { r.Status=PlatformUnsupported; r.Issues=append(r.Issues,"Production NVML adapter chỉ hỗ trợ Linux"); return r }
	if content,err:=os.ReadFile("/proc/sys/kernel/osrelease"); err==nil { r.KernelVersion=strings.TrimSpace(string(content)) }
	if _,err:=os.Stat("/sys/fs/cgroup/cgroup.controllers"); err==nil { r.CgroupMode="v2" } else if _,err:=os.Stat("/sys/fs/cgroup"); err==nil { r.CgroupMode="v1" }
	if err:=docker.Ping(ctx); err!=nil { r.Issues=append(r.Issues,"Docker/socket không truy cập được: "+err.Error()) } else {
		r.DockerReachable=true
		if probe,ok:=docker.(RuntimeProbe); ok {
			info,err:=probe.Compatibility(ctx); r.Docker=info
			if err!=nil { r.Issues=append(r.Issues,"Không đọc được Docker capability: "+err.Error()) } else {
				if info.OS!="linux" { r.Issues=append(r.Issues,"Docker daemon không chạy Linux containers") }
				if !info.NVIDIARuntime { r.Issues=append(r.Issues,"Docker chưa công bố runtime nvidia; chưa xác minh được GPU container runtime") }
			}
		} else { r.Issues=append(r.Issues,"Runtime adapter chưa hỗ trợ preflight capability") }
	}
	if nvmlError!=nil { r.Issues=append(r.Issues,"NVML không khả dụng: "+nvmlError.Error()) } else if gpus==nil { r.Issues=append(r.Issues,"Thiếu GPUReader") } else {
		devices,_,err:=gpus.Snapshot(ctx)
		if err!=nil { r.Issues=append(r.Issues,"GPU discovery lỗi: "+err.Error()) } else {
			r.NVMLAvailable=true; r.GPUCount=len(devices)
			if len(devices)==0 { r.Issues=append(r.Issues,"NVML không phát hiện GPU") }
		}
	}
	if len(r.Issues)>0 { r.Status=PlatformDegraded }
	r.Schedulable=r.Status==PlatformSupported
	return r
}
