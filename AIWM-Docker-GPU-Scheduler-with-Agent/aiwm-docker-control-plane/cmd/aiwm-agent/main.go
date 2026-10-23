package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
	agentconfig "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/config"
	agentcp "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/controlplane"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/dockerengine"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/gpu"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/state"
)

const agentVersion = "0.2.0"

func main() {
	showVersion := flag.Bool("version", false, "print Agent version and platform, then exit")
	checkOnly := flag.Bool("check", false, "verify Docker Engine and NVML access, print inventory, then exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("aiwm-agent %s (%s/%s)\n", agentVersion, runtime.GOOS, runtime.GOARCH)
		return
	}
	loadConfig := agentconfig.Load
	if *checkOnly {
		loadConfig = agentconfig.LoadForCheck
	}
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
	gpuReader := gpu.NewRecovering()
	defer gpuReader.Close()
	checkCtx, cancelCheck := context.WithTimeout(context.Background(), configuration.RequestTimeout)
	compatibility := agent.Preflight(checkCtx, docker, gpuReader, nil, configuration.MachineID)
	cancelCheck()
	if *checkOnly {
		_ = json.NewEncoder(os.Stdout).Encode(compatibility)
		if !compatibility.ManagedExecutionReady {
			os.Exit(1)
		}
		return
	}
	logger.Info("Agent capability discovery", "mode", compatibility.OperatingMode, "managed_execution_ready", compatibility.ManagedExecutionReady, "reasons", compatibility.Reasons)
	collector := agent.NewInventoryCollector(docker, gpuReader, agent.ProcCgroupResolver{}, configuration.InventoryPolicy)
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
