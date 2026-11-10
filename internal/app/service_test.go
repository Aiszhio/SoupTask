package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"SoupWebApplication/internal/jobs"
	jobsv1 "SoupWebApplication/pkg/api/jobs/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type fakeStore struct {
	job     jobs.Job
	items   []jobs.Job
	hasMore bool
	err     error

	key     string
	payload []byte
	id      int64
	page    jobs.Page
}

func (store *fakeStore) Create(_ context.Context, key string, payload []byte) (jobs.Job, error) {
	store.key = key
	store.payload = payload
	return store.job, store.err
}

func (store *fakeStore) Get(_ context.Context, id int64) (jobs.Job, error) {
	store.id = id
	return store.job, store.err
}

func (store *fakeStore) List(_ context.Context, page jobs.Page) ([]jobs.Job, bool, error) {
	store.page = page
	return store.items, store.hasMore, store.err
}

func (store *fakeStore) Cancel(_ context.Context, id int64) (jobs.Job, error) {
	store.id = id
	return store.job, store.err
}

func TestServiceRequests(t *testing.T) {
	tests := []struct {
		name     string
		call     func(*Service) error
		storeErr error
		wantCode codes.Code
	}{
		{
			name: "create without request",
			call: func(service *Service) error {
				_, err := service.CreateJob(context.Background(), nil)
				return err
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "create without key",
			call: func(service *Service) error {
				_, err := service.CreateJob(context.Background(), &jobsv1.CreateJobRequest{})
				return err
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "create",
			call: func(service *Service) error {
				_, err := service.CreateJob(context.Background(), &jobsv1.CreateJobRequest{IdempotencyKey: "key-1"})
				return err
			},
			wantCode: codes.OK,
		},
		{
			name: "get with invalid ID",
			call: func(service *Service) error {
				_, err := service.GetJob(context.Background(), &jobsv1.GetJobRequest{JobId: "abc"})
				return err
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "get missing job",
			call: func(service *Service) error {
				_, err := service.GetJob(context.Background(), &jobsv1.GetJobRequest{JobId: "12"})
				return err
			},
			storeErr: jobs.ErrNotFound,
			wantCode: codes.NotFound,
		},
		{
			name: "get",
			call: func(service *Service) error {
				_, err := service.GetJob(context.Background(), &jobsv1.GetJobRequest{JobId: "12"})
				return err
			},
			wantCode: codes.OK,
		},
		{
			name: "list without request",
			call: func(service *Service) error {
				_, err := service.ListJobs(context.Background(), nil)
				return err
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "list with negative page size",
			call: func(service *Service) error {
				_, err := service.ListJobs(context.Background(), &jobsv1.ListJobsRequest{PageSize: -1})
				return err
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "list with oversized page",
			call: func(service *Service) error {
				_, err := service.ListJobs(context.Background(), &jobsv1.ListJobsRequest{PageSize: 101})
				return err
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "list with invalid token",
			call: func(service *Service) error {
				_, err := service.ListJobs(context.Background(), &jobsv1.ListJobsRequest{PageToken: "bad"})
				return err
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "list",
			call: func(service *Service) error {
				_, err := service.ListJobs(context.Background(), &jobsv1.ListJobsRequest{})
				return err
			},
			wantCode: codes.OK,
		},
		{
			name: "cancel with invalid ID",
			call: func(service *Service) error {
				_, err := service.CancelJob(context.Background(), &jobsv1.CancelJobRequest{JobId: "0"})
				return err
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "cancel",
			call: func(service *Service) error {
				_, err := service.CancelJob(context.Background(), &jobsv1.CancelJobRequest{JobId: "12"})
				return err
			},
			wantCode: codes.OK,
		},
		{
			name: "storage error",
			call: func(service *Service) error {
				_, err := service.GetJob(context.Background(), &jobsv1.GetJobRequest{JobId: "12"})
				return err
			},
			storeErr: errors.New("database unavailable"),
			wantCode: codes.Internal,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeStore{
				job: jobs.Job{ID: 12, Status: jobs.Queued, CreatedAt: time.Now()},
				err: test.storeErr,
			}
			service := NewService(store)

			if got := status.Code(test.call(service)); got != test.wantCode {
				t.Fatalf("code = %s, want %s", got, test.wantCode)
			}
		})
	}
}

func TestServiceResults(t *testing.T) {
	createdAt := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		job: jobs.Job{ID: 12, Status: jobs.Queued, CreatedAt: createdAt},
	}
	service := NewService(store)

	payload, err := structpb.NewStruct(map[string]any{"value": "test"})
	if err != nil {
		t.Fatal(err)
	}

	created, err := service.CreateJob(context.Background(), &jobsv1.CreateJobRequest{
		IdempotencyKey: "key-1",
		Payload:        payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Job.GetId() != "12" || created.Job.GetStatus() != jobsv1.JobStatus_JOB_STATUS_QUEUED {
		t.Fatalf("created job = %v", created.Job)
	}
	if store.key != "key-1" || string(store.payload) != `{"value":"test"}` {
		t.Fatalf("stored key and payload = %q, %q", store.key, store.payload)
	}

	got, err := service.GetJob(context.Background(), &jobsv1.GetJobRequest{JobId: "12"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Job.GetId() != "12" || store.id != 12 {
		t.Fatalf("got job = %v, requested ID = %d", got.Job, store.id)
	}

	store.job.Status = jobs.Canceled
	canceled, err := service.CancelJob(context.Background(), &jobsv1.CancelJobRequest{JobId: "12"})
	if err != nil {
		t.Fatal(err)
	}
	if canceled.Job.GetStatus() != jobsv1.JobStatus_JOB_STATUS_CANCELED {
		t.Fatalf("canceled job = %v", canceled.Job)
	}
}

func TestListJobsPagination(t *testing.T) {
	createdAt := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		items:   []jobs.Job{{ID: 12, Status: jobs.Running, CreatedAt: createdAt}},
		hasMore: true,
	}
	service := NewService(store)

	first, err := service.ListJobs(context.Background(), &jobsv1.ListJobsRequest{PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Jobs) != 1 || first.Jobs[0].GetStatus() != jobsv1.JobStatus_JOB_STATUS_RUNNING {
		t.Fatalf("first page = %v", first.Jobs)
	}
	if first.NextPageToken == "" {
		t.Fatal("missing next page token")
	}

	store.hasMore = false
	_, err = service.ListJobs(context.Background(), &jobsv1.ListJobsRequest{
		PageSize:  1,
		PageToken: first.NextPageToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.page.AfterID != 12 || !store.page.AfterCreatedAt.Equal(createdAt) {
		t.Fatalf("page cursor = %+v", store.page)
	}
}
