package repository

import (
	"testing"

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
