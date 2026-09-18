package domain

import (
	"context"
	"time"
)

type Role string

const (
	RoleAdmin Role = "ADMIN"
	RoleOrganizationUser Role = "ORGANIZATION_USER"
)

type Organization struct {
	ID string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
	Enabled bool `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type User struct {
	ID string `json:"id"`
	Username string `json:"username"`
	PasswordHash string `json:"-"`
	Role Role `json:"role"`
	OrganizationID string `json:"organizationId"`
	Enabled bool `json:"enabled"`
}

// Principal chỉ được tạo sau khi backend xác thực session.
type Principal struct {
	User User `json:"user"`
	Organization Organization `json:"organization"`
	ScopeOrganizationID string `json:"-"`
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func CurrentPrincipal(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// CanAccess cho phép worker nội bộ đọc pool; HTTP bắt buộc có Principal.
func CanAccess(ctx context.Context, organizationID string) bool {
	p, ok := CurrentPrincipal(ctx)
	if !ok { return true }
	if p.User.Role == RoleAdmin {
		return p.ScopeOrganizationID == "" || p.ScopeOrganizationID == organizationID
	}
	return SameOrganization(p.User.OrganizationID, organizationID)
}

// SameOrganization không coi hai ownership rỗng là hợp lệ.
func SameOrganization(jobOrganizationID, serverOrganizationID string) bool {
	return jobOrganizationID != "" && jobOrganizationID == serverOrganizationID
}

type Enrollment struct {
	ID string `json:"id"`
	OrganizationID string `json:"organizationId"`
	ServerID string `json:"serverId"`
	DisplayName string `json:"displayName"`
	Labels map[string]string `json:"labels"`
	TokenHash string `json:"-"`
	ExpiresAt time.Time `json:"expiresAt"`
	MachineID string `json:"machineId,omitempty"`
	Revoked bool `json:"revoked"`
}

type ServerOwnership struct {
	ServerID string
	MachineID string
	OrganizationID string
	DisplayName string
	Labels map[string]string
}
