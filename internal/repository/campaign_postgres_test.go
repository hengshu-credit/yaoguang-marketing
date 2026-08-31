package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListCampaignMembersUsesOnlyActiveListMemberships(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT membership.customer_id, ''[[:space:]]+FROM customer_list_memberships membership[[:space:]]+WHERE membership.list_id = \$1 AND membership.status = 'active'`).
		WithArgs("news", "", 100).
		WillReturnRows(sqlmock.NewRows([]string{"customer_id", "build_id"}).
			AddRow("11111111-1111-4111-8111-111111111111", ""))
	repository := NewCampaignRepositoryWithDB(db)

	members, next, err := repository.ListCampaignMembers(context.Background(), "workspace-1", domain.CampaignVersion{ListID: "news"}, "", "", 100)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Empty(t, members[0].BuildID)
	assert.Equal(t, members[0].CustomerID, next)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListCampaignMembersUsesTheRunExactAudienceBuild(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	buildID := "22222222-2222-4222-8222-222222222222"
	mock.ExpectQuery(`SELECT membership.customer_id, membership.build_id[[:space:]]+FROM audience_memberships membership[[:space:]]+WHERE membership.build_id = NULLIF\(\$1, ''\)::uuid`).
		WithArgs(buildID, "", 100).
		WillReturnRows(sqlmock.NewRows([]string{"customer_id", "build_id"}).
			AddRow("11111111-1111-4111-8111-111111111111", buildID))
	repository := NewCampaignRepositoryWithDB(db)

	members, _, err := repository.ListCampaignMembers(context.Background(), "workspace-1", domain.CampaignVersion{
		AudienceID: "audience-1", AudienceVersion: 7,
	}, buildID, "", 100)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, buildID, members[0].BuildID)
	require.NoError(t, mock.ExpectationsWereMet())
}
