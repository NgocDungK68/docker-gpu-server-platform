package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
	agentcp "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/controlplane"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/gpu"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/state"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/simulator"
)

func main() {
	cfg, err := simulator.LoadConfig()
	if err != nil {
		slog.Error("load simulator configuration", "error", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	reader, err := gpu.New()
	if err != nil {
		logger.Error("load NVIDIA mock NVML library", "error", err)
		os.Exit(1)
	}
	defer reader.Close()
	gpus, _, err := reader.Snapshot(context.Background())
	if err != nil {
		logger.Error("read mock NVML profile", "error", err)
		os.Exit(1)
	}
	runtime, err := simulator.NewRuntime(cfg.RuntimeStateFile, gpus, cfg.ExternalGPUIndexes, cfg.RejectStarts)
	if err != nil {
		logger.Error("initialize simulated Docker runtime", "error", err)
		os.Exit(1)
	}
	defer runtime.Close()
	collector := agent.NewInventoryCollector(runtime, reader, runtime, cfg.Agent.InventoryPolicy)
	client, err := agentcp.New(cfg.Agent.ControlPlaneURL, cfg.Agent.RequestTimeout, agentcp.TLSConfig{CAFile: cfg.Agent.TLSCAFile, CertFile: cfg.Agent.TLSCertFile, KeyFile: cfg.Agent.TLSKeyFile})
	if err != nil {
		logger.Error("initialize Control Plane client", "error", err)
		os.Exit(1)
	}
	runner := agent.NewRunner(agent.RunnerConfig{
		MachineID: cfg.Agent.MachineID, Name: cfg.Agent.Name, AgentVersion: "nvml-mock/sim-0.3",
		Labels: cfg.Agent.Labels, EnrollmentToken: cfg.Agent.EnrollmentToken,
		HeartbeatEvery: cfg.Agent.HeartbeatEvery, InventoryEvery: cfg.Agent.InventoryEvery, CommandPollEvery: cfg.Agent.CommandPollEvery,
	}, client, collector, agent.NewCommandExecutor(runtime, collector), state.NewFileStore(cfg.Agent.StateFile), runtime, logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runner.Run(ctx); err != nil {
		logger.Error("simulator stopped", "error", err)
		os.Exit(1)
	}
}
