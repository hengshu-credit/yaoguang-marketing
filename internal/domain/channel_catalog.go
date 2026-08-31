package domain

import (
	"context"
	"strings"
)

type ContentFamily string

const (
	ContentFamilyText             ContentFamily = "text"
	ContentFamilyNotification     ContentFamily = "notification"
	ContentFamilyRichCard         ContentFamily = "rich_card"
	ContentFamilyCarousel         ContentFamily = "carousel"
	ContentFamilyExternalTemplate ContentFamily = "external_template"
	ContentFamilyWorkMessage      ContentFamily = "work_message"
	ContentFamilyWebhookPayload   ContentFamily = "webhook_payload"
)

const (
	ChannelDeliveryModeNative        = "native"
	ChannelDeliveryModeSignedWebhook = "signed_webhook"
)

type PreviewProfile struct {
	ID       string `json:"id"`
	LabelKey string `json:"label_key"`
	Surface  string `json:"surface"`
}

type ChannelLimits struct {
	MaxTitleRunes int `json:"max_title_runes,omitempty"`
	MaxBodyRunes  int `json:"max_body_runes,omitempty"`
	MaxActions    int `json:"max_actions,omitempty"`
	MaxCards      int `json:"max_cards,omitempty"`
	MaxPayload    int `json:"max_payload_bytes,omitempty"`
}

type ChannelDefinition struct {
	ID              string           `json:"id"`
	LabelKey        string           `json:"label_key"`
	Regions         []string         `json:"regions,omitempty"`
	RecommendedIn   []string         `json:"recommended_in,omitempty"`
	ContentFamilies []ContentFamily  `json:"content_families"`
	PreviewProfiles []PreviewProfile `json:"preview_profiles"`
	DeliveryModes   []string         `json:"delivery_modes"`
	Limits          ChannelLimits    `json:"limits"`
}

type ChannelCatalogService interface {
	List(context.Context, string) ([]ChannelDefinition, error)
}

func profile(id, label, surface string) PreviewProfile {
	return PreviewProfile{ID: id, LabelKey: label, Surface: surface}
}

var channelDefinitions = []ChannelDefinition{
	{ID: ChannelEmail, LabelKey: "Email", Regions: []string{"global"}, ContentFamilies: []ContentFamily{ContentFamilyRichCard}, PreviewProfiles: []PreviewProfile{profile("email_mobile", "Mobile email", "mobile"), profile("email_desktop", "Desktop email", "desktop")}, DeliveryModes: []string{ChannelDeliveryModeNative}, Limits: ChannelLimits{MaxTitleRunes: 255}},
	{ID: ChannelWeb, LabelKey: "Web", Regions: []string{"global"}, ContentFamilies: []ContentFamily{ContentFamilyRichCard}, PreviewProfiles: []PreviewProfile{profile("web_page", "Web page", "web")}, DeliveryModes: []string{ChannelDeliveryModeNative}},
	{ID: ChannelSMS, LabelKey: "SMS", Regions: []string{"global"}, ContentFamilies: []ContentFamily{ContentFamilyText}, PreviewProfiles: []PreviewProfile{profile("sms_ios", "iOS Messages", "mobile"), profile("sms_android", "Google Messages", "mobile")}, DeliveryModes: []string{ChannelDeliveryModeNative}, Limits: ChannelLimits{MaxBodyRunes: 10000}},
	{ID: ChannelPush, LabelKey: "Push", Regions: []string{"global", "china"}, RecommendedIn: []string{"CN"}, ContentFamilies: []ContentFamily{ContentFamilyNotification}, PreviewProfiles: []PreviewProfile{
		profile("ios_lock_screen", "iOS lock screen", "mobile"), profile("android_notification", "Android notification", "mobile"), profile("web_notification", "Web notification", "desktop"),
		profile("huawei_notification", "Huawei notification", "mobile"), profile("honor_notification", "Honor notification", "mobile"), profile("xiaomi_notification", "Xiaomi notification", "mobile"), profile("oppo_notification", "OPPO notification", "mobile"), profile("vivo_notification", "vivo notification", "mobile"),
	}, DeliveryModes: []string{ChannelDeliveryModeNative}, Limits: ChannelLimits{MaxTitleRunes: 512, MaxBodyRunes: 4096, MaxPayload: 4096}},
	{ID: "in_app", LabelKey: "In-App", Regions: []string{"global"}, ContentFamilies: []ContentFamily{ContentFamilyNotification, ContentFamilyRichCard, ContentFamilyCarousel}, PreviewProfiles: []PreviewProfile{profile("in_app_ios", "iOS app", "mobile"), profile("in_app_android", "Android app", "mobile"), profile("in_app_web", "Web app", "web")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxTitleRunes: 512, MaxBodyRunes: 10000, MaxActions: 10, MaxCards: 10, MaxPayload: 131072}},
	{ID: "rcs", LabelKey: "RCS", Regions: []string{"global", "china", "latam"}, RecommendedIn: []string{"CN", "MX"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard, ContentFamilyCarousel}, PreviewProfiles: []PreviewProfile{profile("google_messages_rcs", "Google Messages RCS", "mobile")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxTitleRunes: 200, MaxBodyRunes: 3072, MaxActions: 4, MaxCards: 10, MaxPayload: 131072}},
	{ID: "webhook", LabelKey: "Webhook", Regions: []string{"global"}, ContentFamilies: []ContentFamily{ContentFamilyWebhookPayload}, PreviewProfiles: []PreviewProfile{profile("http_request", "HTTP request", "developer")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxPayload: 131072}},
	{ID: "wechat_official_account", LabelKey: "WeChat Official Account", Regions: []string{"china"}, RecommendedIn: []string{"CN"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard, ContentFamilyExternalTemplate}, PreviewProfiles: []PreviewProfile{profile("wechat_mobile", "WeChat mobile", "mobile")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxTitleRunes: 200, MaxBodyRunes: 10000, MaxActions: 5, MaxPayload: 131072}},
	{ID: "wechat_mini_program", LabelKey: "WeChat Mini Program", Regions: []string{"china"}, RecommendedIn: []string{"CN"}, ContentFamilies: []ContentFamily{ContentFamilyExternalTemplate}, PreviewProfiles: []PreviewProfile{profile("wechat_mini_program", "WeChat Mini Program", "mobile")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxPayload: 131072}},
	{ID: "wecom", LabelKey: "WeCom", Regions: []string{"china"}, RecommendedIn: []string{"CN"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard, ContentFamilyWorkMessage}, PreviewProfiles: []PreviewProfile{profile("wecom_mobile", "WeCom mobile", "mobile"), profile("wecom_desktop", "WeCom desktop", "desktop")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxTitleRunes: 512, MaxBodyRunes: 10000, MaxActions: 10, MaxPayload: 131072}},
	{ID: "dingtalk", LabelKey: "DingTalk", Regions: []string{"china"}, RecommendedIn: []string{"CN"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard, ContentFamilyWorkMessage}, PreviewProfiles: []PreviewProfile{profile("dingtalk_mobile", "DingTalk mobile", "mobile"), profile("dingtalk_desktop", "DingTalk desktop", "desktop")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxTitleRunes: 512, MaxBodyRunes: 10000, MaxActions: 10, MaxPayload: 131072}},
	{ID: "feishu", LabelKey: "Feishu / Lark", Regions: []string{"china", "global"}, RecommendedIn: []string{"CN"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard, ContentFamilyWorkMessage}, PreviewProfiles: []PreviewProfile{profile("feishu_mobile", "Feishu mobile", "mobile"), profile("feishu_desktop", "Feishu desktop", "desktop")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxTitleRunes: 512, MaxBodyRunes: 10000, MaxActions: 10, MaxPayload: 131072}},
	{ID: "whatsapp", LabelKey: "WhatsApp", Regions: []string{"global", "central_asia", "southeast_asia", "latam", "south_asia"}, RecommendedIn: []string{"KZ", "UZ", "PH", "TH", "VN", "ID", "MX", "PE", "PK"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard, ContentFamilyExternalTemplate}, PreviewProfiles: []PreviewProfile{profile("whatsapp_ios", "WhatsApp iOS", "mobile"), profile("whatsapp_android", "WhatsApp Android", "mobile"), profile("whatsapp_web", "WhatsApp Web", "desktop")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxTitleRunes: 512, MaxBodyRunes: 4096, MaxActions: 10, MaxPayload: 131072}},
	{ID: "telegram", LabelKey: "Telegram", Regions: []string{"global", "central_asia", "southeast_asia", "south_asia"}, RecommendedIn: []string{"KZ", "UZ", "ID", "PK"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard}, PreviewProfiles: []PreviewProfile{profile("telegram_mobile", "Telegram mobile", "mobile"), profile("telegram_desktop", "Telegram desktop", "desktop")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxBodyRunes: 4096, MaxActions: 100, MaxPayload: 131072}},
	{ID: "line", LabelKey: "LINE", Regions: []string{"southeast_asia", "east_asia"}, RecommendedIn: []string{"TH"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard, ContentFamilyCarousel}, PreviewProfiles: []PreviewProfile{profile("line_mobile", "LINE mobile", "mobile"), profile("line_desktop", "LINE desktop", "desktop")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxTitleRunes: 512, MaxBodyRunes: 10000, MaxActions: 10, MaxCards: 10, MaxPayload: 131072}},
	{ID: "zalo", LabelKey: "Zalo", Regions: []string{"southeast_asia"}, RecommendedIn: []string{"VN"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard, ContentFamilyExternalTemplate}, PreviewProfiles: []PreviewProfile{profile("zalo_mobile", "Zalo mobile", "mobile")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxTitleRunes: 512, MaxBodyRunes: 10000, MaxActions: 5, MaxPayload: 131072}},
	{ID: "viber", LabelKey: "Viber", Regions: []string{"global", "southeast_asia"}, RecommendedIn: []string{"PH", "VN"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard}, PreviewProfiles: []PreviewProfile{profile("viber_mobile", "Viber mobile", "mobile"), profile("viber_desktop", "Viber desktop", "desktop")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxBodyRunes: 7000, MaxActions: 6, MaxPayload: 131072}},
	{ID: "messenger", LabelKey: "Messenger", Regions: []string{"global", "southeast_asia", "latam"}, RecommendedIn: []string{"PH", "TH", "VN", "ID", "MX", "PE"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard, ContentFamilyCarousel}, PreviewProfiles: []PreviewProfile{profile("messenger_mobile", "Messenger mobile", "mobile"), profile("messenger_web", "Messenger Web", "desktop")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxTitleRunes: 80, MaxBodyRunes: 2000, MaxActions: 3, MaxCards: 10, MaxPayload: 131072}},
	{ID: "instagram", LabelKey: "Instagram Messaging", Regions: []string{"global", "latam"}, RecommendedIn: []string{"MX", "PE"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard}, PreviewProfiles: []PreviewProfile{profile("instagram_mobile", "Instagram mobile", "mobile"), profile("instagram_web", "Instagram Web", "desktop")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxBodyRunes: 1000, MaxActions: 3, MaxPayload: 131072}},
	{ID: "kakao", LabelKey: "KakaoTalk", Regions: []string{"east_asia"}, ContentFamilies: []ContentFamily{ContentFamilyText, ContentFamilyRichCard, ContentFamilyExternalTemplate}, PreviewProfiles: []PreviewProfile{profile("kakao_mobile", "KakaoTalk mobile", "mobile")}, DeliveryModes: []string{ChannelDeliveryModeSignedWebhook}, Limits: ChannelLimits{MaxTitleRunes: 200, MaxBodyRunes: 1000, MaxActions: 5, MaxPayload: 131072}},
}

var recommendedChannelPacks = map[string][]string{
	"CN": {"wechat_official_account", "wechat_mini_program", "wecom", "dingtalk", "feishu", ChannelPush, "rcs"},
	"KZ": {"whatsapp", "telegram"},
	"UZ": {"telegram", "whatsapp"},
	"PH": {"messenger", "viber", "whatsapp"},
	"TH": {"line", "messenger", "whatsapp"},
	"VN": {"zalo", "messenger", "viber", "whatsapp"},
	"ID": {"whatsapp", "telegram", "messenger"},
	"MX": {"whatsapp", "rcs", "messenger", "instagram"},
	"PE": {"whatsapp", "messenger", "instagram"},
	"PK": {"whatsapp", "telegram"},
}

func cloneChannelDefinition(source ChannelDefinition) ChannelDefinition {
	cloned := source
	cloned.Regions = append([]string(nil), source.Regions...)
	cloned.RecommendedIn = append([]string(nil), source.RecommendedIn...)
	cloned.ContentFamilies = append([]ContentFamily(nil), source.ContentFamilies...)
	cloned.PreviewProfiles = append([]PreviewProfile(nil), source.PreviewProfiles...)
	cloned.DeliveryModes = append([]string(nil), source.DeliveryModes...)
	return cloned
}

func ListChannelDefinitions() []ChannelDefinition {
	definitions := make([]ChannelDefinition, len(channelDefinitions))
	for index, definition := range channelDefinitions {
		definitions[index] = cloneChannelDefinition(definition)
	}
	return definitions
}

func FindChannelDefinition(channelID string) (ChannelDefinition, bool) {
	channelID = strings.ToLower(strings.TrimSpace(channelID))
	for _, definition := range channelDefinitions {
		if definition.ID == channelID {
			return cloneChannelDefinition(definition), true
		}
	}
	return ChannelDefinition{}, false
}

func IsRegisteredChannel(channelID string) bool {
	_, ok := FindChannelDefinition(channelID)
	return ok
}

func RecommendedChannelIDs(country string) []string {
	ids := recommendedChannelPacks[strings.ToUpper(strings.TrimSpace(country))]
	return append([]string(nil), ids...)
}
