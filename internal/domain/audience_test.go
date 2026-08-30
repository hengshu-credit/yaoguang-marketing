package domain

import (
	"testing"

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
