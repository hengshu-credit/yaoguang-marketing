package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

type MarketingPreflightSeverity string

const (
	MarketingPreflightBlocking MarketingPreflightSeverity = "blocking"
	MarketingPreflightWarning  MarketingPreflightSeverity = "warning"
)

type MarketingPreflightCounts struct {
	TargetTotal      int64 `json:"target_total"`
	Reachable        int64 `json:"reachable"`
	MissingIdentity  int64 `json:"missing_identity"`
	MissingConsent   int64 `json:"missing_consent"`
	Suppressed       int64 `json:"suppressed"`
	FrequencyDeny    int64 `json:"frequency_deny"`
	VariableFailures int64 `json:"variable_failures"`
}

type MarketingPreflightIssue struct {
	Code        string                     `json:"code"`
	Severity    MarketingPreflightSeverity `json:"severity"`
	Title       string                     `json:"title"`
	Description string                     `json:"description"`
	FixPath     string                     `json:"fix_path,omitempty"`
}

type MarketingPreflightRequest struct {
	WorkspaceID string `json:"workspace_id"`
	BroadcastID string `json:"broadcast_id"`
}

type MarketingPreflightResult struct {
	WorkspaceID   string                    `json:"workspace_id"`
	BroadcastID   string                    `json:"broadcast_id"`
	Counts        MarketingPreflightCounts  `json:"counts"`
	Issues        []MarketingPreflightIssue `json:"issues"`
	BlockingCount int                       `json:"blocking_count"`
	WarningCount  int                       `json:"warning_count"`
	SummaryHash   string                    `json:"summary_hash"`
	GeneratedAt   time.Time                 `json:"generated_at"`
	ExpiresAt     time.Time                 `json:"expires_at"`
}

// MarketingPreflightSnapshot is the bounded, server-side input used to produce
// a preflight result. It contains aggregate counts rather than recipient rows.
type MarketingPreflightSnapshot struct {
	WorkspaceID             string
	BroadcastID             string
	BroadcastUpdatedAt      time.Time
	Counts                  MarketingPreflightCounts
	HasProvider             bool
	MissingTemplates        []string
	TemplateChannelMismatch []string
	AudienceBuildStale      bool
	HasFrequencyPolicy      bool
}

type MarketingPreflightSource interface {
	LoadMarketingPreflightSnapshot(context.Context, string, string) (*MarketingPreflightSnapshot, error)
}

type MarketingPreflightEvaluator interface {
	PreflightBroadcast(context.Context, MarketingPreflightRequest) (*MarketingPreflightResult, error)
	ValidateBroadcastPreflight(context.Context, MarketingPreflightRequest, string) error
}

var (
	ErrMarketingPreflightRequired = errors.New("send preflight is required")
	ErrMarketingPreflightChanged  = errors.New("send preflight changed; run preflight again")
	ErrMarketingPreflightBlocked  = errors.New("send preflight contains blocking issues")
)

func (r *MarketingPreflightResult) Seal(snapshotUpdatedAt time.Time) error {
	type hashInput struct {
		WorkspaceID string                    `json:"workspace_id"`
		BroadcastID string                    `json:"broadcast_id"`
		UpdatedAt   time.Time                 `json:"updated_at"`
		Counts      MarketingPreflightCounts  `json:"counts"`
		Issues      []MarketingPreflightIssue `json:"issues"`
		ExpiresAt   time.Time                 `json:"expires_at"`
	}
	payload, err := json.Marshal(hashInput{
		WorkspaceID: r.WorkspaceID,
		BroadcastID: r.BroadcastID,
		UpdatedAt:   snapshotUpdatedAt.UTC(),
		Counts:      r.Counts,
		Issues:      r.Issues,
		ExpiresAt:   r.ExpiresAt.UTC(),
	})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	r.SummaryHash = hex.EncodeToString(digest[:]) + "." + strconv.FormatInt(r.ExpiresAt.Unix(), 10)
	return nil
}
