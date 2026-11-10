package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"SoupWebApplication/internal/jobs"
)

var _ jobs.WorkerStore = (*Store)(nil)

// Claim - atomically takes a queued job or an expired lease
func (store *Store) Claim(ctx context.Context, owner string, leaseFor time.Duration) (jobs.Lease, bool, error) {
	if leaseFor < time.Second {
		return jobs.Lease{}, false, fmt.Errorf("lease must be at least one second")
	}

	const query = `
	WITH picked AS (
		SELECT id
		FROM jobs
		WHERE cancel_requested = false
		  AND (status = 'queued' OR (status = 'running' AND lease_expires_at <= now()))
		ORDER BY created_at, id
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	)
	
	UPDATE jobs AS j
	SET status = 'running',
		attempt_count = j.attempt_count + 1,
		lease_token = j.lease_token + 1,
		lease_owner = $1,
		lease_expires_at = now() + ($2::bigint * interval '1 second'),
		updated_at = now()
	FROM picked
	WHERE j.id = picked.id
	RETURNING j.id, j.lease_token, j.payload
	`

	lease := jobs.Lease{Owner: owner}
	err := store.db.QueryRowContext(ctx, query, owner, int64(leaseFor/time.Second)).
		Scan(&lease.ID, &lease.Token, &lease.Payload)

	if errors.Is(err, sql.ErrNoRows) {
		return jobs.Lease{}, false, nil
	}
	if err != nil {
		return jobs.Lease{}, false, fmt.Errorf("claim job: %w", err)
	}

	return lease, true, nil
}

// Finish - writes the effect and final state only for the current lease
func (store *Store) Finish(ctx context.Context, lease jobs.Lease, success bool) error {
	const query = `
	UPDATE jobs
	SET status = CASE WHEN $4::boolean THEN 'succeeded' ELSE 'failed' END,
		job_effects_count = job_effects_count + CASE WHEN $4::boolean THEN 1 ELSE 0 END,
		lease_owner = NULL,
		lease_expires_at = NULL,
		updated_at = now()
	WHERE id = $1
	  AND lease_token = $2
	  AND lease_owner = $3
	  AND status = 'running'
	  AND cancel_requested = false
	  AND lease_expires_at > now()
	`

	result, err := store.db.ExecContext(ctx, query, lease.ID, lease.Token, lease.Owner, success)
	if err != nil {
		return fmt.Errorf("finish job: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read finish result: %w", err)
	}
	if count == 0 {
		return jobs.ErrLeaseLost
	}

	return nil
}
