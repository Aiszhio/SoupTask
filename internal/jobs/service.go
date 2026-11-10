package jobs

import jobsv1 "SoupWebApplication/pkg/api/jobs/v1"

// Service exposes the API skeleton until job processing is implemented.
type Service struct {
	jobsv1.UnimplementedJobServiceServer
}
