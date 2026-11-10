package postgres

import (
	"database/sql"

	"SoupWebApplication/internal/jobs"
)

// Store - persists jobs in PostgreSQL
type Store struct {
	db *sql.DB
}

var _ jobs.Store = (*Store)(nil)

// New - creates a PostgreSQL job store from an open database
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(row rowScanner) (jobs.Job, error) {
	var job jobs.Job
	var state string

	err := row.Scan(&job.ID, &state, &job.CreatedAt)
	if err != nil {
		return jobs.Job{}, err
	}

	job.Status = jobs.Status(state)

	return job, nil
}
