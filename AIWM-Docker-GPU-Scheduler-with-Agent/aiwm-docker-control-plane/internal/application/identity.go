package application

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/ports"
)

const SessionTTL = 8*time.Hour
const passwordIterations = 600000

type IdentityService struct { Metadata ports.MetadataRepository }

func NewIdentity(metadata ports.MetadataRepository) *IdentityService {
	return &IdentityService{Metadata:metadata}
}

func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func hashPassword(password string) (string,error) {
	if len(password)<12 || len(password)>256 { return "",domain.ErrInvalidInput }
	salt,err := randomToken(16); if err != nil { return "",err }
	key,err := pbkdf2.Key(sha256.New,password,[]byte(salt),passwordIterations,32)
	if err != nil { return "",err }
	return "pbkdf2-sha256-600000$"+salt+"$"+hex.EncodeToString(key),nil
}

func verifyPassword(encoded,password string) bool {
	parts := strings.Split(encoded,"$")
	valid := len(parts)==3 && parts[0]=="pbkdf2-sha256-600000"
	salt := "00000000000000000000000000000000"
	want := make([]byte,32)
	if valid {
		salt = parts[1]
		decoded,err := hex.DecodeString(parts[2])
		valid = err==nil && len(decoded)==32
		if valid { want=decoded }
	}
	got,err := pbkdf2.Key(sha256.New,password,[]byte(salt),passwordIterations,32)
	return err==nil && subtle.ConstantTimeCompare(got,want)==1 && valid
}

type LoginResult struct {
	Token string `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	domain.Principal
}

func (s *IdentityService) Login(ctx context.Context,username,password string) (LoginResult,error) {
	if len(username)>200 || len(password)>256 { return LoginResult{},domain.ErrUnauthorized }
	u,err := s.Metadata.UserByUsername(ctx,strings.ToLower(strings.TrimSpace(username)))
	if err!=nil && !errors.Is(err,domain.ErrNotFound) { return LoginResult{},err }
	if !verifyPassword(u.PasswordHash,password) || !u.Enabled { return LoginResult{},domain.ErrUnauthorized }
	o,err := s.Metadata.GetOrganization(ctx,u.OrganizationID)
	if err!=nil { return LoginResult{},err }; if !o.Enabled { return LoginResult{},domain.ErrUnauthorized }
	token,err := randomToken(32); if err!=nil { return LoginResult{},err }
	expires := time.Now().UTC().Add(SessionTTL)
	if err=s.Metadata.CreateSession(ctx,tokenHash(token),u.ID,expires); err!=nil { return LoginResult{},err }
	return LoginResult{Token:token,ExpiresAt:expires,Principal:domain.Principal{User:u,Organization:o}},nil
}

func (s *IdentityService) Authenticate(ctx context.Context,token string) (domain.Principal,error) {
	if token=="" { return domain.Principal{},domain.ErrUnauthorized }
	return s.Metadata.SessionPrincipal(ctx,tokenHash(token),time.Now().UTC())
}

func (s *IdentityService) Logout(ctx context.Context,token string) error {
	return s.Metadata.DeleteSession(ctx,tokenHash(token))
}

func requireAdmin(ctx context.Context) error {
	p,ok := domain.CurrentPrincipal(ctx)
	if !ok { return domain.ErrUnauthorized }
	if p.User.Role!=domain.RoleAdmin { return domain.ErrForbidden }
	return nil
}

func (s *IdentityService) Organizations(ctx context.Context) ([]domain.Organization,error) {
	items,err := s.Metadata.ListOrganizations(ctx); if err!=nil { return nil,err }
	result := []domain.Organization{}
	for _,o := range items { if domain.CanAccess(ctx,o.ID) { result=append(result,o) } }
	return result,nil
}

type OrganizationInput struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Enabled bool `json:"enabled"`
}

func (s *IdentityService) SaveOrganization(ctx context.Context,id string,input OrganizationInput) (domain.Organization,error) {
	if err:=requireAdmin(ctx); err!=nil { return domain.Organization{},err }
	input.Code=strings.ToUpper(strings.TrimSpace(input.Code)); input.Name=strings.TrimSpace(input.Name)
	if input.Code=="" || len(input.Code)>32 || input.Name=="" || len(input.Name)>200 { return domain.Organization{},domain.ErrInvalidInput }
	var err error
	if id=="" { id,err=newID("org") } else { _,err=s.Metadata.GetOrganization(ctx,id) }
	if err!=nil { return domain.Organization{},err }
	return s.Metadata.SaveOrganization(ctx,domain.Organization{ID:id,Code:input.Code,Name:input.Name,Enabled:input.Enabled})
}

type UserInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role domain.Role `json:"role"`
	OrganizationID string `json:"organizationId"`
	Enabled bool `json:"enabled"`
}

func (s *IdentityService) Users(ctx context.Context) ([]domain.User,error) {
	if err:=requireAdmin(ctx); err!=nil { return nil,err }
	return s.Metadata.ListUsers(ctx)
}

func (s *IdentityService) SaveUser(ctx context.Context,id string,input UserInput) (domain.User,error) {
	if err:=requireAdmin(ctx); err!=nil { return domain.User{},err }
	input.Username=strings.ToLower(strings.TrimSpace(input.Username))
	if input.Username=="" || len(input.Username)>200 || (input.Role!=domain.RoleAdmin && input.Role!=domain.RoleOrganizationUser) { return domain.User{},domain.ErrInvalidInput }
	o,err:=s.Metadata.GetOrganization(ctx,input.OrganizationID)
	if err!=nil { return domain.User{},err }; if !o.Enabled { return domain.User{},domain.ErrConflict }
	isNew:=id==""
	if isNew { id,err=newID("usr"); if err!=nil { return domain.User{},err } } else {
		users,e:=s.Metadata.ListUsers(ctx); if e!=nil { return domain.User{},e }
		found:=false; for _,u:=range users { if u.ID==id { found=true } }
		if !found { return domain.User{},domain.ErrNotFound }
	}
	hash:=""
	if isNew || input.Password!="" { hash,err=hashPassword(input.Password); if err!=nil { return domain.User{},err } }
	return s.Metadata.SaveUser(ctx,domain.User{ID:id,Username:input.Username,PasswordHash:hash,Role:input.Role,OrganizationID:input.OrganizationID,Enabled:input.Enabled})
}

// Bootstrap chỉ tạo account đầu tiên qua CLI nội bộ; không có public self-registration.
func (s *IdentityService) Bootstrap(ctx context.Context,code,name,username,password string) error {
	code=strings.ToUpper(strings.TrimSpace(code)); name=strings.TrimSpace(name); username=strings.ToLower(strings.TrimSpace(username))
	if code=="" || len(code)>32 || name=="" || len(name)>200 || username=="" || len(username)>200 { return domain.ErrInvalidInput }
	hash,err:=hashPassword(password); if err!=nil { return err }
	orgID,err:=newID("org"); if err!=nil { return err }
	userID,err:=newID("usr"); if err!=nil { return err }
	return s.Metadata.Bootstrap(ctx,domain.Organization{ID:orgID,Code:code,Name:name,Enabled:true},domain.User{ID:userID,Username:username,PasswordHash:hash,Role:domain.RoleAdmin,OrganizationID:orgID,Enabled:true})
}

type EnrollmentInput struct {
	DisplayName string `json:"displayName"`
	Labels map[string]string `json:"labels"`
	OrganizationID string `json:"organizationId,omitempty"`
}

type EnrollmentResult struct {
	domain.Enrollment
	EnrollmentToken string `json:"enrollmentToken"`
}

func (s *IdentityService) CreateEnrollment(ctx context.Context,input EnrollmentInput) (EnrollmentResult,error) {
	p,ok:=domain.CurrentPrincipal(ctx); if !ok { return EnrollmentResult{},domain.ErrUnauthorized }
	orgID:=p.User.OrganizationID
	if p.User.Role==domain.RoleAdmin && input.OrganizationID!="" { orgID=input.OrganizationID }
	if p.User.Role!=domain.RoleAdmin && input.OrganizationID!="" { return EnrollmentResult{},domain.ErrForbidden }
	o,err:=s.Metadata.GetOrganization(ctx,orgID); if err!=nil { return EnrollmentResult{},err }; if !o.Enabled { return EnrollmentResult{},domain.ErrConflict }
	input.DisplayName=strings.TrimSpace(input.DisplayName)
	if input.DisplayName=="" || len(input.DisplayName)>200 || len(input.Labels)>32 { return EnrollmentResult{},domain.ErrInvalidInput }
	for key,value:=range input.Labels { if len(key)==0 || len(key)>64 || len(value)>256 { return EnrollmentResult{},domain.ErrInvalidInput } }
	id,err:=newID("enr"); if err!=nil { return EnrollmentResult{},err }
	serverID,err:=newID("srv"); if err!=nil { return EnrollmentResult{},err }
	token,err:=randomToken(32); if err!=nil { return EnrollmentResult{},err }
	e:=domain.Enrollment{ID:id,ServerID:serverID,OrganizationID:orgID,DisplayName:input.DisplayName,Labels:input.Labels,TokenHash:tokenHash(token),ExpiresAt:time.Now().UTC().Add(24*time.Hour)}
	if err=s.Metadata.CreateEnrollment(ctx,e); err!=nil { return EnrollmentResult{},err }
	return EnrollmentResult{Enrollment:e,EnrollmentToken:token},nil
}

func (s *IdentityService) Enrollments(ctx context.Context) ([]domain.Enrollment,error) {
	items,err:=s.Metadata.ListEnrollments(ctx); if err!=nil { return nil,err }
	result:=[]domain.Enrollment{}
	for _,e:=range items { if domain.CanAccess(ctx,e.OrganizationID) { result=append(result,e) } }
	return result,nil
}

func (s *IdentityService) RevokeEnrollment(ctx context.Context,id string) error {
	items,err:=s.Enrollments(ctx); if err!=nil { return err }
	for _,e:=range items { if e.ID==id { return s.Metadata.RevokeEnrollment(ctx,id) } }
	return domain.ErrNotFound
}
