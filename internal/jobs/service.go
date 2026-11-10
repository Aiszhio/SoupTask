package jobs

import "context"

// Store - reads and changes jobs in persistent storage
type Store interface {
	Create(ctx context.Context, key string, payload []byte) (Job, error)
	Get(ctx context.Context, id int64) (Job, error)
	List(ctx context.Context, page Page) ([]Job, bool, error)
	Cancel(ctx context.Context, id int64) (Job, error)
}
