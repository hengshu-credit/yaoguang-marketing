package domain

import (
	"context"
	"errors"
	"strings"
	"time"
)

type DeliveryListRequest struct {
	WorkspaceID string         `json:"workspace_id"`
	Status      DeliveryStatus `json:"status,omitempty"`
	Channel     string         `json:"channel,omitempty"`
	SourceType  string         `json:"source_type,omitempty"`
	SourceID    string         `json:"source_id,omitempty"`
	Provider    string         `json:"provider,omitempty"`
	CustomerID  string         `json:"customer_id,omitempty"`
	From        *time.Time     `json:"from,omitempty"`
	To          *time.Time     `json:"to,omitempty"`
	Limit       int            `json:"limit,omitempty"`
	Offset      int            `json:"offset,omitempty"`
}

func (r *DeliveryListRequest) Validate() error {
	if r == nil || strings.TrimSpace(r.WorkspaceID) == "" {
		return errors.New("workspace_id is required")
	}
	if r.Status != "" && !r.Status.Valid() {
		return errors.New("status is invalid")
	}
	for name, value := range map[string]string{"channel": r.Channel, "source_type": r.SourceType, "source_id": r.SourceID, "provider": r.Provider, "customer_id": r.CustomerID} {
		if len(strings.TrimSpace(value)) > 255 {
			return errors.New(name + " cannot exceed 255 characters")
		}
	}
	if r.From != nil && r.To != nil && r.From.After(*r.To) {
		return errors.New("from cannot be after to")
	}
	if r.Limit < 0 || r.Limit > 500 || r.Offset < 0 {
		return errors.New("limit must be between 0 and 500 and offset must be non-negative")
	}
	return nil
}

type DeliveryGetRequest struct {
	WorkspaceID string `json:"workspace_id"`
	IntentID    string `json:"intent_id"`
}

func (r *DeliveryGetRequest) Validate() error {
	if r == nil || strings.TrimSpace(r.WorkspaceID) == "" || strings.TrimSpace(r.IntentID) == "" {
		return errors.New("workspace_id and intent_id are required")
	}
	return nil
}

type DeliveryDetail struct {
	Intent          DeliveryIntent           `json:"intent"`
	Attempts        []DeliveryAttempt        `json:"attempts"`
	Reconciliations []DeliveryReconciliation `json:"reconciliations"`
}

type DeliveryProgress struct {
	AudienceTotal int64 `json:"audience_total"`
	Planned       int64 `json:"planned"`
	Reserved      int64 `json:"reserved"`
	Queued        int64 `json:"queued"`
	Submitting    int64 `json:"submitting"`
	Accepted      int64 `json:"accepted"`
	Confirmed     int64 `json:"confirmed"`
	Suppressed    int64 `json:"suppressed"`
	Deferred      int64 `json:"deferred"`
	Failed        int64 `json:"failed"`
	Unknown       int64 `json:"unknown"`
	Cancelled     int64 `json:"cancelled"`
	Processed     int64 `json:"processed"` // Deprecated aggregate for compatibility.
}

type DeliveryResolutionAction string

const (
	DeliveryResolutionMarkConfirmed            DeliveryResolutionAction = "mark_confirmed"
	DeliveryResolutionMarkTerminalFailed       DeliveryResolutionAction = "mark_terminal_failed"
	DeliveryResolutionRetryVerifiedNotAccepted DeliveryResolutionAction = "retry_after_verified_not_accepted"
)

type DeliveryReconcileRequest struct {
	WorkspaceID string `json:"workspace_id"`
	IntentID    string `json:"intent_id"`
}

func (r *DeliveryReconcileRequest) Validate() error {
	if r == nil || strings.TrimSpace(r.WorkspaceID) == "" || strings.TrimSpace(r.IntentID) == "" {
		return errors.New("workspace_id and intent_id are required")
	}
	return nil
}

type DeliveryResolveUnknownRequest struct {
	WorkspaceID string                   `json:"workspace_id"`
	IntentID    string                   `json:"intent_id"`
	Action      DeliveryResolutionAction `json:"action"`
	Reason      string                   `json:"reason"`
}

func (r *DeliveryResolveUnknownRequest) Validate() error {
	if r == nil || strings.TrimSpace(r.WorkspaceID) == "" || strings.TrimSpace(r.IntentID) == "" {
		return errors.New("workspace_id and intent_id are required")
	}
	switch r.Action {
	case DeliveryResolutionMarkConfirmed, DeliveryResolutionMarkTerminalFailed, DeliveryResolutionRetryVerifiedNotAccepted:
	default:
		return errors.New("action is invalid")
	}
	if len(strings.TrimSpace(r.Reason)) < 8 || len(r.Reason) > 2000 {
		return errors.New("reason must contain between 8 and 2000 characters")
	}
	return nil
}

var ErrDeliveryNotFound = errors.New("delivery intent not found")

type DeliveryManagementRepository interface {
	ListDeliveries(context.Context, DeliveryListRequest) ([]DeliveryIntent, int, error)
	GetDelivery(context.Context, string, string) (*DeliveryDetail, error)
	RequestDeliveryReconciliation(context.Context, string, string, string) error
	ResolveUnknownDelivery(context.Context, string, string, DeliveryResolutionAction, string, string) error
	GetDeliveryProgress(context.Context, string, DeliverySource, string, string) (DeliveryProgress, error)
}

type DeliveryManagementService interface {
	List(context.Context, *DeliveryListRequest) ([]DeliveryIntent, int, error)
	Get(context.Context, *DeliveryGetRequest) (*DeliveryDetail, error)
	Reconcile(context.Context, *DeliveryReconcileRequest) error
	ResolveUnknown(context.Context, *DeliveryResolveUnknownRequest) error
}
