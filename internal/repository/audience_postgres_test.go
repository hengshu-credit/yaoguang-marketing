package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudienceCompilerUsesParametersForEveryLeaf(t *testing.T) {
	expression := domain.AudienceExpression{Operator: domain.AudienceOperatorExclusion, Children: []domain.AudienceExpression{
		{Operator: domain.AudienceOperatorIntersection, Children: []domain.AudienceExpression{{LeafType: domain.AudienceLeafList, RefID: "list-user-value"}, {LeafType: domain.AudienceLeafSegment, RefID: "segment-user-value"}}},
		{LeafType: domain.AudienceLeafAudience, RefID: "11111111-1111-4111-8111-111111111111"},
	}}
	query, args, err := compileAudienceExpression(expression)
	require.NoError(t, err)
	assert.Contains(t, query, "$1")
	assert.Contains(t, query, "$2")
	assert.Contains(t, query, "$3")
	assert.NotContains(t, query, "list-user-value")
	assert.Equal(t, []interface{}{"list-user-value", "segment-user-value", "11111111-1111-4111-8111-111111111111"}, args)
}

func TestAudienceCompilerOffsetsParametersForBuildInsert(t *testing.T) {
	query, args, err := compileAudienceExpressionWithOffset(domain.AudienceExpression{LeafType: domain.AudienceLeafList, RefID: "list-1"}, 1)
	require.NoError(t, err)
	assert.Contains(t, query, "$2")
	assert.Equal(t, []interface{}{"list-1"}, args)
}

func TestAudienceCompilerUsesConditionCompilerAtCurrentPlaceholderOffset(t *testing.T) {
	tree := &domain.TreeNode{Kind: "leaf", Leaf: &domain.TreeNodeLeaf{
		Source: "contacts",
		Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{{
			FieldName: "profile_status", FieldType: "string", Operator: "equals", StringValues: []string{"unpaid"},
		}}},
	}}
	calledOffset := -1
	conditionCompiler := func(got *domain.TreeNode, offset int) (string, []interface{}, error) {
		assert.Same(t, tree, got)
		calledOffset = offset
		return "SELECT customer_id FROM contacts WHERE profile_status = $3", []interface{}{"unpaid"}, nil
	}

	query, args, err := compileAudienceExpressionWithConditionCompiler(
		domain.AudienceExpression{Operator: domain.AudienceOperatorIntersection, Children: []domain.AudienceExpression{
			{LeafType: domain.AudienceLeafList, RefID: "due-list"},
			{Condition: tree},
		}},
		1,
		conditionCompiler,
	)
	require.NoError(t, err)
	assert.Equal(t, 2, calledOffset)
	assert.Contains(t, query, "profile_status = $3")
	assert.Equal(t, []interface{}{"due-list", "unpaid"}, args)
}

func TestAudienceCompilerRejectsConditionWithoutServerCompiler(t *testing.T) {
	_, _, err := compileAudienceExpression(domain.AudienceExpression{Condition: &domain.TreeNode{
		Kind: "leaf", Leaf: &domain.TreeNodeLeaf{Source: "contacts", Contact: &domain.ContactCondition{Filters: []*domain.DimensionFilter{{
			FieldName: "country", FieldType: "string", Operator: "equals", StringValues: []string{"CN"},
		}}}},
	}})
	assert.ErrorContains(t, err, "condition compiler is required")
}

func TestBuildAudienceSnapshotMaterializesConditionInOneTransactionWithoutActivatingBuild(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	audienceID := "11111111-1111-4111-8111-111111111111"
	createdAt := time.Date(2026, 8, 31, 1, 2, 3, 0, time.UTC)
	definition := `{"condition":{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"profile_status","field_type":"string","operator":"equals","string_values":["unpaid"]}]}}}}`
	mock.ExpectQuery(`SELECT audience_id, version, definition, definition_hash, created_at`).
		WithArgs(audienceID, 3).
		WillReturnRows(sqlmock.NewRows([]string{"audience_id", "version", "definition", "definition_hash", "created_at"}).
			AddRow(audienceID, 3, []byte(definition), "hash", createdAt))
	mock.ExpectExec(`INSERT INTO audience_builds`).
		WithArgs(sqlmock.AnyArg(), audienceID, 3, createdAt).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE audience_builds SET status = 'building'`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audience_memberships`).
		WithArgs(sqlmock.AnyArg(), "unpaid").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`UPDATE audience_builds SET status = 'completed'`).
		WithArgs(sqlmock.AnyArg(), int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repository := NewAudienceRepositoryWithDB(db)
	repository.SetConditionCompiler(func(_ *domain.TreeNode, offset int) (string, []interface{}, error) {
		assert.Equal(t, 1, offset)
		return "SELECT contacts.customer_id FROM contacts WHERE profile_status = $2", []interface{}{"unpaid"}, nil
	})
	repository.now = func() time.Time { return createdAt }

	build, err := repository.BuildAudienceSnapshot(context.Background(), "workspace-1", audienceID, 3)
	require.NoError(t, err)
	assert.Equal(t, "completed", build.Status)
	assert.Equal(t, int64(2), build.MemberCount)
	assert.Equal(t, audienceID, build.AudienceID)
	assert.Equal(t, 3, build.AudienceVersion)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMatchesAudienceCustomerUsesNamedVersionAndCurrentFacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	audienceID := "11111111-1111-4111-8111-111111111111"
	customerID := "22222222-2222-4222-8222-222222222222"
	createdAt := time.Date(2026, 8, 31, 1, 2, 3, 0, time.UTC)
	definition := `{"condition":{"kind":"leaf","leaf":{"source":"contacts","contact":{"filters":[{"field_name":"profile_status","field_type":"string","operator":"equals","string_values":["unpaid"]}]}}}}`
	mock.ExpectQuery(`SELECT audience_id, version, definition, definition_hash, created_at`).
		WithArgs(audienceID, 4).
		WillReturnRows(sqlmock.NewRows([]string{"audience_id", "version", "definition", "definition_hash", "created_at"}).
			AddRow(audienceID, 4, []byte(definition), "hash", createdAt))
	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM \(`).
		WithArgs("unpaid", customerID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	repository := NewAudienceRepositoryWithDB(db)
	repository.SetConditionCompiler(func(_ *domain.TreeNode, offset int) (string, []interface{}, error) {
		assert.Zero(t, offset)
		return "SELECT contacts.customer_id FROM contacts WHERE profile_status = $1", []interface{}{"unpaid"}, nil
	})

	matches, err := repository.MatchesAudienceCustomer(context.Background(), "workspace-1", audienceID, 4, customerID)
	require.NoError(t, err)
	assert.True(t, matches)
	require.NoError(t, mock.ExpectationsWereMet())
}
