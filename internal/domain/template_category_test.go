package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTemplateCategoryDefinitionValidation(t *testing.T) {
	valid := TemplateCategoryDefinition{
		ID: "vip_offer", Name: "VIP Offer", Purpose: TemplateCategoryPurposeMarketing,
		SortOrder: 25, IsActive: true,
	}
	require.NoError(t, valid.Validate())

	tests := []struct {
		name     string
		category TemplateCategoryDefinition
		want     string
	}{
		{"invalid id", TemplateCategoryDefinition{ID: "VIP Offer", Name: "VIP", Purpose: TemplateCategoryPurposeMarketing}, "lowercase"},
		{"missing name", TemplateCategoryDefinition{ID: "vip", Purpose: TemplateCategoryPurposeMarketing}, "name is required"},
		{"invalid purpose", TemplateCategoryDefinition{ID: "vip", Name: "VIP", Purpose: "other"}, "purpose"},
		{"invalid order", TemplateCategoryDefinition{ID: "vip", Name: "VIP", Purpose: TemplateCategoryPurposeMarketing, SortOrder: 10001}, "sort_order"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorContains(t, tt.category.Validate(), tt.want)
		})
	}
}

func TestBuiltInTemplateCategoriesPreserveCompliancePurpose(t *testing.T) {
	categories := BuiltInTemplateCategories()
	require.Len(t, categories, 9)
	byID := make(map[string]TemplateCategoryDefinition, len(categories))
	for _, category := range categories {
		require.NoError(t, category.Validate())
		require.True(t, category.IsSystem)
		byID[category.ID] = category
	}
	require.Equal(t, TemplateCategoryPurposeMarketing, byID["marketing"].Purpose)
	require.Equal(t, TemplateCategoryPurposeMarketing, byID["blog"].Purpose)
	require.Equal(t, TemplateCategoryPurposeTransactional, byID["transactional"].Purpose)
	require.Equal(t, TemplateCategoryPurposeTransactional, byID["other"].Purpose)
}

func TestEffectiveTemplateCategoryPurposeUsesResolvedValueAndLegacyFallback(t *testing.T) {
	require.Equal(t, TemplateCategoryPurposeMarketing, EffectiveTemplateCategoryPurpose("custom", TemplateCategoryPurposeMarketing))
	require.Equal(t, TemplateCategoryPurposeMarketing, EffectiveTemplateCategoryPurpose("blog", ""))
	require.Equal(t, TemplateCategoryPurposeTransactional, EffectiveTemplateCategoryPurpose("custom", ""))
}

func TestTemplateCategoryMutationRequestsValidateWorkspaceAndImmutableFields(t *testing.T) {
	require.NoError(t, (&CreateTemplateCategoryRequest{
		WorkspaceID: "ws1", ID: "vip_offer", Name: "VIP Offer",
		Purpose: TemplateCategoryPurposeMarketing, SortOrder: 20,
	}).Validate())
	require.ErrorContains(t, (&CreateTemplateCategoryRequest{WorkspaceID: "ws1", ID: "bad id", Name: "Bad", Purpose: TemplateCategoryPurposeMarketing}).Validate(), "lowercase")
	require.NoError(t, (&UpdateTemplateCategoryRequest{WorkspaceID: "ws1", ID: "vip_offer", Name: "VIP", SortOrder: 30, IsActive: true}).Validate())
	require.ErrorContains(t, (&DeleteTemplateCategoryRequest{WorkspaceID: "", ID: "vip_offer"}).Validate(), "workspace_id")
}
