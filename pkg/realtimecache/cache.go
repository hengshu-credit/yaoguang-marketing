// Package realtimecache contains Redis-backed, rebuildable runtime state. It
// must never be used as the authoritative event or workflow store.
package realtimecache

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type TriggerEntry struct {
	AutomationVersion int             `json:"automation_version"`
	CachedAt          time.Time       `json:"cached_at"`
	Payload           json.RawMessage `json:"payload"`
}

type WindowResult struct {
	Allowed    bool
	Count      int
	RetryAfter time.Duration
}

type FrequencyWindowStore interface {
	ReserveSlidingWindow(
		ctx context.Context,
		workspaceID, subjectID, channel, policyID, reservationID string,
		now time.Time,
		window time.Duration,
		maxEvents int,
	) (WindowResult, error)
}

func TriggerKey(workspaceID, automationID string, automationVersion int) (string, error) {
	if err := validateKeyParts(workspaceID, automationID); err != nil {
		return "", err
	}
	if automationVersion <= 0 {
		return "", fmt.Errorf("automation version must be positive")
	}
	return fmt.Sprintf(
		"notifuse:rt:trigger:%s:%s:v%d",
		url.QueryEscape(workspaceID), url.QueryEscape(automationID), automationVersion,
	), nil
}

func FrequencyKey(workspaceID, subjectID, channel, policyID string) (string, error) {
	if err := validateKeyParts(workspaceID, subjectID, channel, policyID); err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"notifuse:rt:frequency:%s:%s:%s:%s",
		url.QueryEscape(workspaceID), url.QueryEscape(subjectID),
		url.QueryEscape(channel), url.QueryEscape(policyID),
	), nil
}

func validateKeyParts(parts ...string) error {
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return fmt.Errorf("cache key components must not be empty")
		}
		for _, character := range part {
			if character < 0x20 || character == 0x7f {
				return fmt.Errorf("cache key components must not contain control characters")
			}
		}
	}
	return nil
}
