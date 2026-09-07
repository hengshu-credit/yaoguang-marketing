package domain

import "encoding/json"

// CampaignConfiguration is edited in place. Executions copy it into their own
// immutable snapshot; CampaignVersion only adapts historical API requests.
type CampaignConfiguration struct {
	AudienceID      string            `json:"audience_id,omitempty"`
	AudienceVersion int               `json:"audience_version,omitempty"`
	ListID          string            `json:"list_id,omitempty"`
	Channel         string            `json:"channel"`
	Variants        []CampaignVariant `json:"variants"`
	Settings        json.RawMessage   `json:"settings,omitempty"`
}

func (c CampaignConfiguration) LegacyAdapter(id string) CampaignVersion {
	return CampaignVersion{CampaignID: id, Version: 1, AudienceID: c.AudienceID, AudienceVersion: c.AudienceVersion, ListID: c.ListID, Channel: c.Channel, Variants: c.Variants}
}
func (c CampaignConfiguration) Validate() error { return c.LegacyAdapter("configuration").Validate() }
