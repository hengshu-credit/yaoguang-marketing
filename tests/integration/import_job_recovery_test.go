package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportJobRecoveryIntegration(t *testing.T) {
	fixture := newDeliveryIntegrationFixture(t)
	db, err := fixture.suite.DBManager.GetWorkspaceDB(fixture.workspaceID)
	require.NoError(t, err)
	repo := repository.NewImportJobRepositoryWithDB(db)
	jobID := uuid.New().String()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateImportJob(context.Background(), fixture.workspaceID, domain.ImportJob{
		ID: jobID, Status: domain.ImportJobUploading, Filename: "large.csv", CreatedAt: now,
	}))
	for start := 1; start <= 10_001; start += 2_000 {
		end := start + 1_999
		if end > 10_001 {
			end = 10_001
		}
		rows := make([]domain.ImportJobRow, 0, end-start+1)
		for ordinal := start; ordinal <= end; ordinal++ {
			payload, _ := json.Marshal(map[string]string{"external_user_id": fmt.Sprintf("external-%d", ordinal)})
			digest := sha256.Sum256(payload)
			rows = append(rows, domain.ImportJobRow{JobID: jobID, Ordinal: int64(ordinal), RawPayload: payload,
				Checksum: hex.EncodeToString(digest[:]), Status: domain.ImportRowPending})
		}
		inserted, stageErr := repo.StageImportRows(context.Background(), fixture.workspaceID, jobID, rows)
		require.NoError(t, stageErr)
		assert.Equal(t, int64(len(rows)), inserted)
	}
	require.NoError(t, repo.CommitImportJob(context.Background(), fixture.workspaceID, jobID, fmt.Sprintf("%064x", 1)))
	job, err := repo.GetImportJob(context.Background(), fixture.workspaceID, jobID)
	require.NoError(t, err)
	assert.Equal(t, int64(10_001), job.Counters.Total)
	require.NoError(t, job.Counters.Validate())

	first, _, err := repo.ClaimImportRows(context.Background(), fixture.workspaceID, jobID, 2_000, time.Millisecond)
	require.NoError(t, err)
	require.Len(t, first, 2_000)
	time.Sleep(5 * time.Millisecond)
	replayed, _, err := repo.ClaimImportRows(context.Background(), fixture.workspaceID, jobID, 2_000, time.Minute)
	require.NoError(t, err)
	require.Len(t, replayed, 2_000)
	assert.Equal(t, first[0].Ordinal, replayed[0].Ordinal)
	job, err = repo.GetImportJob(context.Background(), fixture.workspaceID, jobID)
	require.NoError(t, err)
	assert.Equal(t, int64(8_001), job.Counters.Pending)
	assert.Equal(t, int64(2_000), job.Counters.Processing, "lease replay must not double count processing rows")
	require.NoError(t, job.Counters.Validate())
}
