package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"SoupWebApplication/internal/jobs"
)

// Get - loads one job by ID
func (store *Store) Get(ctx context.Context, id int64) (jobs.Job, error) {
	const query = `
	SELECT id, status, created_at
	FROM jobs
	WHERE id = $1
	`

	job, err := scanJob(store.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return jobs.Job{}, jobs.ErrNotFound
	}
	if err != nil {
		return jobs.Job{}, fmt.Errorf("get job: %w", err)
	}

	return job, nil
}
