package domain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelCatalogIncludesTargetChannelsWithUsableCapabilities(t *testing.T) {
	definitions := ListChannelDefinitions()
	require.NotEmpty(t, definitions)

	byID := make(map[string]ChannelDefinition, len(definitions))
	for _, definition := range definitions {
		require.NotEmpty(t, definition.ID)
		require.NotEmpty(t, definition.LabelKey, definition.ID)
		require.NotEmpty(t, definition.ContentFamilies, definition.ID)
		require.NotEmpty(t, definition.PreviewProfiles, definition.ID)
		require.False(t, byID[definition.ID].ID != "", "duplicate channel id %s", definition.ID)
		byID[definition.ID] = definition
	}

	requiredChannels := []string{
		ChannelEmail, ChannelWeb, ChannelSMS, ChannelPush,
		"in_app", "rcs", "webhook",
		"wechat_official_account", "wechat_mini_program", "wecom", "dingtalk", "feishu",
		"whatsapp", "telegram", "line", "zalo", "viber", "messenger", "instagram", "kakao",
	}
	for _, channelID := range requiredChannels {
		_, ok := byID[channelID]
		assert.True(t, ok, "expected channel %s in catalogue", channelID)
	}
}

func TestRecommendedChannelIDsReturnsMarketSpecificOpenMessagingChannels(t *testing.T) {
	tests := []struct {
		country string
		want    []string
	}{
		{country: "CN", want: []string{"wechat_official_account", "wechat_mini_program", "wecom", "dingtalk", "feishu", "push", "rcs"}},
		{country: "KZ", want: []string{"whatsapp", "telegram"}},
		{country: "UZ", want: []string{"telegram", "whatsapp"}},
		{country: "PH", want: []string{"messenger", "viber", "whatsapp"}},
		{country: "TH", want: []string{"line", "messenger", "whatsapp"}},
		{country: "VN", want: []string{"zalo", "messenger", "viber", "whatsapp"}},
		{country: "ID", want: []string{"whatsapp", "telegram", "messenger"}},
		{country: "MX", want: []string{"whatsapp", "rcs", "messenger", "instagram"}},
		{country: "PE", want: []string{"whatsapp", "messenger", "instagram"}},
		{country: "PK", want: []string{"whatsapp", "telegram"}},
	}

	for _, test := range tests {
		t.Run(test.country, func(t *testing.T) {
			assert.Equal(t, test.want, RecommendedChannelIDs(test.country))
			assert.Equal(t, test.want, RecommendedChannelIDs(" "+test.country+" "))
			assert.Equal(t, test.want, RecommendedChannelIDs(strings.ToLower(test.country)))
		})
	}
	assert.Empty(t, RecommendedChannelIDs("ZZ"))
}

func TestListChannelDefinitionsReturnsDefensiveCopies(t *testing.T) {
	first := ListChannelDefinitions()
	require.NotEmpty(t, first)
	require.NotEmpty(t, first[0].PreviewProfiles)

	originalID := first[0].ID
	originalProfileID := first[0].PreviewProfiles[0].ID
	first[0].ID = "mutated"
	first[0].PreviewProfiles[0].ID = "mutated"

	second := ListChannelDefinitions()
	assert.Equal(t, originalID, second[0].ID)
	assert.Equal(t, originalProfileID, second[0].PreviewProfiles[0].ID)
}
