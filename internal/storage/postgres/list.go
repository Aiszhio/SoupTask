package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"SoupWebApplication/internal/jobs"
)

// List - loads a page of jobs ordered by creation time and ID
func (store *Store) List(ctx context.Context, page jobs.Page) ([]jobs.Job, bool, error) {
	const firstPage = `
	SELECT id, status, created_at
	FROM jobs
	ORDER BY created_at, id
	LIMIT $1
	`
	const nextPage = `
	SELECT id, status, created_at
	FROM jobs
	WHERE (created_at, id) > ($1, $2)
	ORDER BY created_at, id
	LIMIT $3
	`

	var (
		rows *sql.Rows
		err  error
	)

	if page.AfterCreatedAt.IsZero() {
		rows, err = store.db.QueryContext(ctx, firstPage, page.Limit+1)
	} else {
		rows, err = store.db.QueryContext(ctx, nextPage, page.AfterCreatedAt, page.AfterID, page.Limit+1)
	}
	if err != nil {
		return nil, false, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	items := make([]jobs.Job, 0, page.Limit+1)

	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, false, fmt.Errorf("scan job: %w", err)
		}

		items = append(items, job)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("read jobs: %w", err)
	}

	hasMore := len(items) > page.Limit
	if hasMore {
		items = items[:page.Limit]
	}

	return items, hasMore, nil
}
