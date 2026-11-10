package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"SoupWebApplication/internal/jobs"
)

// Cancel - marks an unfinished job canceled and fences off its worker
func (store *Store) Cancel(ctx context.Context, id int64) (jobs.Job, error) {
	const query = `
	UPDATE jobs
	SET status = 'canceled',
		cancel_requested = true,
		lease_owner = NULL,
		lease_expires_at = NULL,
		updated_at = now()
	WHERE id = $1 AND status IN ('queued', 'running')
	RETURNING id, status, created_at
	`

	job, err := scanJob(store.db.QueryRowContext(ctx, query, id))
	if err == nil {
		return job, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return jobs.Job{}, fmt.Errorf("cancel job: %w", err)
	}

	return store.Get(ctx, id)
}
