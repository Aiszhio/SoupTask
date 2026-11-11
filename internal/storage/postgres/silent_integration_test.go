package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"SoupWebApplication/internal/jobs"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func silentStore(t *testing.T) (context.Context, *sql.DB, *Store) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	db.SetMaxOpenConns(16)

	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	return ctx, db, New(db)
}

func silentJob(t *testing.T, ctx context.Context, store *Store, number int) int64 {
	t.Helper()

	key := fmt.Sprintf("silent-%s-%d-%d", t.Name(), number, time.Now().UnixNano())
	job, err := store.Create(ctx, key, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}

	return job.ID
}

func TestSilentDoubleClaim(t *testing.T) {
	ctx, db, store := silentStore(t)
	jobID := silentJob(t, ctx, store, 0)

	type result struct {
		lease jobs.Lease
		found bool
		err   error
	}

	start := make(chan struct{})
	results := make(chan result, 8)

	for number := 0; number < 8; number++ {
		go func(number int) {
			<-start
			lease, found, err := store.Claim(
				ctx, fmt.Sprintf("worker-%d", number), 15*time.Second,
			)
			results <- result{lease: lease, found: found, err: err}
		}(number)
	}

	close(start)

	claims := 0
	var winner jobs.Lease

	for number := 0; number < 8; number++ {
		got := <-results
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.found {
			claims++
			winner = got.lease
		}
	}

	var attempts int
	if err := db.QueryRowContext(
		ctx, "SELECT attempt_count FROM jobs WHERE id = $1", jobID,
	).Scan(&attempts); err != nil {
		t.Fatal(err)
	}

	if claims != 1 || winner.ID != jobID || attempts != 1 {
		t.Fatalf("claims=%d winner=%d attempts=%d", claims, winner.ID, attempts)
	}

	if err := store.Finish(ctx, winner, true); err != nil {
		t.Fatal(err)
	}
}

func TestSilentZombie(t *testing.T) {
	ctx, db, store := silentStore(t)
	jobID := silentJob(t, ctx, store, 0)

	oldLease, found, err := store.Claim(ctx, "worker-A", 15*time.Second)
	if err != nil || !found || oldLease.ID != jobID {
		t.Fatalf("first claim: found=%v lease=%+v err=%v", found, oldLease, err)
	}

	_, err = db.ExecContext(ctx,
		"UPDATE jobs SET lease_expires_at = now() - interval '1 second' WHERE id = $1",
		jobID,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Finish(ctx, oldLease, true); !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatalf("expired worker finish: got %v, want ErrLeaseLost", err)
	}

	newLease, found, err := store.Claim(ctx, "worker-B", 15*time.Second)
	if err != nil || !found || newLease.ID != jobID {
		t.Fatalf("second claim: found=%v lease=%+v err=%v", found, newLease, err)
	}
	if newLease.Token <= oldLease.Token {
		t.Fatalf("lease token did not increase: old=%d new=%d",
			oldLease.Token, newLease.Token)
	}

	if err := store.Finish(ctx, oldLease, true); !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatalf("zombie finish: got %v, want ErrLeaseLost", err)
	}

	var before int
	if err := db.QueryRowContext(ctx,
		"SELECT job_effects_count FROM jobs WHERE id = $1", jobID,
	).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 0 {
		t.Fatalf("effect before current worker finished: %d", before)
	}

	if err := store.Finish(ctx, newLease, true); err != nil {
		t.Fatal(err)
	}

	var status string
	var effects int
	if err := db.QueryRowContext(ctx,
		"SELECT status, job_effects_count FROM jobs WHERE id = $1", jobID,
	).Scan(&status, &effects); err != nil {
		t.Fatal(err)
	}
	if status != "succeeded" || effects != 1 {
		t.Fatalf("status=%s effects=%d", status, effects)
	}
}

func TestSilentCancelFinishRace(t *testing.T) {
	ctx, db, store := silentStore(t)

	for number := 0; number < 100; number++ {
		jobID := silentJob(t, ctx, store, number)
		lease, found, err := store.Claim(ctx, "race-worker", 15*time.Second)
		if err != nil || !found || lease.ID != jobID {
			t.Fatalf("job %d claim: found=%v err=%v", number, found, err)
		}

		var cancelErr, finishErr error

		switch number {
		case 0:
			_, cancelErr = store.Cancel(ctx, jobID)
			finishErr = store.Finish(ctx, lease, true)

		case 1:
			finishErr = store.Finish(ctx, lease, true)
			_, cancelErr = store.Cancel(ctx, jobID)

		default:
			start := make(chan struct{})
			cancelDone := make(chan error, 1)
			finishDone := make(chan error, 1)

			go func() {
				<-start
				_, err := store.Cancel(ctx, jobID)
				cancelDone <- err
			}()

			go func() {
				<-start
				finishDone <- store.Finish(ctx, lease, true)
			}()

			close(start)
			cancelErr = <-cancelDone
			finishErr = <-finishDone
		}

		if cancelErr != nil {
			t.Fatalf("job %d cancel: %v", number, cancelErr)
		}
		if finishErr != nil && !errors.Is(finishErr, jobs.ErrLeaseLost) {
			t.Fatalf("job %d finish: %v", number, finishErr)
		}

		var status string
		var effects int
		var cancelRequested bool

		err = db.QueryRowContext(ctx,
			`SELECT status, job_effects_count, cancel_requested
			 FROM jobs WHERE id = $1`, jobID,
		).Scan(&status, &effects, &cancelRequested)
		if err != nil {
			t.Fatal(err)
		}

		validCanceled := status == "canceled" && effects == 0 && cancelRequested
		validSucceeded := status == "succeeded" && effects == 1 && !cancelRequested

		if !validCanceled && !validSucceeded {
			t.Fatalf("job %d: status=%s effects=%d cancel_requested=%v",
				number, status, effects, cancelRequested)
		}
	}
}
