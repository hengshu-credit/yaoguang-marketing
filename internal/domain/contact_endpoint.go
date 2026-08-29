package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ChannelPush = "push"

	EndpointOperationUpsert  = "upsert"
	EndpointOperationDisable = "disable"

	PushProviderFCM        = "fcm"
	PushProviderAPNS       = "apns"
	PushProviderWebPush    = "webpush"
	EndpointProviderTwilio = "twilio"

	EndpointPlatformAndroid = "android"
	EndpointPlatformIOS     = "ios"
	EndpointPlatformWeb     = "web"
	EndpointPlatformPhone   = "phone"
)

var endpointLocalePattern = regexp.MustCompile(`^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$`)
var endpointE164Pattern = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

// ContactEndpoint is a provider address owned by one marketing contact. Address
// is intentionally excluded from JSON so API responses and logs cannot leak a
// device token or browser subscription.
type ContactEndpoint struct {
	EndpointID string                 `json:"endpoint_id"`
	Email      string                 `json:"email"`
	Channel    string                 `json:"channel"`
	Provider   string                 `json:"provider"`
	Platform   string                 `json:"platform"`
	Address    string                 `json:"-"`
	Locale     string                 `json:"locale,omitempty"`
	Timezone   string                 `json:"timezone,omitempty"`
	AppID      string                 `json:"app_id,omitempty"`
	DeviceID   string                 `json:"device_id,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Enabled    bool                   `json:"enabled"`
	Version    int64                  `json:"version"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	LastSeenAt time.Time              `json:"last_seen_at"`
}

func (e *ContactEndpoint) MarshalPublicJSON() ([]byte, error) {
	return json.Marshal(e)
}

// ContactEndpointMutation is embedded in an ingest contact item. Upsert carries
// a current provider address; disable only needs the stable endpoint id.
type ContactEndpointMutation struct {
	Operation  string                 `json:"operation"`
	EndpointID string                 `json:"endpoint_id"`
	Channel    string                 `json:"channel,omitempty"`
	Provider   string                 `json:"provider,omitempty"`
	Platform   string                 `json:"platform,omitempty"`
	Address    string                 `json:"address,omitempty"`
	Locale     string                 `json:"locale,omitempty"`
	Timezone   string                 `json:"timezone,omitempty"`
	AppID      string                 `json:"app_id,omitempty"`
	DeviceID   string                 `json:"device_id,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

func (m ContactEndpointMutation) Validate() (*ContactEndpoint, error) {
	m.Operation = strings.TrimSpace(strings.ToLower(m.Operation))
	m.EndpointID = strings.TrimSpace(m.EndpointID)
	if m.EndpointID == "" || utf8.RuneCountInString(m.EndpointID) > 128 {
		return nil, fmt.Errorf("endpoint_id must contain 1 to 128 characters")
	}
	if m.Operation != EndpointOperationUpsert && m.Operation != EndpointOperationDisable {
		return nil, fmt.Errorf("endpoint operation must be upsert or disable")
	}
	if m.Operation == EndpointOperationDisable {
		return &ContactEndpoint{EndpointID: m.EndpointID, Enabled: false}, nil
	}

	m.Channel = strings.TrimSpace(strings.ToLower(m.Channel))
	m.Provider = strings.TrimSpace(strings.ToLower(m.Provider))
	m.Platform = strings.TrimSpace(strings.ToLower(m.Platform))
	m.Address = strings.TrimSpace(m.Address)
	m.Locale = strings.TrimSpace(m.Locale)
	m.Timezone = strings.TrimSpace(m.Timezone)
	m.AppID = strings.TrimSpace(m.AppID)
	m.DeviceID = strings.TrimSpace(m.DeviceID)
	if m.Channel != ChannelPush && m.Channel != ChannelSMS {
		return nil, fmt.Errorf("endpoint channel must be sms or push")
	}
	if m.Address == "" || utf8.RuneCountInString(m.Address) > 4096 {
		return nil, fmt.Errorf("endpoint address must contain 1 to 4096 characters")
	}
	switch m.Provider {
	case EndpointProviderTwilio:
		if m.Channel != ChannelSMS || m.Platform != EndpointPlatformPhone {
			return nil, fmt.Errorf("provider twilio requires sms channel and phone platform")
		}
		if !endpointE164Pattern.MatchString(m.Address) {
			return nil, fmt.Errorf("twilio sms address must be in E.164 format")
		}
	case PushProviderAPNS:
		if m.Channel != ChannelPush || m.Platform != EndpointPlatformIOS {
			return nil, fmt.Errorf("provider apns requires platform ios")
		}
	case PushProviderFCM:
		if m.Channel != ChannelPush || (m.Platform != EndpointPlatformAndroid && m.Platform != EndpointPlatformIOS) {
			return nil, fmt.Errorf("provider fcm requires platform android or ios")
		}
	case PushProviderWebPush:
		if m.Channel != ChannelPush || m.Platform != EndpointPlatformWeb {
			return nil, fmt.Errorf("provider webpush requires platform web")
		}
	default:
		return nil, fmt.Errorf("endpoint provider must be twilio, fcm, apns, or webpush")
	}
	if m.Locale != "" && !endpointLocalePattern.MatchString(m.Locale) {
		return nil, fmt.Errorf("invalid endpoint locale: %s", m.Locale)
	}
	if m.Timezone != "" && !IsValidTimezone(m.Timezone) {
		return nil, fmt.Errorf("invalid endpoint timezone: %s", m.Timezone)
	}
	if utf8.RuneCountInString(m.AppID) > 255 {
		return nil, fmt.Errorf("app_id cannot exceed 255 characters")
	}
	if utf8.RuneCountInString(m.DeviceID) > 255 {
		return nil, fmt.Errorf("device_id cannot exceed 255 characters")
	}
	if m.Attributes != nil {
		encoded, err := json.Marshal(m.Attributes)
		if err != nil {
			return nil, fmt.Errorf("invalid endpoint attributes: %w", err)
		}
		if len(encoded) > 16*1024 {
			return nil, fmt.Errorf("endpoint attributes cannot exceed 16 KiB")
		}
	}

	return &ContactEndpoint{
		EndpointID: m.EndpointID, Channel: m.Channel, Provider: m.Provider,
		Platform: m.Platform, Address: m.Address, Locale: m.Locale,
		Timezone: m.Timezone, AppID: m.AppID, DeviceID: m.DeviceID,
		Attributes: m.Attributes, Enabled: true,
	}, nil
}

type ContactEndpointRepository interface {
	Upsert(ctx context.Context, workspaceID, email string, endpoint *ContactEndpoint) error
	Disable(ctx context.Context, workspaceID, email, endpointID string) error
	ListActiveByEmail(ctx context.Context, workspaceID, email, channel string) ([]*ContactEndpoint, error)
}

type ListContactEndpointsRequest struct {
	WorkspaceID string `json:"workspace_id"`
	Email       string `json:"email"`
	Channel     string `json:"channel,omitempty"`
}

func (r *ListContactEndpointsRequest) Validate() error {
	if r == nil {
		return fmt.Errorf("request is required")
	}
	r.WorkspaceID = strings.TrimSpace(r.WorkspaceID)
	if r.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	r.Email = NormalizeEmail(r.Email)
	contact := &Contact{Email: r.Email}
	if err := contact.Validate(); err != nil {
		return fmt.Errorf("invalid email")
	}
	r.Email = contact.Email
	r.Channel = strings.TrimSpace(strings.ToLower(r.Channel))
	if r.Channel == "" {
		r.Channel = ChannelPush
	}
	if r.Channel != ChannelPush && r.Channel != ChannelSMS {
		return fmt.Errorf("channel must be sms or push")
	}
	return nil
}

type ContactEndpointService interface {
	List(ctx context.Context, request *ListContactEndpointsRequest) ([]*ContactEndpoint, error)
}
