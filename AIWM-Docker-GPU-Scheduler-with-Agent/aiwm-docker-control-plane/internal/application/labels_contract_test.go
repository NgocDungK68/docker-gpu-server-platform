package application

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

func TestDispatchPreservesInternalLabelsAndContainerEnvironment(t *testing.T) {
	f := newPlanningFixture(t)
	r := validAllocation()
	r.NeededAt = f.now.Format(time.RFC3339)
	r.Environment = map[string]string{"APP_MODE": "training", "EMPTY": "", "URL": "a=b"}
	j, err := f.cp.CreateJob(f.ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	f.cycle()
	for _, command := range f.repo.Export().Commands {
		if command.Type != domain.CommandStartContainer {
			continue
		}
		var payload struct {
			JobID       string            `json:"jobId"`
			Labels      map[string]string `json:"labels"`
			Environment map[string]string `json:"environment"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.JobID != j.ID || payload.Labels["aiwm.managed"] != "true" || payload.Labels["aiwm.job-id"] != j.ID {
			t.Fatalf("missing internal ownership/correlation: %+v", payload)
		}
		for key, value := range r.Environment {
			if got, exists := payload.Environment[key]; !exists || got != value {
				t.Fatalf("environment %s not preserved", key)
			}
		}
		return
	}
	t.Fatal("missing START command")
}
