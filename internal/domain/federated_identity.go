package domain

import (
	"context"
	"time"
)

//go:generate mockgen -destination mocks/mock_federated_identity_repository.go -package mocks github.com/Notifuse/notifuse/internal/domain FederatedIdentityRepository

// FederatedIdentity links an external IdP subject to a Notifuse user. The durable
// identity key is (IDPIssuer, IDPSub); email is used only to bridge to an invited
// user on first login and is never used for authentication once a link exists.
type FederatedIdentity struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	IDPIssuer string    `json:"idp_issuer" db:"idp_issuer"`
	IDPSub    string    `json:"idp_sub" db:"idp_sub"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ErrFederatedIdentityNotFound is returned when no row matches the lookup.
type ErrFederatedIdentityNotFound struct{ Message string }

func (e *ErrFederatedIdentityNotFound) Error() string { return e.Message }

// ErrFederatedIdentityExists is returned on a UNIQUE violation (PG 23505) of
// EITHER UNIQUE(idp_issuer, idp_sub) OR UNIQUE(user_id, idp_issuer). The service
// distinguishes a benign exact-duplicate race from a genuine link conflict.
type ErrFederatedIdentityExists struct{ Message string }

func (e *ErrFederatedIdentityExists) Error() string { return e.Message }

// FederatedIdentityRepository is the data-access contract for federated identities.
type FederatedIdentityRepository interface {
	// GetByIssuerSubject returns the identity for (issuer, sub) or
	// *ErrFederatedIdentityNotFound. Served by UNIQUE(idp_issuer, idp_sub).
	GetByIssuerSubject(ctx context.Context, issuer, sub string) (*FederatedIdentity, error)
	// GetByUserAndIssuer returns the link for (user_id, issuer) or
	// *ErrFederatedIdentityNotFound. Deterministic via UNIQUE(user_id, idp_issuer).
	GetByUserAndIssuer(ctx context.Context, userID, issuer string) (*FederatedIdentity, error)
	// Create inserts a new link; *ErrFederatedIdentityExists on a UNIQUE violation.
	Create(ctx context.Context, fi *FederatedIdentity) error
	// ListByUserID returns all federated identities for a user (audit / account view).
	ListByUserID(ctx context.Context, userID string) ([]*FederatedIdentity, error)
}
