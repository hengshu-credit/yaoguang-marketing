package domain

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
	"time"
)

type CampaignStatus string

const (
	CampaignStatusDraft     CampaignStatus = "draft"
	CampaignStatusScheduled CampaignStatus = "scheduled"
	CampaignStatusRunning   CampaignStatus = "running"
	CampaignStatusPaused    CampaignStatus = "paused"
	CampaignStatusCompleted CampaignStatus = "completed"
	CampaignStatusCancelled CampaignStatus = "cancelled"
)

type Campaign struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Status        CampaignStatus `json:"status"`
	DraftVersion  int            `json:"draft_version"`
	ActiveVersion int            `json:"active_version,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type CampaignVariant struct {
	ID       string `json:"id"`
	WeightBP int    `json:"weight_bp"`
}

type CampaignVersion struct {
	CampaignID      string            `json:"campaign_id"`
	Version         int               `json:"version"`
	AudienceID      string            `json:"audience_id"`
	AudienceVersion int               `json:"audience_version"`
	ListID          string            `json:"list_id,omitempty"`
	Channel         string            `json:"channel"`
	Variants        []CampaignVariant `json:"variants"`
	ActivatedAt     *time.Time        `json:"activated_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

type CampaignRun struct {
	ID                     string    `json:"id"`
	CampaignID             string    `json:"campaign_id"`
	CampaignVersion        int       `json:"campaign_version"`
	AudienceID             string    `json:"audience_id,omitempty"`
	AudienceVersion        int       `json:"audience_version,omitempty"`
	AudienceBuildID        string    `json:"audience_build_id,omitempty"`
	Status                 string    `json:"status"`
	RunSeed                string    `json:"run_seed"`
	SnapshotLastCustomerID string    `json:"snapshot_last_customer_id,omitempty"`
	SnapshotCount          int64     `json:"snapshot_count"`
	NextOrdinal            int64     `json:"next_ordinal"`
	CreatedAt              time.Time `json:"created_at"`
}

type CampaignRecipientSnapshot struct {
	RunID         string    `json:"run_id"`
	Ordinal       int64     `json:"ordinal"`
	CustomerID    string    `json:"customer_id"`
	Variant       string    `json:"variant"`
	SourceBuildID string    `json:"source_build_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type CampaignAudienceMember struct {
	CustomerID string `json:"customer_id"`
	BuildID    string `json:"build_id"`
}

type CampaignRepository interface {
	CreateCampaign(context.Context, string, Campaign, CampaignVersion) error
	GetCampaign(context.Context, string, string) (*Campaign, error)
	ListCampaigns(context.Context, string, int, int) ([]Campaign, int, error)
	GetCampaignVersion(context.Context, string, string, int) (*CampaignVersion, error)
	CreateCampaignRun(context.Context, string, CampaignRun) error
	GetCampaignRun(context.Context, string, string) (*CampaignRun, error)
	ListCampaignMembers(context.Context, string, CampaignVersion, string, string, int) ([]CampaignAudienceMember, string, error)
	AppendCampaignSnapshots(context.Context, string, string, []CampaignRecipientSnapshot) (int64, error)
	CompleteCampaignSnapshot(context.Context, string, string, int64) error
	ListCampaignSnapshots(context.Context, string, string, int64, int) ([]CampaignRecipientSnapshot, int64, error)
}

func (v CampaignVersion) Validate() error {
	if strings.TrimSpace(v.CampaignID) == "" || v.Version <= 0 {
		return errors.New("campaign and version are required")
	}
	v.AudienceID = strings.TrimSpace(v.AudienceID)
	v.ListID = strings.TrimSpace(v.ListID)
	audienceFieldsPresent := v.AudienceID != "" || v.AudienceVersion != 0
	audienceSource := v.AudienceID != "" && v.AudienceVersion > 0
	listSource := v.ListID != ""
	if (audienceFieldsPresent && !audienceSource) || audienceSource == listSource {
		return errors.New("campaign version requires exactly one recipient source: a versioned audience or a list")
	}
	if len(v.ListID) > 32 {
		return errors.New("campaign list id must contain at most 32 characters")
	}
	if strings.TrimSpace(v.Channel) == "" || len(v.Variants) == 0 {
		return errors.New("campaign channel and variants are required")
	}
	total := 0
	seen := map[string]struct{}{}
	for _, variant := range v.Variants {
		if strings.TrimSpace(variant.ID) == "" || variant.WeightBP <= 0 {
			return errors.New("campaign variant id and positive weight are required")
		}
		if _, exists := seen[variant.ID]; exists {
			return errors.New("campaign variant ids must be unique")
		}
		seen[variant.ID] = struct{}{}
		total += variant.WeightBP
	}
	if total != 10_000 {
		return errors.New("campaign variant weights must total 10000 basis points")
	}
	return nil
}

func (v CampaignVersion) EnsureMutable() error {
	if v.ActivatedAt != nil {
		return errors.New("activated campaign version is immutable")
	}
	return nil
}

func (v CampaignVersion) AssignVariant(customerID, runSeed string) (string, error) {
	if err := v.Validate(); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(customerID + "\x00" + runSeed))
	position := int(binary.BigEndian.Uint64(digest[:8]) % 10_000)
	cumulative := 0
	for _, variant := range v.Variants {
		cumulative += variant.WeightBP
		if position < cumulative {
			return variant.ID, nil
		}
	}
	return "", errors.New("campaign variant assignment failed")
}
