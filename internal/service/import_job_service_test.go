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
func (r *importJobRepositoryMemory) ClaimImportRows(context.Context, string, string, int, time.Duration) ([]domain.ImportJobRow, string, error) {
	return nil, "", nil
}
func (r *importJobRepositoryMemory) CompleteImportRow(context.Context, string, string, int64, string, domain.ImportRowStatus, string, string, string) error {
	return nil
}
func (r *importJobRepositoryMemory) GetImportJob(context.Context, string, string) (*domain.ImportJob, error) {
	copy := r.job
	return &copy, nil
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
	job, err := service.StageCSV(context.Background(), "workspace1", "customers.csv", strings.NewReader(csv.String()))
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
	job, err := service.StageCSV(context.Background(), "workspace1", "customers.csv", strings.NewReader("external_user_id,email\na,a@example.com\nbad-row\nc,c@example.com\n"))
	require.NoError(t, err)
	assert.Equal(t, domain.ImportJobStaged, job.Status)
	assert.Len(t, repository.rows, 3)
	assert.Equal(t, domain.ImportRowFailed, repository.rows[2].Status)
	assert.Equal(t, "csv_parse_error", repository.rows[2].ErrorCode)
	assert.Equal(t, int64(3), job.Counters.Total)
	require.NoError(t, job.Counters.Validate())
}
