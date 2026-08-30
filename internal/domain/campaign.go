package domain

import (
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
	Channel         string            `json:"channel"`
	Variants        []CampaignVariant `json:"variants"`
	ActivatedAt     *time.Time        `json:"activated_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

func (v CampaignVersion) Validate() error {
	if strings.TrimSpace(v.CampaignID) == "" || v.Version <= 0 || strings.TrimSpace(v.AudienceID) == "" || v.AudienceVersion <= 0 {
		return errors.New("campaign, version and audience version are required")
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
