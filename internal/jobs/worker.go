package jobs

import (
	"context"
	"errors"
	"time"
)

var ErrLeaseLost = errors.New("job lease was lost")

// Lease - gives one worker temporary ownership of a job
type Lease struct {
	ID      int64
	Token   int64
	Owner   string
	Payload []byte
}

// WorkerStore - provides atomic worker operations
type WorkerStore interface {
	Claim(ctx context.Context, owner string, leaseFor time.Duration) (Lease, bool, error)
	Finish(ctx context.Context, lease Lease, success bool) error
}
