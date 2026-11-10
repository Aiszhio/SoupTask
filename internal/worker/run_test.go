package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"SoupWebApplication/internal/jobs"
)

type fakeStore struct {
	lease     jobs.Lease
	claimed   bool
	success   bool
	finishErr error
	stop      context.CancelFunc
}

func (store *fakeStore) Claim(_ context.Context, _ string, _ time.Duration) (jobs.Lease, bool, error) {
	if store.claimed {
		return jobs.Lease{}, false, nil
	}

	store.claimed = true
	return store.lease, true, nil
}

func (store *fakeStore) Finish(_ context.Context, _ jobs.Lease, success bool) error {
	store.success = success
	store.stop()
	return store.finishErr
}

func TestRun(t *testing.T) {
	tests := []struct {
		name        string
		workErr     error
		finishErr   error
		wantSuccess bool
	}{
		{name: "success", wantSuccess: true},
		{name: "failed work", workErr: errors.New("work failed")},
		{name: "stale lease", finishErr: jobs.ErrLeaseLost, wantSuccess: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, stop := context.WithCancel(context.Background())
			defer stop()

			store := &fakeStore{
				lease:     jobs.Lease{ID: 7, Token: 2, Owner: "worker-1"},
				finishErr: test.finishErr,
				stop:      stop,
			}

			err := Run(ctx, store, "worker-1", func(context.Context, []byte) error {
				return test.workErr
			})
			if err != nil {
				t.Fatal(err)
			}
			if store.success != test.wantSuccess {
				t.Fatalf("success = %v, want %v", store.success, test.wantSuccess)
			}
		})
	}
}
