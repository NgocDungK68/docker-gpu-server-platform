-- Reference PostgreSQL target schema; not executed by the current application.
-- Runnable persistence uses internal/store/durable snapshots (version 1).

CREATE TABLE servers (
    id                  TEXT PRIMARY KEY,
    machine_id          TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL,
    status              TEXT NOT NULL,
    drained             BOOLEAN NOT NULL DEFAULT FALSE,
    labels              JSONB NOT NULL DEFAULT '{}'::jsonb,
    agent_token_hash    BYTEA NOT NULL,
    last_heartbeat_at   TIMESTAMPTZ NOT NULL,
    last_inventory_at   TIMESTAMPTZ,
    inventory_version   BIGINT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE gpus (
    uuid                TEXT PRIMARY KEY,
    server_id           TEXT NOT NULL REFERENCES servers(id),
    gpu_index           INTEGER NOT NULL,
    model               TEXT NOT NULL,
    memory_total_mib    BIGINT NOT NULL,
    memory_used_mib     BIGINT NOT NULL DEFAULT 0,
    utilization_pct     DOUBLE PRECISION NOT NULL DEFAULT 0,
    healthy             BOOLEAN NOT NULL,
    state               TEXT NOT NULL,
    assigned_job_id     TEXT,
    observed_at         TIMESTAMPTZ NOT NULL,
    UNIQUE (server_id, gpu_index)
);

CREATE TABLE jobs (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    backend             TEXT NOT NULL DEFAULT 'DOCKER',
    image               TEXT NOT NULL,
    specification       JSONB NOT NULL,
    priority            INTEGER NOT NULL DEFAULT 0,
    strategy            TEXT NOT NULL,
    status              TEXT NOT NULL,
    status_reason       TEXT NOT NULL DEFAULT '',
    server_id           TEXT REFERENCES servers(id),
    assigned_gpu_uuids  TEXT[] NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL
);

ALTER TABLE gpus
    ADD CONSTRAINT gpus_assigned_job_fk
    FOREIGN KEY (assigned_job_id) REFERENCES jobs(id);

CREATE TABLE agent_commands (
    id                  TEXT PRIMARY KEY,
    agent_id            TEXT NOT NULL REFERENCES servers(id),
    job_id              TEXT REFERENCES jobs(id),
    command_type        TEXT NOT NULL,
    status              TEXT NOT NULL,
    payload             JSONB NOT NULL,
    attempts            INTEGER NOT NULL DEFAULT 0,
    lease_until         TIMESTAMPTZ,
    error_message       TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL,
    completed_at        TIMESTAMPTZ
);

CREATE INDEX jobs_queue_idx ON jobs (priority DESC, created_at ASC) WHERE status = 'QUEUED';
CREATE INDEX commands_agent_poll_idx ON agent_commands (agent_id, created_at) WHERE status IN ('PENDING', 'DELIVERED');
