package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

type Config struct {
	HTTPAddr          string
	StateFile         string
	PublicAPIToken    string
	TLSCertFile       string
	TLSKeyFile        string
	EnrollmentToken   string
	HeartbeatInterval time.Duration
	OfflineAfter      time.Duration
	CommandLease      time.Duration
	SchedulerInterval time.Duration
	ReconcileInterval time.Duration
	SchedulerStrategy domain.SchedulingStrategy
	CORSOrigins       []string
	LogLevel          string
}

func Load() (Config, error) {
	if err := loadDotEnv(".env"); err != nil {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}
	config := Config{
		StateFile:      env("AIWM_STATE_FILE", ".local/control-plane.gob"),
		PublicAPIToken: env("AIWM_API_TOKEN", ""),
		TLSCertFile:    env("AIWM_TLS_CERT_FILE", ""), TLSKeyFile: env("AIWM_TLS_KEY_FILE", ""),
		HTTPAddr:          env("AIWM_HTTP_ADDR", "127.0.0.1:8080"),
		EnrollmentToken:   env("AIWM_ENROLLMENT_TOKEN", ""),
		SchedulerStrategy: domain.SchedulingStrategy(env("AIWM_SCHEDULER_STRATEGY", string(domain.StrategyBestFit))),
		CORSOrigins:       splitCSV(env("AIWM_CORS_ORIGINS", "http://localhost:5173,http://localhost:3000")),
		LogLevel:          env("AIWM_LOG_LEVEL", "info"),
	}
	var err error
	if config.HeartbeatInterval, err = envDuration("AIWM_AGENT_HEARTBEAT_INTERVAL", 5*time.Second); err != nil {
		return Config{}, err
	}
	if config.OfflineAfter, err = envDuration("AIWM_AGENT_OFFLINE_AFTER", 20*time.Second); err != nil {
		return Config{}, err
	}
	if config.CommandLease, err = envDuration("AIWM_COMMAND_LEASE", 15*time.Second); err != nil {
		return Config{}, err
	}
	if config.SchedulerInterval, err = envDuration("AIWM_SCHEDULER_INTERVAL", 2*time.Second); err != nil {
		return Config{}, err
	}
	if config.ReconcileInterval, err = envDuration("AIWM_RECONCILE_INTERVAL", 5*time.Second); err != nil {
		return Config{}, err
	}
	if config.OfflineAfter <= config.HeartbeatInterval {
		return Config{}, fmt.Errorf("AIWM_AGENT_OFFLINE_AFTER must be greater than heartbeat interval")
	}
	if !domain.ValidStrategy(config.SchedulerStrategy) {
		return Config{}, fmt.Errorf("invalid AIWM_SCHEDULER_STRATEGY")
	}
	if (config.TLSCertFile == "") != (config.TLSKeyFile == "") {
		return Config{}, fmt.Errorf("both TLS certificate and key are required")
	}
	if config.PublicAPIToken == "" {
		return Config{}, fmt.Errorf("AIWM_API_TOKEN is required")
	}
	if config.EnrollmentToken == "" {
		return Config{}, fmt.Errorf("AIWM_ENROLLMENT_TOKEN cannot be empty")
	}
	return config, nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s=%q: %w", key, raw, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return value, nil
}

func splitCSV(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			return fmt.Errorf("invalid .env line: expected KEY=VALUE")
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func BoolEnv(key string, fallback bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("parse %s=%q: %w", key, raw, err)
	}
	return value, nil
}
