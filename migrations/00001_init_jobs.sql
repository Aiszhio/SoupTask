-- +goose Up

CREATE TABLE jobs (
    id                uuid PRIMARY KEY,
    idempotency_key   text NOT NULL UNIQUE,
    request_hash      bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    job_type          text NOT NULL CHECK (job_type <> ''),
    payload           jsonb NOT NULL,
    status            text NOT NULL DEFAULT 'queued',
    cancel_requested  boolean NOT NULL DEFAULT false,
    attempt_count     integer NOT NULL DEFAULT 0,
    lease_token       bigint NOT NULL DEFAULT 0,
    lease_owner       text,
    lease_expires_at  timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    started_at        timestamptz,
    finished_at       timestamptz,
    error_message     text
);

CREATE INDEX jobs_list_idx
    ON jobs (created_at, id);

CREATE INDEX jobs_queued_idx
    ON jobs (created_at, id)
    WHERE status = 'queued';

CREATE INDEX jobs_expired_lease_idx
    ON jobs (lease_expires_at, id)
    WHERE status = 'running';

CREATE TABLE job_effects (
    job_id        uuid PRIMARY KEY REFERENCES jobs(id),
    lease_token   bigint NOT NULL,
    recorded_at   timestamptz NOT NULL DEFAULT now()
);

-- +goose Down

DROP TABLE job_effects;
DROP TABLE jobs;