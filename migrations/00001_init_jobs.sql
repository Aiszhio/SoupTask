-- +goose Up

CREATE TABLE jobs (
    id                bigserial PRIMARY KEY,
    idempotency_key   text NOT NULL UNIQUE,
    payload           jsonb NOT NULL,
    status            text NOT NULL DEFAULT 'queued',
    cancel_requested  boolean NOT NULL DEFAULT false,
    attempt_count     integer NOT NULL DEFAULT 0,
    lease_token       bigint NOT NULL DEFAULT 0,
    lease_owner       text,
    lease_expires_at  timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    job_effects_count integer NOT NULL DEFAULT 0
);

CREATE INDEX jobs_list_idx
    ON jobs (created_at, id);

CREATE INDEX jobs_queued_idx
    ON jobs (created_at, id)
    WHERE status = 'queued';

CREATE INDEX jobs_expired_lease_idx
    ON jobs (lease_expires_at, id)
    WHERE status = 'running';

-- +goose Down

DROP TABLE jobs;