package config

import (
	"bufio"
	"fmt"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ControlPlaneURL  string
	EnrollmentToken  string
	MachineID        string
	Name             string
	Labels           map[string]string
	StateFile        string
	HeartbeatEvery   time.Duration
	InventoryEvery   time.Duration
	CommandPollEvery time.Duration
	RequestTimeout   time.Duration
	TLSCAFile        string
	TLSCertFile      string
	TLSKeyFile       string
	LogLevel         string
	DockerEndpoint   string
	InventoryPolicy  agent.InventoryPolicy
}

func Load() (Config, error) {
	if err := loadDotEnv(".env.agent"); err != nil {
		return Config{}, fmt.Errorf("load .env.agent: %w", err)
	}
	hostname, _ := os.Hostname()
	machineID := strings.TrimSpace(os.Getenv("AIWM_AGENT_MACHINE_ID"))
	if machineID == "" {
		machineID = readMachineID()
	}
	if machineID == "" {
		machineID = hostname
	}
	configuration := Config{
		DockerEndpoint:  env("AIWM_DOCKER_ENDPOINT", "unix:///var/run/docker.sock"),
		ControlPlaneURL: env("AIWM_CONTROL_PLANE_URL", "http://localhost:8080"),
		EnrollmentToken: env("AIWM_ENROLLMENT_TOKEN", ""),
		MachineID:       machineID, Name: env("AIWM_AGENT_NAME", hostname),
		Labels:    parseLabels(os.Getenv("AIWM_AGENT_LABELS")),
		StateFile: env("AIWM_AGENT_STATE_FILE", "/var/lib/aiwm-agent/state.json"),
		TLSCAFile: os.Getenv("AIWM_AGENT_TLS_CA_FILE"), TLSCertFile: os.Getenv("AIWM_AGENT_TLS_CERT_FILE"),
		TLSKeyFile: os.Getenv("AIWM_AGENT_TLS_KEY_FILE"), LogLevel: env("AIWM_LOG_LEVEL", "info"),
	}
	var err error
	if configuration.HeartbeatEvery, err = duration("AIWM_AGENT_HEARTBEAT_INTERVAL", 5*time.Second); err != nil {
		return Config{}, err
	}
	if configuration.InventoryEvery, err = duration("AIWM_AGENT_INVENTORY_INTERVAL", 15*time.Second); err != nil {
		return Config{}, err
	}
	if configuration.CommandPollEvery, err = duration("AIWM_AGENT_COMMAND_POLL_INTERVAL", 2*time.Second); err != nil {
		return Config{}, err
	}
	if configuration.RequestTimeout, err = duration("AIWM_AGENT_REQUEST_TIMEOUT", 30*time.Second); err != nil {
		return Config{}, err
	}
	if configuration.MachineID == "" || configuration.Name == "" || configuration.EnrollmentToken == "" {
		return Config{}, fmt.Errorf("agent machine ID, name and enrollment token must not be empty")
	}
	if configuration.HeartbeatEvery <= 0 || configuration.InventoryEvery <= 0 || configuration.CommandPollEvery <= 0 {
		return Config{}, fmt.Errorf("agent intervals must be positive")
	}
	parsed, err := url.Parse(configuration.ControlPlaneURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return Config{}, fmt.Errorf("invalid AIWM_CONTROL_PLANE_URL")
	}
	if !strings.HasPrefix(configuration.DockerEndpoint, "unix://") && !strings.HasPrefix(configuration.DockerEndpoint, "npipe://") {
		return Config{}, fmt.Errorf("AIWM_DOCKER_ENDPOINT must be a local socket")
	}
	memory, err := strconv.ParseInt(env("AIWM_UNKNOWN_MEMORY_MIB", "256"), 10, 64)
	if err != nil || memory <= 0 {
		return Config{}, fmt.Errorf("invalid AIWM_UNKNOWN_MEMORY_MIB")
	}
	util, err := strconv.ParseFloat(env("AIWM_UNKNOWN_UTILIZATION_PCT", "5"), 64)
	if err != nil || util <= 0 || util > 100 {
		return Config{}, fmt.Errorf("invalid AIWM_UNKNOWN_UTILIZATION_PCT")
	}
	configuration.InventoryPolicy = agent.InventoryPolicy{UnknownMemoryMiB: memory, UnknownUtilizationPct: util}
	if configuration.RequestTimeout <= 0 {
		return Config{}, fmt.Errorf("request timeout must be positive")
	}
	return configuration, nil
}

func readMachineID() string {
	content, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s=%q: %w", key, raw, err)
	}
	return value, nil
}

func parseLabels(value string) map[string]string {
	result := make(map[string]string)
	for _, item := range strings.Split(value, ",") {
		key, labelValue, ok := strings.Cut(item, "=")
		if ok && strings.TrimSpace(key) != "" {
			result[strings.TrimSpace(key)] = strings.TrimSpace(labelValue)
		}
	}
	return result
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid .env.agent line: expected KEY=VALUE")
		}
		key, value = strings.TrimSpace(key), strings.Trim(strings.TrimSpace(value), "\"'")
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func Bool(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("parse %s=%q: %w", key, raw, err)
	}
	return value, nil
}
