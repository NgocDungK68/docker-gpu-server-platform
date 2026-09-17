package postgres

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/domain"
	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/ports"
)

//go:embed 001_metadata.sql
var schema string

type Store struct { db *sql.DB }

var _ ports.MetadataRepository = (*Store)(nil)

func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil { return nil, err }
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30*time.Minute)
	if err = db.PingContext(ctx); err != nil { db.Close(); return nil, err }
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Bootstrap khóa việc tạo user đầu tiên và commit organization/user cùng transaction.
func (s *Store) Bootstrap(ctx context.Context,o domain.Organization,u domain.User) error {
	tx,err:=s.db.BeginTx(ctx,nil); if err!=nil { return err }; defer tx.Rollback()
	if _,err=tx.ExecContext(ctx,`LOCK TABLE users IN EXCLUSIVE MODE`); err!=nil { return err }
	var count int
	if err=tx.QueryRowContext(ctx,`SELECT count(*) FROM users`).Scan(&count); err!=nil { return err }
	if count!=0 { return domain.ErrConflict }
	if _,err=tx.ExecContext(ctx,`INSERT INTO organizations(id,code,name,enabled) VALUES($1,$2,$3,true)`,o.ID,o.Code,o.Name); err!=nil { return translate(err) }
	if _,err=tx.ExecContext(ctx,`INSERT INTO users(id,username,password_hash,role,organization_id,enabled) VALUES($1,$2,$3,'ADMIN',$4,true)`,u.ID,u.Username,u.PasswordHash,o.ID); err!=nil { return translate(err) }
	return tx.Commit()
}

// Migrate chỉ được gọi từ cờ --migrate, không chạy ngầm lúc khởi động server.
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, schema); err != nil { return err }
	return tx.Commit()
}

func translate(err error) error {
	if errors.Is(err, sql.ErrNoRows) { return domain.ErrNotFound }
	var pe *pq.Error
	if errors.As(err, &pe) {
		if pe.Code == "23505" { return domain.ErrConflict }
		if pe.Code == "23503" || pe.Code == "23514" { return domain.ErrInvalidInput }
	}
	return err
}

func (s *Store) ListOrganizations(ctx context.Context) ([]domain.Organization, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,code,name,enabled,created_at,updated_at FROM organizations ORDER BY code,id`)
	if err != nil { return nil, err }
	defer rows.Close()
	items := []domain.Organization{}
	for rows.Next() {
		var o domain.Organization
		if err := rows.Scan(&o.ID,&o.Code,&o.Name,&o.Enabled,&o.CreatedAt,&o.UpdatedAt); err != nil { return nil,err }
		items = append(items,o)
	}
	return items, rows.Err()
}

func (s *Store) GetOrganization(ctx context.Context, id string) (domain.Organization,error) {
	var o domain.Organization
	err := s.db.QueryRowContext(ctx, `SELECT id,code,name,enabled,created_at,updated_at FROM organizations WHERE id=$1`,id).Scan(&o.ID,&o.Code,&o.Name,&o.Enabled,&o.CreatedAt,&o.UpdatedAt)
	return o, translate(err)
}

func (s *Store) SaveOrganization(ctx context.Context, o domain.Organization) (domain.Organization,error) {
	err := s.db.QueryRowContext(ctx, `INSERT INTO organizations(id,code,name,enabled) VALUES($1,$2,$3,$4)
 ON CONFLICT(id) DO UPDATE SET code=excluded.code,name=excluded.name,enabled=excluded.enabled,updated_at=now()
 RETURNING created_at,updated_at`,o.ID,o.Code,o.Name,o.Enabled).Scan(&o.CreatedAt,&o.UpdatedAt)
	return o, translate(err)
}

func (s *Store) ListUsers(ctx context.Context) ([]domain.User,error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,username,role,organization_id,enabled FROM users ORDER BY username,id`)
	if err != nil { return nil,err }
	defer rows.Close()
	items := []domain.User{}
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID,&u.Username,&u.Role,&u.OrganizationID,&u.Enabled); err != nil { return nil,err }
		items = append(items,u)
	}
	return items, rows.Err()
}

func (s *Store) UserByUsername(ctx context.Context, username string) (domain.User,error) {
	var u domain.User
	err := s.db.QueryRowContext(ctx, `SELECT id,username,password_hash,role,organization_id,enabled FROM users WHERE username=$1`,username).Scan(&u.ID,&u.Username,&u.PasswordHash,&u.Role,&u.OrganizationID,&u.Enabled)
	return u, translate(err)
}

// SaveUser giữ nguyên organization của account đã có và revoke mọi session khi sửa account.
func (s *Store) SaveUser(ctx context.Context, u domain.User) (domain.User,error) {
	tx,err := s.db.BeginTx(ctx,nil)
	if err != nil { return u,err }; defer tx.Rollback()
	result,err := tx.ExecContext(ctx, `INSERT INTO users(id,username,password_hash,role,organization_id,enabled) VALUES($1,$2,$3,$4,$5,$6)
 ON CONFLICT(id) DO UPDATE SET username=excluded.username,role=excluded.role,enabled=excluded.enabled,
 password_hash=CASE WHEN excluded.password_hash='' THEN users.password_hash ELSE excluded.password_hash END
 WHERE users.organization_id=excluded.organization_id`,u.ID,u.Username,u.PasswordHash,u.Role,u.OrganizationID,u.Enabled)
	if err != nil { return u,translate(err) }
	n,_ := result.RowsAffected(); if n != 1 { return u,domain.ErrConflict }
	if _,err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=$1`,u.ID); err != nil { return u,err }
	return u,tx.Commit()
}

func (s *Store) CreateSession(ctx context.Context, hash,userID string, expires time.Time) error {
	_,err := s.db.ExecContext(ctx, `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)`,hash,userID,expires)
	return err
}

func (s *Store) SessionPrincipal(ctx context.Context, hash string, now time.Time) (domain.Principal,error) {
	var p domain.Principal
	err := s.db.QueryRowContext(ctx, `SELECT u.id,u.username,u.role,u.organization_id,u.enabled,o.id,o.code,o.name,o.enabled,o.created_at,o.updated_at
 FROM sessions s JOIN users u ON u.id=s.user_id JOIN organizations o ON o.id=u.organization_id
 WHERE s.token_hash=$1 AND s.expires_at>$2 AND u.enabled AND o.enabled`,hash,now).Scan(
		&p.User.ID,&p.User.Username,&p.User.Role,&p.User.OrganizationID,&p.User.Enabled,
		&p.Organization.ID,&p.Organization.Code,&p.Organization.Name,&p.Organization.Enabled,&p.Organization.CreatedAt,&p.Organization.UpdatedAt)
	if errors.Is(err,sql.ErrNoRows) { return p,domain.ErrUnauthorized }
	return p,err
}

func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	_,err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=$1 OR expires_at<=now()`,hash)
	return err
}

func (s *Store) CreateEnrollment(ctx context.Context,e domain.Enrollment) error {
	labels,err := json.Marshal(e.Labels); if err != nil { return err }
	_,err = s.db.ExecContext(ctx, `INSERT INTO server_enrollments(id,organization_id,server_id,display_name,labels,token_hash,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`,e.ID,e.OrganizationID,e.ServerID,e.DisplayName,string(labels),e.TokenHash,e.ExpiresAt)
	return translate(err)
}

func (s *Store) ListEnrollments(ctx context.Context) ([]domain.Enrollment,error) {
	rows,err := s.db.QueryContext(ctx, `SELECT id,organization_id,server_id,display_name,labels,expires_at,coalesce(machine_id,''),revoked FROM server_enrollments ORDER BY expires_at DESC,id`)
	if err != nil { return nil,err }; defer rows.Close()
	items := []domain.Enrollment{}
	for rows.Next() {
		var e domain.Enrollment; var labels []byte
		if err := rows.Scan(&e.ID,&e.OrganizationID,&e.ServerID,&e.DisplayName,&labels,&e.ExpiresAt,&e.MachineID,&e.Revoked); err != nil { return nil,err }
		if err := json.Unmarshal(labels,&e.Labels); err != nil { return nil,err }
		items = append(items,e)
	}
	return items,rows.Err()
}

func (s *Store) RevokeEnrollment(ctx context.Context,id string) error {
	_,err := s.db.ExecContext(ctx, `UPDATE server_enrollments SET revoked=true WHERE id=$1`,id)
	return err
}

// BindEnrollment khóa token và bind machine đúng một lần; retry cùng machine trả lại ownership cũ.
func (s *Store) BindEnrollment(ctx context.Context,hash,machine string,now time.Time) (domain.ServerOwnership,error) {
	var owner domain.ServerOwnership
	tx,err := s.db.BeginTx(ctx,nil); if err != nil { return owner,err }; defer tx.Rollback()
	var id,bound string; var labels []byte; var expires time.Time
	err = tx.QueryRowContext(ctx, `SELECT e.id,e.server_id,e.organization_id,e.display_name,e.labels,coalesce(e.machine_id,''),e.expires_at
 FROM server_enrollments e JOIN organizations o ON o.id=e.organization_id
 WHERE e.token_hash=$1 AND NOT e.revoked AND o.enabled FOR UPDATE OF e`,hash).Scan(&id,&owner.ServerID,&owner.OrganizationID,&owner.DisplayName,&labels,&bound,&expires)
	if errors.Is(err,sql.ErrNoRows) { return owner,domain.ErrUnauthorized }; if err != nil { return owner,err }
	if bound != "" && bound != machine || bound == "" && !now.Before(expires) { return owner,domain.ErrUnauthorized }
	if err = json.Unmarshal(labels,&owner.Labels); err != nil { return owner,err }
	owner.MachineID = machine
	if bound == "" {
		_,err = tx.ExecContext(ctx, `INSERT INTO server_ownership(server_id,machine_id,organization_id,enrollment_id) VALUES($1,$2,$3,$4)`,owner.ServerID,machine,owner.OrganizationID,id)
		if err != nil { return owner,translate(err) }
		if _,err = tx.ExecContext(ctx, `UPDATE server_enrollments SET machine_id=$1 WHERE id=$2`,machine,id); err != nil { return owner,err }
	}
	return owner,tx.Commit()
}
