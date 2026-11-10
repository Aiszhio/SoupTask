package app

import (
	"context"

	jobsv1 "SoupWebApplication/pkg/api/jobs/v1"
)

// GetJob - returns the current state of a job
func (service *Service) GetJob(ctx context.Context, request *jobsv1.GetJobRequest) (*jobsv1.GetJobResponse, error) {
	var value string
	if request != nil {
		value = request.GetJobId()
	}

	id, err := parseJobID(value)
	if err != nil {
		return nil, err
	}

	job, err := service.store.Get(ctx, id)
	if err != nil {
		return nil, storeError(err)
	}

	result, err := toProtoJob(job)
	if err != nil {
		return nil, err
	}

	return &jobsv1.GetJobResponse{Job: result}, nil
}
