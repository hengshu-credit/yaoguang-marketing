package domain

import (
	"context"
	"errors"
	"strings"
)

type DeliveryListRequest struct {
	WorkspaceID string         `json:"workspace_id"`
	Status      DeliveryStatus `json:"status,omitempty"`
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
	ListDeliveries(context.Context, string, DeliveryStatus, int, int) ([]DeliveryIntent, int, error)
	GetDelivery(context.Context, string, string) (*DeliveryDetail, error)
	RequestDeliveryReconciliation(context.Context, string, string, string) error
	ResolveUnknownDelivery(context.Context, string, string, DeliveryResolutionAction, string, string) error
}

type DeliveryManagementService interface {
	List(context.Context, *DeliveryListRequest) ([]DeliveryIntent, int, error)
	Get(context.Context, *DeliveryGetRequest) (*DeliveryDetail, error)
	Reconcile(context.Context, *DeliveryReconcileRequest) error
	ResolveUnknown(context.Context, *DeliveryResolveUnknownRequest) error
}
