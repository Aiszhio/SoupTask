package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"SoupWebApplication/internal/jobs"
)

const leaseFor = 15 * time.Second
const pollEvery = 200 * time.Millisecond

// Execute - runs one job's simulated work
type Execute func(context.Context, []byte) error

// Run - claims and processes jobs until the context is canceled
func Run(ctx context.Context, store jobs.WorkerStore, owner string, execute Execute) error {
	for {
		if ctx.Err() != nil {
			return nil
		}

		lease, found, err := store.Claim(ctx, owner, leaseFor)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return fmt.Errorf("claim: %w", err)
		}

		if !found {
			timer := time.NewTimer(pollEvery)

			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
				continue
			}
		}

		workCtx, cancel := context.WithTimeout(ctx, leaseFor)
		workErr := execute(workCtx, lease.Payload)
		cancel()

		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(workErr, context.Canceled) || errors.Is(workErr, context.DeadlineExceeded) {
			continue
		}

		err = store.Finish(ctx, lease, workErr == nil)
		if errors.Is(err, jobs.ErrLeaseLost) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return fmt.Errorf("finish job %d: %w", lease.ID, err)
		}
	}
}
