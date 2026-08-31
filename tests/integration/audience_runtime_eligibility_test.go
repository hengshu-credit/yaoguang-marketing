package integration

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/repository"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudienceRuntimeEligibilityKeepsCandidateButRejectsPaidCustomerAtTouch(t *testing.T) {
	fixture := newDeliveryIntegrationFixture(t)
	db, err := fixture.suite.DBManager.GetWorkspaceDB(fixture.workspaceID)
	require.NoError(t, err)

	customerID := uuid.NewString()
	email := "repayment-" + uuid.NewString() + "@example.com"
	_, err = db.Exec(`INSERT INTO customers (id, customer_no) VALUES ($1, $2)`, customerID, "repayment-"+customerID[:8])
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO contacts (email, customer_id) VALUES ($1, $2)`, email, customerID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO contact_profiles (email, status) VALUES ($1, 'unpaid')`, email)
	require.NoError(t, err)

	audienceRepository := repository.NewAudienceRepositoryWithDB(db)
	queryBuilder := service.NewQueryBuilder()
	audienceRepository.SetConditionCompiler(queryBuilder.BuildCustomerIDSQL)
	audienceService, err := service.NewAudienceService(audienceRepository)
	require.NoError(t, err)
	audience, err := audienceService.Create(context.Background(), service.CreateAudienceRequest{
		WorkspaceID: fixture.workspaceID,
		Name:        "Unpaid customers",
		Kind:        domain.AudienceKindDynamic,
		Definition: domain.AudienceExpression{Condition: &domain.TreeNode{
			Kind: "leaf",
			Leaf: &domain.TreeNodeLeaf{Source: "contacts", Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{{
				FieldName: "profile_status", FieldType: "string", Operator: "equals", StringValues: []string{"unpaid"},
			}}}},
		}},
	})
	require.NoError(t, err)
	build, err := audienceRepository.BuildAudienceSnapshot(context.Background(), fixture.workspaceID, audience.ID, audience.ActiveVersion)
	require.NoError(t, err)
	assert.Equal(t, int64(1), build.MemberCount)

	_, err = db.Exec(`UPDATE contact_profiles SET status = 'paid', updated_at = CURRENT_TIMESTAMP WHERE email = $1`, email)
	require.NoError(t, err)
	matches, err := audienceRepository.MatchesAudienceCustomer(
		context.Background(), fixture.workspaceID, audience.ID, audience.ActiveVersion, customerID,
	)
	require.NoError(t, err)
	assert.False(t, matches, "a customer who repaid after snapshot must fail the touch-time check")

	var stillCandidate bool
	require.NoError(t, db.QueryRow(`SELECT EXISTS (SELECT 1 FROM audience_memberships WHERE build_id = $1 AND customer_id = $2)`, build.ID, customerID).Scan(&stillCandidate))
	assert.True(t, stillCandidate, "the immutable candidate snapshot must remain auditable")
}
