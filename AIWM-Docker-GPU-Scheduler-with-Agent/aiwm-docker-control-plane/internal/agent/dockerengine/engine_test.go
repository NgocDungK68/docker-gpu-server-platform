package dockerengine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moby/moby/client"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

func TestStartManagedContainerUsesExactGPUUUIDsAndSafeDefaults(t *testing.T) {
	var createBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1.55/containers/json":
			writeTestJSON(writer, http.StatusOK, []any{})
		case request.Method == http.MethodGet && request.URL.Path == "/v1.55/images/busybox:latest/json":
			writeTestJSON(writer, http.StatusOK, map[string]any{"Id": "sha256:test"})
		case request.Method == http.MethodPost && request.URL.Path == "/v1.55/containers/create":
			if err := json.NewDecoder(request.Body).Decode(&createBody); err != nil {
				t.Error(err)
			}
			writeTestJSON(writer, http.StatusCreated, map[string]any{"Id": "container-1", "Warnings": []string{}})
		case request.Method == http.MethodPost && request.URL.Path == "/v1.55/containers/container-1/start":
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected Docker API request: %s %s", request.Method, request.URL.String())
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	dockerClient, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.55"))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewWithClient(dockerClient)
	defer engine.Close()
	id, err := engine.StartManagedContainer(context.Background(), agentv1.StartContainerPayload{
		JobID: "job-1", Name: "aiwm-job-1", Image: "busybox:latest", GPUUUIDs: []string{"GPU-aaa", "GPU-bbb"},
		CPUMilli: 1500, MemoryMiB: 2048, Labels: map[string]string{"team": "vision"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "container-1" {
		t.Fatalf("container id = %q", id)
	}
	config := createBody["Labels"].(map[string]any)
	if config[agentv1.ManagedLabel] != "true" || config[agentv1.JobIDLabel] != "job-1" {
		t.Fatalf("mandatory ownership labels missing: %#v", config)
	}
	host := createBody["HostConfig"].(map[string]any)
	requests := host["DeviceRequests"].([]any)
	request := requests[0].(map[string]any)
	deviceIDs := request["DeviceIDs"].([]any)
	if len(deviceIDs) != 2 || deviceIDs[0] != "GPU-aaa" || deviceIDs[1] != "GPU-bbb" {
		t.Fatalf("DeviceRequests = %#v", requests)
	}
	if privileged, ok := host["Privileged"].(bool); ok && privileged {
		t.Fatal("managed container unexpectedly requested privileged mode")
	}
}

func TestStopManagedJobRefusesLegacyContainerID(t *testing.T) {
	stopCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1.55/containers/json":
			writeTestJSON(writer, http.StatusOK, []any{map[string]any{
				"Id": "legacy-1", "Names": []string{"/legacy"}, "Image": "old/image", "State": "running", "Labels": map[string]string{"owner": "business"},
			}})
		case request.Method == http.MethodGet && request.URL.Path == "/v1.55/containers/legacy-1/json":
			writeTestJSON(writer, http.StatusOK, map[string]any{
				"Id": "legacy-1", "Name": "/legacy",
				"Config":     map[string]any{"Image": "old/image", "Labels": map[string]string{"owner": "business"}},
				"State":      map[string]any{"Status": "running", "Running": true, "Pid": 101, "StartedAt": "2026-01-01T00:00:00Z"},
				"HostConfig": map[string]any{"DeviceRequests": []any{map[string]any{"Driver": "nvidia", "DeviceIDs": []string{"GPU-old"}, "Capabilities": [][]string{{"gpu"}}}}},
			})
		case request.Method == http.MethodPost && request.URL.Path == "/v1.55/containers/legacy-1/stop":
			stopCalled = true
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected Docker API request: %s %s", request.Method, request.URL.String())
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	dockerClient, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.55"))
	if err != nil {
		t.Fatal(err)
	}
	engine := NewWithClient(dockerClient)
	defer engine.Close()
	_, err = engine.StopManagedJob(context.Background(), agentv1.StopContainerPayload{JobID: "job-1", ContainerID: "legacy-1", GraceSeconds: 1})
	if !errors.Is(err, agent.ErrLegacyTarget) {
		t.Fatalf("StopManagedJob() error = %v, want ErrLegacyTarget", err)
	}
	if stopCalled {
		t.Fatal("legacy container received a Docker stop request")
	}
}

func writeTestJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
