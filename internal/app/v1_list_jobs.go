package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"SoupWebApplication/internal/jobs"
	jobsv1 "SoupWebApplication/pkg/api/jobs/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultPageSize = 50
const maxPageSize = 100

type pageCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
}

// ListJobs - returns jobs in creation order with a cursor for the next page
func (service *Service) ListJobs(ctx context.Context, request *jobsv1.ListJobsRequest) (*jobsv1.ListJobsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	page, err := readPage(request)
	if err != nil {
		return nil, err
	}

	items, hasMore, err := service.store.List(ctx, page)
	if err != nil {
		return nil, storeError(err)
	}

	response := &jobsv1.ListJobsResponse{
		Jobs: make([]*jobsv1.Job, 0, len(items)),
	}

	for _, job := range items {
		result, err := toProtoJob(job)
		if err != nil {
			return nil, err
		}

		response.Jobs = append(response.Jobs, result)
	}

	if hasMore && len(items) > 0 {
		token, err := writePageToken(items[len(items)-1])
		if err != nil {
			return nil, status.Error(codes.Internal, "cannot encode page token")
		}

		response.NextPageToken = token
	}

	return response, nil
}

func readPage(request *jobsv1.ListJobsRequest) (jobs.Page, error) {
	size := int(request.GetPageSize())
	if size < 0 || size > maxPageSize {
		return jobs.Page{}, status.Error(codes.InvalidArgument, "page_size must be between 0 and 100")
	}
	if size == 0 {
		size = defaultPageSize
	}

	page := jobs.Page{Limit: size}
	token := request.GetPageToken()
	if token == "" {
		return page, nil
	}
	if len(token) > 256 {
		return jobs.Page{}, status.Error(codes.InvalidArgument, "invalid page_token")
	}

	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return jobs.Page{}, status.Error(codes.InvalidArgument, "invalid page_token")
	}

	var cursor pageCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.CreatedAt.IsZero() || cursor.ID <= 0 {
		return jobs.Page{}, status.Error(codes.InvalidArgument, "invalid page_token")
	}

	page.AfterCreatedAt = cursor.CreatedAt
	page.AfterID = cursor.ID

	return page, nil
}

func writePageToken(job jobs.Job) (string, error) {
	data, err := json.Marshal(pageCursor{
		CreatedAt: job.CreatedAt,
		ID:        job.ID,
	})
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(data), nil
}
