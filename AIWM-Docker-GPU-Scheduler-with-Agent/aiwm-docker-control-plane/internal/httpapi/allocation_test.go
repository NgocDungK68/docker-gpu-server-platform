package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

func validAllocationRequest() domain.CreateJobRequest {
	fp8 := false
	return domain.CreateJobRequest{AllocationIntent: domain.AllocationIntent{WorkloadType: domain.WorkloadTraining, NecessityLevel: domain.Necessity2, NecessityReason: "GO_LIVE_90_DAYS", SystemImportance: domain.ImportanceImportant, NeededAt: time.Now().UTC().Format(time.RFC3339), TTLSeconds: 3600},
		Name: "valid-job", Image: "alpine:3.21", Resources: domain.AllocationResources{GPUCount: 1, MinVRAMMiB: 1024, PerformanceProfile: "general", FP8Required: &fp8}}
}
func TestAllocationContractRejectsPlacementAndInvalidJSONTypes(t *testing.T) {
	raw, _ := json.Marshal(validAllocationRequest())
	for _, change := range []func(string) string{
		func(s string) string { return strings.Replace(s, "{", `{"strategy":"first-fit",`, 1) },
		func(s string) string { return strings.Replace(s, "{", `{"priority":1000,`, 1) },
		func(s string) string { return strings.Replace(s, "{", `{"serverSelector":{},`, 1) },
		func(s string) string { return strings.Replace(s, `"gpuCount":1`, `"gpuCount":1.5`, 1) },
		func(s string) string { return strings.Replace(s, `"gpuCount":1`, `"gpuCount":"1"`, 1) },
		func(s string) string { return strings.Replace(s, `"fp8Required":false`, `"fp8Required":"false"`, 1) },
		func(s string) string { return strings.Replace(s, `"fp8Required":false`, `"fp8Required":null`, 1) },
		func(s string) string { return strings.Replace(s, `"gpuCount":1`, `"gpuCount":1,"gpuModel":"A100"`, 1) },
	} {
		for _, path := range []string{"/api/v1/jobs", "/api/v1/jobs/preview"} {
			r := httptest.NewRequest("POST", path, strings.NewReader(change(string(raw))))
			r.Header.Set("Authorization", "Bearer public-test")
			w := httptest.NewRecorder()
			testAPI().ServeHTTP(w, r)
			if w.Code != 400 || !strings.Contains(w.Body.String(), `"fields"`) {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		}
	}
}
func TestPreviewOptionsAndAuth(t *testing.T) {
	for _, tc := range []struct {
		method, path, token string
		code                int
	}{
		{"GET", "/api/v1/jobs/options", "", 401}, {"POST", "/api/v1/jobs/preview", "", 401},
		{"GET", "/api/v1/jobs/options", "Bearer public-test", 200}, {"POST", "/api/v1/jobs/preview", "Bearer public-test", 200},
	} {
		raw, _ := json.Marshal(validAllocationRequest())
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(string(raw)))
		r.Header.Set("Authorization", tc.token)
		w := httptest.NewRecorder()
		testAPI().ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "auxiliaryScore") || strings.Contains(w.Body.String(), "gpuUuids") || strings.Contains(w.Body.String(), "ResolvedModels") {
			t.Fatal("internal placement leaked")
		}
	}
}

func TestFutureJobTimeContract(t *testing.T) {
	api := testAPI()
	input := validAllocationRequest()
	input.NeededAt = time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	raw, _ := json.Marshal(input)
	for _, path := range []string{"/api/v1/jobs/preview", "/api/v1/jobs"} {
		r := httptest.NewRequest("POST", path, strings.NewReader(string(raw)))
		r.Header.Set("Authorization", "Bearer public-test")
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		if w.Code != 200 && w.Code != 201 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var envelope struct {
			Data struct {
				RequestedStartAt, RequestedEndAt time.Time
				Status, PlanningStatus           string
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		start, _ := time.Parse(time.RFC3339Nano, input.NeededAt)
		if !envelope.Data.RequestedStartAt.Equal(start) || !envelope.Data.RequestedEndAt.Equal(start.Add(time.Hour)) {
			t.Fatalf("bad time contract: %s", w.Body)
		}
		if path == "/api/v1/jobs" && envelope.Data.Status != "QUEUED" {
			t.Fatal("submit executed immediately")
		}
	}
}
