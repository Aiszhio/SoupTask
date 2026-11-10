package jobs

import (
	"errors"
	"time"
)

// Status - describes the current state of a job
type Status string

const (
	Queued    Status = "queued"
	Running   Status = "running"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
	Canceled  Status = "canceled"
)

// Job - contains the fields needed by the API
type Job struct {
	ID        int64
	Status    Status
	CreatedAt time.Time
}

// Page - selects a page of jobs after the given cursor
type Page struct {
	Limit          int
	AfterCreatedAt time.Time
	AfterID        int64
}

var ErrNotFound = errors.New("job not found")
