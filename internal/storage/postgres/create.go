package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"SoupWebApplication/internal/jobs"
)

// Create - inserts a job once for an idempotency key
func (store *Store) Create(ctx context.Context, key string, payload []byte) (jobs.Job, error) {
	const insert = `
	INSERT INTO jobs (idempotency_key, payload)
	VALUES ($1, $2::jsonb)
	ON CONFLICT (idempotency_key) DO NOTHING
	RETURNING id, status, created_at
	`

	job, err := scanJob(store.db.QueryRowContext(ctx, insert, key, string(payload)))
	if err == nil {
		return job, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return jobs.Job{}, fmt.Errorf("insert job: %w", err)
	}

	const existing = `
	SELECT id, status, created_at
	FROM jobs
	WHERE idempotency_key = $1
	`

	job, err = scanJob(store.db.QueryRowContext(ctx, existing, key))
	if err != nil {
		return jobs.Job{}, fmt.Errorf("get job by idempotency key: %w", err)
	}

	return job, nil
}
