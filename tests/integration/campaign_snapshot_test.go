package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/repository"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCampaignSnapshotIntegration(t *testing.T) {
	fixture := newDeliveryIntegrationFixture(t)
	db, err := fixture.suite.DBManager.GetWorkspaceDB(fixture.workspaceID)
	require.NoError(t, err)
	listID := "snapshot-list"
	_, err = db.Exec(`INSERT INTO lists (id, name, is_double_optin, is_public) VALUES ($1, 'Snapshot list', FALSE, FALSE)`, listID)
	require.NoError(t, err)
	for ordinal := 1; ordinal <= 5; ordinal++ {
		customerID := uuid.New().String()
		_, err = db.Exec(`INSERT INTO customers (id, customer_no, external_user_id) VALUES ($1, $2, $3)`, customerID, fmt.Sprintf("U0001snapshot%02d", ordinal), fmt.Sprintf("snapshot-%d", ordinal))
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO customer_list_memberships (customer_id, list_id, status) VALUES ($1, $2, 'active')`, customerID, listID)
		require.NoError(t, err)
	}

	audienceRepo := repository.NewAudienceRepositoryWithDB(db)
	audienceService, err := service.NewAudienceService(audienceRepo)
	require.NoError(t, err)
	audience, err := audienceService.Create(context.Background(), service.CreateAudienceRequest{WorkspaceID: fixture.workspaceID,
		Name: "Snapshot audience", Kind: domain.AudienceKindStatic, Definition: domain.AudienceExpression{LeafType: domain.AudienceLeafList, RefID: listID}})
	require.NoError(t, err)
	buildID, members, err := audienceService.Build(context.Background(), fixture.workspaceID, audience.ID, 1)
	require.NoError(t, err)
	assert.NotEmpty(t, buildID)
	assert.Equal(t, int64(5), members)

	campaignID := uuid.New().String()
	now := time.Now().UTC()
	campaignVersion := domain.CampaignVersion{CampaignID: campaignID, Version: 1, AudienceID: audience.ID, AudienceVersion: 1,
		Channel: "email", Variants: []domain.CampaignVariant{{ID: "template-a", WeightBP: 5000}, {ID: "template-b", WeightBP: 5000}}, CreatedAt: now}
	campaignRepo := repository.NewCampaignRepositoryWithDB(db)
	require.NoError(t, campaignRepo.CreateCampaign(context.Background(), fixture.workspaceID,
		domain.Campaign{ID: campaignID, Name: "Frozen campaign", Status: domain.CampaignStatusDraft, DraftVersion: 1, CreatedAt: now}, campaignVersion))
	snapshotService, err := service.NewCampaignSnapshotService(campaignRepo, 2)
	require.NoError(t, err)
	run, err := snapshotService.Start(context.Background(), fixture.workspaceID, campaignID, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(5), run.SnapshotCount)

	_, err = db.Exec(`DELETE FROM customer_list_memberships WHERE list_id = $1`, listID)
	require.NoError(t, err)
	var persisted int64
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM campaign_recipient_snapshots WHERE run_id = $1`, run.ID).Scan(&persisted))
	assert.Equal(t, int64(5), persisted, "started campaign membership must remain immutable after list changes")
}
