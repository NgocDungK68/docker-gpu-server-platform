package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/application"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/store/memory"
)

func testAPI() http.Handler {
	cp := application.New(memory.New(), application.Options{EnrollmentToken: "enroll-test", OfflineAfter: time.Minute, DefaultStrategy: domain.StrategyBestFit})
	return New(cp, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, Options{PublicAPIToken: "public-test"}).Handler()
}
func TestAuthenticationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		method, path, token string
		status              int
	}{
		{"GET", "/api/v1/jobs", "", 401}, {"GET", "/api/v1/jobs", "Bearer wrong", 401},
		{"GET", "/api/v1/jobs", "public-test", 401}, {"GET", "/api/v1/jobs", "Bearer public-test", 200},
		{"GET", "/api/v1/agents/other/commands", "Bearer public-test", 401},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Authorization", tc.token)
		w := httptest.NewRecorder()
		testAPI().ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
}
func TestPublicValidationAndSecretRedaction(t *testing.T) {
	handler := testAPI()
	valid := domain.CreateJobRequest{Name: "valid-job", Image: "alpine:3.21", Resources: domain.ResourceRequest{GPUCount: 1}}
	for _, tc := range []struct {
		name   string
		change func(*domain.CreateJobRequest)
	}{
		{"invalid image", func(j *domain.CreateJobRequest) { j.Image = "not a valid image" }},
		{"strategy", func(j *domain.CreateJobRequest) { j.Strategy = "made-up" }},
		{"negative memory", func(j *domain.CreateJobRequest) { j.Resources.MemoryMiB = -1 }},
		{"gpu override", func(j *domain.CreateJobRequest) { j.Environment = map[string]string{"NVIDIA_VISIBLE_DEVICES": "all"} }},
		{"sharing", func(j *domain.CreateJobRequest) { j.Resources.AllowSharedGPU = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := valid
			tc.change(&input)
			data, _ := json.Marshal(input)
			r := httptest.NewRequest("POST", "/api/v1/jobs", bytes.NewReader(data))
			r.Header.Set("Authorization", "Bearer public-test")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != 400 {
				t.Fatalf("status %d body %s", w.Code, w.Body)
			}
		})
	}
	valid.Environment = map[string]string{"SECRET": "never-show-me"}
	data, _ := json.Marshal(valid)
	r := httptest.NewRequest("POST", "/api/v1/jobs", bytes.NewReader(data))
	r.Header.Set("Authorization", "Bearer public-test")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 201 || strings.Contains(w.Body.String(), "never-show-me") {
		t.Fatalf("secret exposed or invalid response %d", w.Code)
	}
}
func TestBodyLimitRejectsTrailingWhitespaceAndInternalErrorsAreGeneric(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/v1/jobs", strings.NewReader("{}"+strings.Repeat(" ", 1<<20)))
	r.Header.Set("Authorization", "Bearer public-test")
	w := httptest.NewRecorder()
	testAPI().ServeHTTP(w, r)
	if w.Code != 400 && w.Code != 413 {
		t.Fatalf("oversized body accepted %d", w.Code)
	}
	w = httptest.NewRecorder()
	respond(w, nil, context.DeadlineExceeded, 0)
	if strings.Contains(w.Body.String(), "deadline") {
		t.Fatal("internal error exposed")
	}
}
