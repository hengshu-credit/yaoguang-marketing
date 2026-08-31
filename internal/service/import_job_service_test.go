package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type importJobRepositoryMemory struct {
	job  domain.ImportJob
	rows map[int64]domain.ImportJobRow
}

type importCustomerServiceStub struct {
	domain.CustomerService
	requests []*domain.CustomerBatchUpsertRequest
}

func (s *importCustomerServiceStub) UpsertCustomerBatch(_ context.Context, request *domain.CustomerBatchUpsertRequest) (*domain.CustomerBatchUpsertResponse, error) {
	s.requests = append(s.requests, request)
	response := &domain.CustomerBatchUpsertResponse{Accepted: len(request.Items), Results: make([]domain.CustomerBatchItemResult, len(request.Items))}
	for index := range request.Items {
		response.Results[index] = domain.CustomerBatchItemResult{Index: index, Status: "accepted", Customer: &domain.CustomerMutationResult{CustomerID: fmt.Sprintf("customer-%d", index), Action: "created"}}
	}
	return response, nil
}

type importTaskSchedulerMemory struct{ tasks []*domain.Task }

func (s *importTaskSchedulerMemory) CreateTask(_ context.Context, _ string, task *domain.Task) error {
	s.tasks = append(s.tasks, task)
	return nil
}

func (r *importJobRepositoryMemory) CreateImportJob(_ context.Context, _ string, job domain.ImportJob) error {
	r.job, r.rows = job, map[int64]domain.ImportJobRow{}
	return nil
}
func (r *importJobRepositoryMemory) StageImportRows(_ context.Context, _, _ string, rows []domain.ImportJobRow) (int64, error) {
	var inserted int64
	for _, row := range rows {
		if _, exists := r.rows[row.Ordinal]; exists {
			continue
		}
		r.rows[row.Ordinal] = row
		r.job.Counters.Total++
		if row.Status == domain.ImportRowFailed {
			r.job.Counters.Failed++
		} else {
			r.job.Counters.Pending++
		}
		inserted++
	}
	return inserted, nil
}
func (r *importJobRepositoryMemory) CommitImportJob(_ context.Context, _, _, checksum string) error {
	r.job.FileChecksum = checksum
	if r.job.Counters.Pending == 0 {
		r.job.Status = domain.ImportJobCompleted
	} else {
		r.job.Status = domain.ImportJobStaged
	}
	return nil
}
func (r *importJobRepositoryMemory) RejectImportJob(_ context.Context, _, _, _ string) error {
	for ordinal, row := range r.rows {
		if row.Status == domain.ImportRowPending {
			row.Status = domain.ImportRowFailed
			row.ErrorCode = "job_rejected"
			r.rows[ordinal] = row
			r.job.Counters.Pending--
			r.job.Counters.Failed++
		}
	}
	r.job.Status = domain.ImportJobRejected
	return nil
}
func (r *importJobRepositoryMemory) ClaimImportRows(_ context.Context, _, _ string, limit int, _ time.Duration) ([]domain.ImportJobRow, string, error) {
	claimed := make([]domain.ImportJobRow, 0, limit)
	for ordinal := int64(1); len(claimed) < limit; ordinal++ {
		row, exists := r.rows[ordinal]
		if !exists {
			if ordinal > r.job.Counters.Total {
				break
			}
			continue
		}
		if row.Status != domain.ImportRowPending {
			continue
		}
		row.Status = domain.ImportRowProcessing
		r.rows[ordinal] = row
		r.job.Counters.Pending--
		r.job.Counters.Processing++
		claimed = append(claimed, row)
	}
	return claimed, "claim-token", nil
}
func (r *importJobRepositoryMemory) CompleteImportRow(_ context.Context, _, _ string, ordinal int64, _ string, status domain.ImportRowStatus, customerID, action, errorCode string) error {
	row := r.rows[ordinal]
	row.Status, row.CustomerID, row.Action, row.ErrorCode = status, customerID, action, errorCode
	r.rows[ordinal] = row
	r.job.Counters.Processing--
	if status == domain.ImportRowSucceeded {
		r.job.Counters.Succeeded++
	} else {
		r.job.Counters.Failed++
	}
	return nil
}
func (r *importJobRepositoryMemory) GetImportJob(context.Context, string, string) (*domain.ImportJob, error) {
	copy := r.job
	return &copy, nil
}
func (r *importJobRepositoryMemory) ListImportJobs(context.Context, string, int, int) ([]domain.ImportJob, int, error) {
	return []domain.ImportJob{r.job}, 1, nil
}
func (r *importJobRepositoryMemory) CancelImportJob(context.Context, string, string, string) error {
	for ordinal, row := range r.rows {
		if row.Status == domain.ImportRowPending || row.Status == domain.ImportRowProcessing {
			row.Status, row.ErrorCode = domain.ImportRowFailed, "cancelled_by_user"
			r.rows[ordinal] = row
			r.job.Counters.Failed++
		}
	}
	r.job.Counters.Pending, r.job.Counters.Processing = 0, 0
	r.job.Status = domain.ImportJobCancelled
	return nil
}
func (r *importJobRepositoryMemory) ListImportJobErrors(context.Context, string, string, int, int) ([]domain.ImportJobRow, int, error) {
	items := make([]domain.ImportJobRow, 0)
	for _, row := range r.rows {
		if row.Status == domain.ImportRowFailed {
			items = append(items, row)
		}
	}
	return items, len(items), nil
}

func TestImportJobStagesEveryRowBeforeRejectingConfiguredLimit(t *testing.T) {
	repository := &importJobRepositoryMemory{}
	service, err := NewImportJobService(ImportJobServiceDependencies{Repository: repository, MaxRows: 10_000, ChunkSize: 2_000, MaxFileBytes: 1 << 30})
	require.NoError(t, err)
	var csv strings.Builder
	csv.WriteString("external_user_id,email\n")
	for index := 1; index <= 10_001; index++ {
		_, _ = fmt.Fprintf(&csv, "user-%d,user-%d@example.com\n", index, index)
	}
	job, err := service.StageCSV(context.Background(), "workspace1", "customers.csv", nil, strings.NewReader(csv.String()))
	require.NoError(t, err)
	assert.Equal(t, domain.ImportJobRejected, job.Status)
	assert.Len(t, repository.rows, 10_001, "no accepted upload row may disappear")
	assert.Equal(t, int64(10_001), job.Counters.Total)
	assert.Equal(t, int64(10_001), job.Counters.Failed)
	require.NoError(t, job.Counters.Validate())
}

func TestImportJobPersistsMalformedRowsAsExplicitFailures(t *testing.T) {
	repository := &importJobRepositoryMemory{}
	service, err := NewImportJobService(ImportJobServiceDependencies{Repository: repository, MaxRows: 100, ChunkSize: 2, MaxFileBytes: 1 << 20})
	require.NoError(t, err)
	job, err := service.StageCSV(context.Background(), "workspace1", "customers.csv", nil, strings.NewReader("external_user_id,email\na,a@example.com\nbad-row\nc,c@example.com\n"))
	require.NoError(t, err)
	assert.Equal(t, domain.ImportJobStaged, job.Status)
	assert.Len(t, repository.rows, 3)
	assert.Equal(t, domain.ImportRowFailed, repository.rows[2].Status)
	assert.Equal(t, "csv_parse_error", repository.rows[2].ErrorCode)
	assert.Equal(t, int64(3), job.Counters.Total)
	require.NoError(t, job.Counters.Validate())
}

func TestImportJobCommitCreatesDurableBackgroundTask(t *testing.T) {
	repository := &importJobRepositoryMemory{}
	scheduler := &importTaskSchedulerMemory{}
	service, err := NewImportJobService(ImportJobServiceDependencies{Repository: repository, Tasks: scheduler, MaxRows: 100, ChunkSize: 20, MaxFileBytes: 1 << 20})
	require.NoError(t, err)
	job, err := service.StageCSV(context.Background(), "workspace1", "customers.csv", nil, strings.NewReader("external_user_id\ncustomer-1\n"))
	require.NoError(t, err)
	require.Equal(t, domain.ImportJobStaged, job.Status)
	require.Len(t, scheduler.tasks, 1)
	task := scheduler.tasks[0]
	assert.Equal(t, job.ID, task.ID, "job id makes task creation idempotent")
	assert.Equal(t, domain.ImportCustomersTaskType, task.Type)
	require.NotNil(t, task.State.ImportCustomers)
	assert.Equal(t, job.ID, task.State.ImportCustomers.JobID)
	assert.Equal(t, int64(1), task.State.ImportCustomers.TotalRows)
}

func TestImportJobAddsCustomersToSelectedListsWithoutReplacementSemantics(t *testing.T) {
	repository := &importJobRepositoryMemory{}
	customers := &importCustomerServiceStub{}
	service, err := NewImportJobService(ImportJobServiceDependencies{Repository: repository, Customers: customers, MaxRows: 100, ChunkSize: 20, MaxFileBytes: 1 << 20})
	require.NoError(t, err)

	job, err := service.StageCSV(context.Background(), "workspace1", "customers.csv", []string{" listB ", "listA", "listB"}, strings.NewReader("external_user_id\ncustomer-1\n"))
	require.NoError(t, err)
	assert.Equal(t, []string{"listB", "listA"}, job.ListIDs)

	processed, err := service.ProcessNextChunk(context.Background(), "workspace1", job.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, customers.requests, 1)
	additions := customers.requests[0].Items[0].Customer.ListMembershipsAdd
	require.NotNil(t, additions)
	assert.Equal(t, []domain.CustomerListMembershipInput{
		{ListID: "listB", Status: "active"},
		{ListID: "listA", Status: "active"},
	}, *additions)
	assert.Nil(t, customers.requests[0].Items[0].Customer.ListMemberships)
}
