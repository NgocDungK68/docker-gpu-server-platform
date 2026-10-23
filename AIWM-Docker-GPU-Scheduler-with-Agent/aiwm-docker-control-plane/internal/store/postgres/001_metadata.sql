-- Chỉ business metadata; Job/Assignment/Command/inventory vẫn ở durable.Store.
CREATE TABLE IF NOT EXISTS organizations (
 id text PRIMARY KEY, code text NOT NULL UNIQUE, name text NOT NULL,
 enabled boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS users (
 id text PRIMARY KEY, username text NOT NULL UNIQUE, password_hash text NOT NULL,
 role text NOT NULL CHECK (role IN ('ADMIN', 'ORGANIZATION_USER')),
 organization_id text NOT NULL REFERENCES organizations(id), enabled boolean NOT NULL DEFAULT true
);
CREATE TABLE IF NOT EXISTS sessions (
 token_hash text PRIMARY KEY, user_id text NOT NULL REFERENCES users(id), expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_expiry ON sessions(expires_at);
CREATE TABLE IF NOT EXISTS server_enrollments (
 id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organizations(id),
 server_id text NOT NULL UNIQUE, display_name text NOT NULL, labels jsonb NOT NULL DEFAULT '{}',
 token_hash text NOT NULL UNIQUE, expires_at timestamptz NOT NULL,
 machine_id text, revoked boolean NOT NULL DEFAULT false
);
CREATE TABLE IF NOT EXISTS server_ownership (
 server_id text PRIMARY KEY, machine_id text NOT NULL UNIQUE,
 organization_id text NOT NULL REFERENCES organizations(id),
 enrollment_id text NOT NULL UNIQUE REFERENCES server_enrollments(id)
);
