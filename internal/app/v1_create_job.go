package app

import (
	"context"
	"strings"

	jobsv1 "SoupWebApplication/pkg/api/jobs/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

// CreateJob - creates a job or returns the existing job for the same key
func (service *Service) CreateJob(ctx context.Context, request *jobsv1.CreateJobRequest) (*jobsv1.CreateJobResponse, error) {
	if request == nil || strings.TrimSpace(request.GetIdempotencyKey()) == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}

	payload := []byte("{}")
	if request.GetPayload() != nil {
		var err error

		payload, err = protojson.Marshal(request.GetPayload())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid payload")
		}
	}

	job, err := service.store.Create(ctx, request.GetIdempotencyKey(), payload)
	if err != nil {
		return nil, storeError(err)
	}

	result, err := toProtoJob(job)
	if err != nil {
		return nil, err
	}

	return &jobsv1.CreateJobResponse{Job: result}, nil
}
