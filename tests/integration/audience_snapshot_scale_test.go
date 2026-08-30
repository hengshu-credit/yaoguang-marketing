package integration

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/repository"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAudienceSnapshotScaleIntegration is safe for ordinary CI at 10,001
// rows. Capacity runs can set YAOGUANG_SCALE_CUSTOMERS=1000000 on the reference
// hardware without changing the test or taking a different code path.
func TestAudienceSnapshotScaleIntegration(t *testing.T) {
	fixture := newDeliveryIntegrationFixture(t)
	db, err := fixture.suite.DBManager.GetWorkspaceDB(fixture.workspaceID)
	require.NoError(t, err)
	total := 10_001
	if configured := os.Getenv("YAOGUANG_SCALE_CUSTOMERS"); configured != "" {
		value, parseErr := strconv.Atoi(configured)
		require.NoError(t, parseErr)
		require.Positive(t, value)
		total = value
	}
	// List identifiers are intentionally capped at 32 characters by the
	// workspace schema. Keep the fixture realistic so this test exercises the
	// audience build path instead of failing during data setup.
	listID := "scale_" + uuid.NewString()[:24]
	_, err = db.Exec(`INSERT INTO lists (id, name, is_double_optin, is_public) VALUES ($1, 'Scale audience', FALSE, FALSE)`, listID)
	require.NoError(t, err)
	_, err = db.Exec(`WITH generated AS (
		SELECT value,
			(substr(md5($2 || value::text), 1, 8) || '-' || substr(md5($2 || value::text), 9, 4) || '-4' || substr(md5($2 || value::text), 14, 3) || '-8' || substr(md5($2 || value::text), 18, 3) || '-' || substr(md5($2 || value::text), 21, 12))::uuid AS customer_id
		FROM generate_series(1, $1) value
	), customers_inserted AS (
		INSERT INTO customers (id, customer_no, external_user_id)
		SELECT customer_id, 'U0001scale' || lpad(value::text, 8, '0'), $2 || value::text FROM generated
		RETURNING id
	) INSERT INTO customer_list_memberships (customer_id, list_id, status)
	SELECT id, $3, 'active' FROM customers_inserted`, total, fixture.workspaceID+"-scale-", listID)
	require.NoError(t, err)

	repo := repository.NewAudienceRepositoryWithDB(db)
	svc, err := service.NewAudienceService(repo)
	require.NoError(t, err)
	audience, err := svc.Create(context.Background(), service.CreateAudienceRequest{WorkspaceID: fixture.workspaceID,
		Name: "Scale audience", Kind: domain.AudienceKindStatic,
		Definition: domain.AudienceExpression{LeafType: domain.AudienceLeafList, RefID: listID}})
	require.NoError(t, err)
	build, err := repo.StartAudienceBuild(context.Background(), fixture.workspaceID, audience.ID, audience.ActiveVersion)
	require.NoError(t, err)
	chunks := 0
	for {
		build, completed, chunkErr := repo.ProcessAudienceBuildChunk(context.Background(), fixture.workspaceID, build.ID, 2_000)
		require.NoError(t, chunkErr)
		chunks++
		if completed {
			assert.Equal(t, int64(total), build.MemberCount)
			break
		}
	}
	assert.Greater(t, chunks, 1, "the test must exercise keyset resume across transactions")
	var persisted, duplicates int64
	require.NoError(t, db.QueryRow(`SELECT COUNT(*), COUNT(*) - COUNT(DISTINCT customer_id)
		FROM audience_memberships WHERE build_id = $1`, build.ID).Scan(&persisted, &duplicates))
	assert.Equal(t, int64(total), persisted)
	assert.Zero(t, duplicates)
	status, err := repo.GetAudienceBuild(context.Background(), fixture.workspaceID, build.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", status.Status)
	assert.NotEmpty(t, status.LastCustomerID, fmt.Sprintf("checkpoint missing after %d rows", total))

	campaignID := uuid.NewString()
	now := time.Now().UTC()
	campaignRepo := repository.NewCampaignRepositoryWithDB(db)
	require.NoError(t, campaignRepo.CreateCampaign(context.Background(), fixture.workspaceID,
		domain.Campaign{ID: campaignID, Name: "Scale campaign", Status: domain.CampaignStatusDraft,
			DraftVersion: 1, CreatedAt: now, UpdatedAt: now},
		domain.CampaignVersion{CampaignID: campaignID, Version: 1, AudienceID: audience.ID,
			AudienceVersion: audience.ActiveVersion, Channel: "email", CreatedAt: now,
			Variants: []domain.CampaignVariant{{ID: "control", WeightBP: 5000}, {ID: "experiment", WeightBP: 5000}}}))
	snapshotService, err := service.NewCampaignSnapshotService(campaignRepo, 5_000)
	require.NoError(t, err)
	snapshotStartedAt := time.Now()
	run, err := snapshotService.Start(context.Background(), fixture.workspaceID, campaignID, 1)
	require.NoError(t, err)
	t.Logf("materialized %d immutable campaign recipients in %s", total, time.Since(snapshotStartedAt))
	assert.Equal(t, "dispatching", run.Status)
	assert.Equal(t, int64(total), run.SnapshotCount)
	var snapshotCount, snapshotDuplicates int64
	require.NoError(t, db.QueryRow(`SELECT COUNT(*), COUNT(*) - COUNT(DISTINCT customer_id)
		FROM campaign_recipient_snapshots WHERE run_id = $1`, run.ID).Scan(&snapshotCount, &snapshotDuplicates))
	assert.Equal(t, int64(total), snapshotCount)
	assert.Zero(t, snapshotDuplicates)
}
