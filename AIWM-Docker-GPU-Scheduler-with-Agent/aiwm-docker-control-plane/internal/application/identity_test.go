package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/ports"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/store/memory"
)

type loginMetadata struct {
	ports.MetadataRepository
	user domain.User
	organization domain.Organization
	sessionHash string
	sessionExpires time.Time
}

func (m *loginMetadata) UserByUsername(_ context.Context, username string) (domain.User, error) {
	if username != m.user.Username {
		return domain.User{}, domain.ErrNotFound
	}
	return m.user, nil
}

func (m *loginMetadata) GetOrganization(_ context.Context, id string) (domain.Organization, error) {
	if id != m.organization.ID {
		return domain.Organization{}, domain.ErrNotFound
	}
	return m.organization, nil
}

func (m *loginMetadata) CreateSession(_ context.Context, hash, _ string, expires time.Time) error {
	m.sessionHash, m.sessionExpires = hash, expires
	return nil
}

func TestLoginRequiresEnabledIdentityAndStoresOnlyTokenHash(t *testing.T) {
	const password = "test-password-only"
	hash, err := hashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	metadata := &loginMetadata{
		user: domain.User{ID: "user", Username: "user", PasswordHash: hash, Role: domain.RoleOrganizationUser, OrganizationID: "org", Enabled: true},
		organization: domain.Organization{ID: "org", Enabled: true},
	}
	service := NewIdentity(metadata)
	ctx := context.Background()
	for _, input := range []struct { username, password string }{
		{"user", "wrong-password"}, {"missing", password},
	} {
		if _, err := service.Login(ctx, input.username, input.password); !errors.Is(err, domain.ErrUnauthorized) {
			t.Fatalf("invalid login: %v", err)
		}
	}
	if metadata.sessionHash != "" {
		t.Fatal("invalid login created a session")
	}
	before := time.Now()
	result, err := service.Login(ctx, " USER ", password)
	if err != nil {
		t.Fatal(err)
	}
	if result.Token == "" || metadata.sessionHash == result.Token || metadata.sessionHash != tokenHash(result.Token) ||
		metadata.sessionExpires.Before(before.Add(SessionTTL)) || metadata.sessionExpires.After(time.Now().Add(SessionTTL)) {
		t.Fatal("invalid session hash or TTL")
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), "passwordHash") || strings.Contains(string(encoded), hash) {
		t.Fatal("password hash escaped to login response")
	}
	metadata.organization.Enabled = false
	if _, err := service.Login(ctx, "user", password); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("disabled organization could login")
	}
	metadata.organization.Enabled, metadata.user.Enabled = true, false
	if _, err := service.Login(ctx, "user", password); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatal("disabled account could login")
	}
}

type onboardingMetadata struct {
	ports.MetadataRepository
}

func (*onboardingMetadata) BindEnrollment(_ context.Context, hash, machine string, _ time.Time) (domain.ServerOwnership, error) {
	if hash != tokenHash("enrollment-test") || machine != "machine-test" {
		return domain.ServerOwnership{}, domain.ErrUnauthorized
	}
	return domain.ServerOwnership{
		ServerID: "bound-server", OrganizationID: "bound-org", MachineID: machine,
		DisplayName: "Enrollment name", Labels: map[string]string{"owner": "metadata"},
	}, nil
}

func TestRegistrationTakesOwnershipAndLabelsOnlyFromEnrollment(t *testing.T) {
	repo := memory.New()
	cp := New(repo, Options{Metadata: &onboardingMetadata{}})
	request := agentv1.RegisterRequest{
		ProtocolVersion: agentv1.ProtocolVersion, MachineID: "machine-test", Name: "Agent supplied",
		Labels: map[string]string{"owner": "untrusted", "organizationId": "foreign-org"},
	}
	response, err := cp.RegisterAgent(context.Background(), "enrollment-test", request)
	if err != nil {
		t.Fatal(err)
	}
	server, err := repo.GetServer(context.Background(), response.AgentID)
	if err != nil || server.ID != "bound-server" || server.OrganizationID != "bound-org" ||
		server.Name != "Enrollment name" || server.Labels["owner"] != "metadata" || server.Labels["organizationId"] != "" {
		t.Fatalf("ownership from Agent input: %+v %v", server, err)
	}
	if !server.InventoryReceivedAt.IsZero() {
		t.Fatal("registration became schedulable without full inventory")
	}
}
