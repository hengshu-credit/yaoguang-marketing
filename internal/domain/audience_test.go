package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudienceExpressionCanonicalVersionIsStable(t *testing.T) {
	expression := AudienceExpression{Operator: AudienceOperatorExclusion, Children: []AudienceExpression{
		{Operator: AudienceOperatorUnion, Children: []AudienceExpression{{LeafType: AudienceLeafList, RefID: "list-a"}, {LeafType: AudienceLeafSegment, RefID: "segment-b"}}},
		{LeafType: AudienceLeafAudience, RefID: "audience-c"},
	}}
	first, err := expression.VersionHash()
	require.NoError(t, err)
	second, err := expression.VersionHash()
	require.NoError(t, err)
	assert.Len(t, first, 64)
	assert.Equal(t, first, second)
}

func TestAudienceExpressionRejectsSQLAndMalformedTrees(t *testing.T) {
	assert.Error(t, (AudienceExpression{LeafType: AudienceLeafList, RefID: ""}).Validate())
	assert.Error(t, (AudienceExpression{Operator: AudienceOperatorExclusion, Children: []AudienceExpression{{LeafType: AudienceLeafList, RefID: "a"}}}).Validate())
	canonical, err := (AudienceExpression{LeafType: AudienceLeafSegment, RefID: "segment-1"}).CanonicalJSON()
	require.NoError(t, err)
	assert.NotContains(t, string(canonical), "sql")
}

func TestAudienceExpressionAcceptsExactlyOneConditionLeaf(t *testing.T) {
	tree := &TreeNode{
		Kind: "leaf",
		Leaf: &TreeNodeLeaf{
			Source: "contacts",
			Contact: &ContactCondition{Filters: []*DimensionFilter{{
				FieldName: "profile_status", FieldType: "string", Operator: "equals", StringValues: []string{"unpaid"},
			}}},
		},
	}

	expression := AudienceExpression{Condition: tree}
	require.NoError(t, expression.Validate())
	hash, err := expression.VersionHash()
	require.NoError(t, err)
	assert.Len(t, hash, 64)

	assert.ErrorContains(t, (AudienceExpression{
		Condition: tree,
		LeafType:  AudienceLeafList,
		RefID:     "repayment-list",
	}).Validate(), "exactly one")
	assert.ErrorContains(t, (AudienceExpression{
		Condition: tree,
		Operator:  AudienceOperatorUnion,
		Children: []AudienceExpression{
			{LeafType: AudienceLeafList, RefID: "one"},
			{LeafType: AudienceLeafList, RefID: "two"},
		},
	}).Validate(), "exactly one")
}

func TestAudienceExpressionRejectsInvalidConditionTree(t *testing.T) {
	expression := AudienceExpression{Condition: &TreeNode{Kind: "leaf", Leaf: &TreeNodeLeaf{Source: "contacts"}}}
	assert.ErrorContains(t, expression.Validate(), "condition")
}

func TestAudienceMemberQueryValidateEnforcesSourceAndFilterSemantics(t *testing.T) {
	joinedAfter := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	joinedBefore := joinedAfter.Add(24 * time.Hour)

	valid := AudienceMemberQuery{
		ListID: "newsletter", Status: "active", EventName: "purchase",
		JoinedAfter: &joinedAfter, JoinedBefore: &joinedBefore,
		AttributeKey: "tier", AttributeValue: "gold",
	}
	require.NoError(t, valid.Validate())
	assert.Equal(t, DefaultAudienceMemberLimit, valid.Limit)

	tests := []struct {
		name    string
		query   AudienceMemberQuery
		message string
	}{
		{name: "missing source", query: AudienceMemberQuery{}, message: "exactly one member source"},
		{name: "ambiguous source", query: AudienceMemberQuery{ListID: "a", AudienceID: uuid.NewString()}, message: "exactly one member source"},
		{name: "invalid status", query: AudienceMemberQuery{ListID: "a", Status: "paused"}, message: "subscription status"},
		{name: "dynamic join time", query: AudienceMemberQuery{AudienceID: uuid.NewString(), JoinedAfter: &joinedAfter}, message: "join time filters require a list"},
		{name: "attribute value without key", query: AudienceMemberQuery{ListID: "a", AttributeValue: "gold"}, message: "attribute key and value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.query.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.message)
		})
	}
}
