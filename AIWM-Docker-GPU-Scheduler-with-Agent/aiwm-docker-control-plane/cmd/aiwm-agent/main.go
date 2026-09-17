package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
	agentconfig "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/config"
	agentcp "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/controlplane"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/dockerengine"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/gpu"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/state"
)

const agentVersion = "0.2.0"

func main() {
	checkOnly := flag.Bool("check", false, "verify Docker Engine and NVML access, print inventory, then exit")
	flag.Parse()
	loadConfig:=agentconfig.Load
	if *checkOnly { loadConfig=agentconfig.LoadForCheck }
	configuration, err := loadConfig()
	if err != nil {
		slog.Error("load agent configuration", "error", err)
		os.Exit(1)
	}
	logger := newLogger(configuration.LogLevel)
	slog.SetDefault(logger)
	docker, err := dockerengine.New(configuration.DockerEndpoint)
	if err != nil {
		logger.Error("initialize Docker Engine adapter", "error", err)
		os.Exit(1)
	}
	defer docker.Close()
	gpuReader, nvmlError := gpu.New()
	if nvmlError==nil { defer gpuReader.Close() }
	checkCtx,cancelCheck:=context.WithTimeout(context.Background(),configuration.RequestTimeout)
	compatibility:=agent.Preflight(checkCtx,docker,gpuReader,nvmlError)
	cancelCheck()
	if *checkOnly || compatibility.Status!=agent.PlatformSupported {
		_ = json.NewEncoder(os.Stdout).Encode(compatibility)
		if compatibility.Status!=agent.PlatformSupported { os.Exit(1) }
	}
	collector := agent.NewInventoryCollector(docker, gpuReader, agent.ProcCgroupResolver{}, configuration.InventoryPolicy)
	if *checkOnly {
		ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second)
		defer cancel()
		if err := docker.Ping(ctx); err != nil {
			logger.Error("Docker Engine preflight failed", "error", err)
			os.Exit(1)
		}
		report, err := collector.Snapshot(ctx, 1)
		if err != nil {
			logger.Error("inventory preflight failed", "error", err)
			os.Exit(1)
		}
		logger.Info("agent preflight passed", "docker_version", report.DockerVersion, "gpus", len(report.GPUs), "containers", len(report.Containers), "gpu_processes", len(report.Processes))
		return
	}
	controlPlane, err := agentcp.New(configuration.ControlPlaneURL, configuration.RequestTimeout, agentcp.TLSConfig{
		CAFile: configuration.TLSCAFile, CertFile: configuration.TLSCertFile, KeyFile: configuration.TLSKeyFile,
	})
	if err != nil {
		logger.Error("initialize Control Plane client", "error", err)
		os.Exit(1)
	}
	executor := agent.NewCommandExecutor(docker, collector)
	runner := agent.NewRunner(agent.RunnerConfig{
		MachineID: configuration.MachineID, Name: configuration.Name, AgentVersion: agentVersion,
		Labels: configuration.Labels, EnrollmentToken: configuration.EnrollmentToken,
		HeartbeatEvery: configuration.HeartbeatEvery, InventoryEvery: configuration.InventoryEvery,
		CommandPollEvery: configuration.CommandPollEvery, CommandLimit: 10, MaxCommandHistory: 1000,
	}, controlPlane, collector, executor, state.NewFileStore(configuration.StateFile), docker, logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runner.Run(ctx); err != nil {
		logger.Error("AIWM agent stopped", "error", err)
		os.Exit(1)
	}
}

func newLogger(level string) *slog.Logger {
	logLevel := slog.LevelInfo
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
}
