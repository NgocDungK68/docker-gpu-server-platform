package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/application"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/ports"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/store/memory"
)

// Chỉ thay metadata boundary; HTTP middleware và application scoping dùng code thực.
type identityMetadata struct {
	ports.MetadataRepository
	sessions map[string]domain.Principal
	enrollment domain.Enrollment
}

func (m *identityMetadata) SessionPrincipal(_ context.Context, hash string, _ time.Time) (domain.Principal, error) {
	p, ok := m.sessions[hash]
	if !ok {
		return domain.Principal{}, domain.ErrUnauthorized
	}
	return p, nil
}

func (m *identityMetadata) DeleteSession(_ context.Context, hash string) error {
	delete(m.sessions, hash)
	return nil
}

func (m *identityMetadata) ListOrganizations(context.Context) ([]domain.Organization, error) {
	return []domain.Organization{{ID: "own", Enabled: true}, {ID: "other", Enabled: true}}, nil
}

func (m *identityMetadata) GetOrganization(_ context.Context, id string) (domain.Organization, error) {
	return domain.Organization{ID: id, Enabled: true}, nil
}

func (m *identityMetadata) CreateEnrollment(_ context.Context, enrollment domain.Enrollment) error {
	m.enrollment = enrollment
	return nil
}

func identityHandler(t *testing.T) (http.Handler, *identityMetadata) {
	t.Helper()
	ctx := context.Background()
	repo := memory.New(time.Minute)
	for _, org := range []string{"own", "other"} {
		now := time.Now()
		_, err := repo.UpsertServer(ctx, domain.Server{
			ID: "server-" + org, MachineID: "machine-" + org, OrganizationID: org,
			Status: domain.ServerOnline, LastHeartbeatAt: now, InventoryReceivedAt: now,
			GPUs: []domain.GPU{{UUID: "GPU-" + org, Model: "A100", MemoryTotalMiB: 40960, Healthy: true, State: domain.GPUFree}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateJob(ctx, domain.Job{ID: "job-" + org, OrganizationID: org, Status: domain.JobQueued}); err != nil {
			t.Fatal(err)
		}
	}
	m := &identityMetadata{sessions: make(map[string]domain.Principal)}
	for token, role := range map[string]domain.Role{"user-session": domain.RoleOrganizationUser, "admin-session": domain.RoleAdmin} {
		hash := sha256.Sum256([]byte(token))
		m.sessions[hex.EncodeToString(hash[:])] = domain.Principal{
			User: domain.User{ID: token, OrganizationID: "own", Role: role, Enabled: true},
			Organization: domain.Organization{ID: "own", Enabled: true},
		}
	}
	cp := application.New(repo, application.Options{Metadata: m, OfflineAfter: time.Minute, DefaultStrategy: domain.StrategyBestFit})
	handler := New(cp, slog.New(slog.NewTextHandler(io.Discard, nil)), nil, Options{
		Identity: application.NewIdentity(m), PublicAPIToken: "old-shared-token",
	}).Handler()
	return handler, m
}

func identityRequest(handler http.Handler, method, path, token string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestSessionAuthorizesOrganizationAtBackend(t *testing.T) {
	handler, _ := identityHandler(t)
	for _, tc := range []struct {
		method, path, token, body string
		status int
	}{
		{"GET", "/healthz", "", "", 200},
		{"GET", "/api/v1/servers", "", "", 401},
		{"GET", "/api/v1/servers", "old-shared-token", "", 401},
		{"GET", "/api/v1/servers/server-own", "user-session", "", 200},
		{"GET", "/api/v1/servers/server-other", "user-session", "", 404},
		{"POST", "/api/v1/servers/server-other/drain", "user-session", `{"drained":true}`, 404},
		{"GET", "/api/v1/jobs/job-other", "user-session", "", 404},
		{"POST", "/api/v1/jobs/job-other/stop", "user-session", "", 404},
		{"GET", "/api/v1/servers?organizationId=own", "user-session", "", 403},
		{"GET", "/api/v1/users", "user-session", "", 403},
		{"POST", "/api/v1/scheduler/run-once", "user-session", "", 403},
		{"POST", "/api/v1/enrollments", "user-session", `{"displayName":"x","organizationId":"other"}`, 403},
		{"POST", "/api/v1/jobs", "user-session", `{"organizationId":"other"}`, 400},
		{"POST", "/api/v1/agents/register", "", `{"organizationId":"other"}`, 400},
		{"GET", "/api/v1/servers/server-other", "admin-session", "", 200},
	} {
		w := identityRequest(handler, tc.method, tc.path, tc.token, []byte(tc.body))
		if w.Code != tc.status {
			t.Errorf("%s %s: status=%d want=%d body=%s", tc.method, tc.path, w.Code, tc.status, w.Body)
		}
	}
	for _, path := range []string{"/api/v1/servers", "/api/v1/gpus", "/api/v1/jobs", "/api/v1/queue"} {
		w := identityRequest(handler, "GET", path, "user-session", nil)
		var result struct { Data []struct { OrganizationID string `json:"organizationId"` } `json:"data"` }
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Data) != 1 || result.Data[0].OrganizationID != "own" {
			t.Errorf("scope failed for %s: %s", path, w.Body)
		}
	}
	w := identityRequest(handler, "GET", "/api/v1/system/summary", "user-session", nil)
	var summary struct { Data domain.ClusterSummary `json:"data"` }
	if json.Unmarshal(w.Body.Bytes(), &summary) != nil || summary.Data.ServersTotal != 1 || summary.Data.GPUsTotal != 1 {
		t.Fatalf("summary leaked another organization: %s", w.Body)
	}
}

func TestIdentityOwnsJobAndEnrollmentAndLogoutRevokesSession(t *testing.T) {
	handler, metadata := identityHandler(t)
	body, err := json.Marshal(validAllocationRequest())
	if err != nil {
		t.Fatal(err)
	}
	// ADMIN view filter không thay organization của Job submit.
	for _, token := range []string{"user-session", "admin-session"} {
		path := "/api/v1/jobs"
		if token == "admin-session" {
			path += "?organizationId=other"
		}
		w := identityRequest(handler, "POST", path, token, body)
		var result struct { Data JobView `json:"data"` }
		if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Data.OrganizationID != "own" {
			t.Fatalf("job did not inherit identity: %d %s", w.Code, w.Body)
		}
	}
	w := identityRequest(handler, "POST", "/api/v1/enrollments", "user-session", []byte(`{"displayName":"demo"}`))
	if w.Code != 201 || metadata.enrollment.OrganizationID != "own" || metadata.enrollment.TokenHash == "" {
		t.Fatalf("enrollment ownership/hash: %d %s", w.Code, w.Body)
	}
	w = identityRequest(handler, "POST", "/api/v1/auth/logout", "user-session", nil)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	w = identityRequest(handler, "GET", "/api/v1/auth/me", "user-session", nil)
	if w.Code != 401 {
		t.Fatal("logged-out session still accepted")
	}
}
