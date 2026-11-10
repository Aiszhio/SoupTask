package app

import (
	"context"
	"errors"
	"strconv"

	"SoupWebApplication/internal/jobs"
	jobsv1 "SoupWebApplication/pkg/api/jobs/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Service - handles requests from the jobs API
type Service struct {
	jobsv1.UnimplementedJobServiceServer
	store jobs.Store
}

var _ jobsv1.JobServiceServer = (*Service)(nil)

// NewService - creates a jobs API service with the given store
func NewService(store jobs.Store) *Service {
	return &Service{store: store}
}

func parseJobID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, status.Error(codes.InvalidArgument, "job_id must be a positive number")
	}

	return id, nil
}

func toProtoJob(job jobs.Job) (*jobsv1.Job, error) {
	var jobStatus jobsv1.JobStatus

	switch job.Status {
	case jobs.Queued:
		jobStatus = jobsv1.JobStatus_JOB_STATUS_QUEUED
	case jobs.Running:
		jobStatus = jobsv1.JobStatus_JOB_STATUS_RUNNING
	case jobs.Succeeded:
		jobStatus = jobsv1.JobStatus_JOB_STATUS_SUCCEEDED
	case jobs.Failed:
		jobStatus = jobsv1.JobStatus_JOB_STATUS_FAILED
	case jobs.Canceled:
		jobStatus = jobsv1.JobStatus_JOB_STATUS_CANCELED
	default:
		return nil, status.Error(codes.Internal, "unknown job status")
	}

	return &jobsv1.Job{
		Id:     strconv.FormatInt(job.ID, 10),
		Status: jobStatus,
	}, nil
}

func storeError(err error) error {
	switch {
	case errors.Is(err, jobs.ErrNotFound):
		return status.Error(codes.NotFound, "job not found")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request canceled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "request deadline exceeded")
	default:
		return status.Error(codes.Internal, "job storage failed")
	}
}
