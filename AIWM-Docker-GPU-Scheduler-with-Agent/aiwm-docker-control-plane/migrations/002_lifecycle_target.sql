-- Reference migration only; no PostgreSQL adapter/migration runner is enabled.
-- Extends 001 for the current lifecycle model, for a future SQL implementation.
ALTER TABLE servers ADD COLUMN inventory_received_at TIMESTAMPTZ;
ALTER TABLE servers ADD COLUMN docker_version TEXT NOT NULL DEFAULT '';
ALTER TABLE servers ADD COLUMN host JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE jobs ADD COLUMN container_id TEXT;
ALTER TABLE jobs ADD COLUMN last_observed_at TIMESTAMPTZ;
ALTER TABLE jobs ADD COLUMN assignment JSONB;
ALTER TABLE jobs ADD COLUMN events JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE gpus ADD COLUMN state_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE gpus ADD COLUMN observed_consumers JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE agent_commands ADD COLUMN delivered_at TIMESTAMPTZ;

CREATE TABLE observed_containers (
    server_id TEXT NOT NULL REFERENCES servers(id),
    container_id TEXT NOT NULL,
    inventory JSONB NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (server_id, container_id)
);
-- Active assignment UUIDs must additionally be constrained transactionally by
-- the future adapter; this reference DDL alone is not an allocation mechanism.
