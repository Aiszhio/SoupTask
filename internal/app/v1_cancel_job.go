package app

import (
	"context"

	jobsv1 "SoupWebApplication/pkg/api/jobs/v1"
)

// CancelJob - cancels a queued or running job
func (service *Service) CancelJob(ctx context.Context, request *jobsv1.CancelJobRequest) (*jobsv1.CancelJobResponse, error) {
	var value string
	if request != nil {
		value = request.GetJobId()
	}

	id, err := parseJobID(value)
	if err != nil {
		return nil, err
	}

	job, err := service.store.Cancel(ctx, id)
	if err != nil {
		return nil, storeError(err)
	}

	result, err := toProtoJob(job)
	if err != nil {
		return nil, err
	}

	return &jobsv1.CancelJobResponse{Job: result}, nil
}
