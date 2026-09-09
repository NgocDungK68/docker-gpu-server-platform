package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/application"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/config"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/httpapi"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/policy"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/store/durable"
)

func main() {
	configuration, err := config.Load()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}
	logger := newLogger(configuration.LogLevel)
	slog.SetDefault(logger)

	repository, err := durable.Open(configuration.StateFile, configuration.OfflineAfter)
	if err != nil {
		logger.Error("open state repository", "error", err)
		os.Exit(1)
	}
	defer repository.Close()
	controlPlane := application.New(repository, application.Options{
		EnrollmentToken: configuration.EnrollmentToken, HeartbeatInterval: configuration.HeartbeatInterval,
		OfflineAfter: configuration.OfflineAfter, CommandLease: configuration.CommandLease,
		DefaultStrategy: configuration.SchedulerStrategy,
		Policy:          policy.New(policy.DevelopmentFacts{InSizingPlan: configuration.DevelopmentInSizingPlan, QuotaGPUs: configuration.DevelopmentQuotaGPUs, UsedGPUs: configuration.DevelopmentUsedGPUs}),
		RequestLimits:   application.RequestLimits{MaxGPUCount: configuration.MaxGPUCount, MaxTTLSeconds: configuration.MaxTTLSeconds},
	})
	api := httpapi.New(controlPlane, logger, configuration.CORSOrigins, httpapi.Options{PublicAPIToken: configuration.PublicAPIToken})
	server := &http.Server{
		Addr:              configuration.HTTPAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go runScheduler(ctx, logger, controlPlane, configuration.SchedulerInterval)
	go runReconciler(ctx, logger, controlPlane, configuration.ReconcileInterval)

	go func() {
		logger.Info("AIWM control plane started", "address", configuration.HTTPAddr, "strategy", configuration.SchedulerStrategy)
		var serveErr error
		if configuration.TLSCertFile != "" {
			serveErr = server.ListenAndServeTLS(configuration.TLSCertFile, configuration.TLSKeyFile)
		} else {
			serveErr = server.ListenAndServe()
		}
		if err := serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server stopped unexpectedly", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
	logger.Info("AIWM control plane stopped")
}

func runScheduler(ctx context.Context, logger *slog.Logger, controlPlane *application.ControlPlane, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			assigned, err := controlPlane.ScheduleOnce(ctx)
			if err != nil {
				logger.Error("scheduler cycle failed", "error", err)
			} else if assigned > 0 {
				logger.Info("scheduler cycle completed", "assigned_jobs", assigned)
			}
		}
	}
}

func runReconciler(ctx context.Context, logger *slog.Logger, controlPlane *application.ControlPlane, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			changed, err := controlPlane.Reconcile(ctx)
			if err != nil {
				logger.Error("reconciliation cycle failed", "error", err)
			} else if changed > 0 {
				logger.Warn("agents marked offline", "count", changed)
			}
		}
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
