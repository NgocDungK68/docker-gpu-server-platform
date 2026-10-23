package ports

import (
	"context"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
)

// MetadataRepository tách business metadata khỏi runtime reservation store.
type MetadataRepository interface {
	Bootstrap(context.Context, domain.Organization, domain.User) error
	ListOrganizations(context.Context) ([]domain.Organization, error)
	GetOrganization(context.Context, string) (domain.Organization, error)
	SaveOrganization(context.Context, domain.Organization) (domain.Organization, error)
	ListUsers(context.Context) ([]domain.User, error)
	UserByUsername(context.Context, string) (domain.User, error)
	SaveUser(context.Context, domain.User) (domain.User, error)
	CreateSession(context.Context, string, string, time.Time) error
	SessionPrincipal(context.Context, string, time.Time) (domain.Principal, error)
	DeleteSession(context.Context, string) error
	CreateEnrollment(context.Context, domain.Enrollment) error
	ListEnrollments(context.Context) ([]domain.Enrollment, error)
	RevokeEnrollment(context.Context, string) error
	BindEnrollment(context.Context, string, string, time.Time) (domain.ServerOwnership, error)
}
