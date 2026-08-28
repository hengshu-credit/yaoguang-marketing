package domain

import (
	"context"
	"errors"
)

//go:generate mockgen -destination mocks/mock_oidc_service.go -package mocks github.com/Notifuse/notifuse/internal/domain OIDCServiceInterface

// ErrOIDCNotConfigured is returned when OIDC is disabled or the upstream provider
// has not yet initialized (graceful degradation). HTTP maps it to 503.
var ErrOIDCNotConfigured = errors.New("oidc not configured")

// ErrOIDCIdentityConflict is returned when (issuer, email) maps to an existing user
// that already holds a DIFFERENT sub for this issuer (possible email-recycling /
// takeover). HTTP maps it to ?oidc_error=link_conflict; the service audit-logs.
var ErrOIDCIdentityConflict = errors.New("oidc identity conflict")

// ErrOIDCAccountNotProvisioned is returned under the invited-only policy: a verified
// SSO identity has no matching Notifuse user and JIT provisioning is disabled.
var ErrOIDCAccountNotProvisioned = errors.New("no Notifuse account for this identity; ask to be invited")

// ErrOIDCEmailNotVerified is returned when the IdP did not assert email_verified==true.
var ErrOIDCEmailNotVerified = errors.New("oidc email not verified by provider")

// ErrOIDCDomainNotAllowed is returned when JIT is enabled but the email domain is
// not in the configured allowlist.
var ErrOIDCDomainNotAllowed = errors.New("oidc email domain not allowed")

// OIDCFlowState is the per-login CSRF/replay state, AEAD-encrypted into the __Host-
// flow cookie by the HTTP layer (Verifier is a secret).
type OIDCFlowState struct {
	State    string `json:"state"`    // opaque CSRF token, compared to ?state=
	Nonce    string `json:"nonce"`    // bound into auth req; checked vs ID-token nonce claim
	Verifier string `json:"verifier"` // PKCE code_verifier (oauth2.GenerateVerifier)
}

// OIDCAuthRequest is returned by BuildAuthURL.
type OIDCAuthRequest struct {
	AuthURL   string
	FlowState OIDCFlowState
}

// OIDCCallbackInput carries the raw callback query params plus the decrypted
// flow-state from the __Host- cookie.
type OIDCCallbackInput struct {
	Code      string        // ?code=
	State     string        // ?state=
	Iss       string        // ?iss= (RFC 9207; may be empty for non-compliant IdPs)
	FlowState OIDCFlowState // decrypted from the __Host- cookie
}

// OIDCServiceInterface is the service contract consumed by the HTTP layer.
type OIDCServiceInterface interface {
	// IsEnabled reports config.Enabled (cheap; does NOT touch the provider).
	IsEnabled() bool
	// BuildAuthURL generates state+nonce+PKCE verifier, builds the IdP authorization
	// URL (S256), and returns both. ErrOIDCNotConfigured if disabled.
	BuildAuthURL(ctx context.Context) (*OIDCAuthRequest, error)
	// HandleCallback runs the full identity state machine, mints the session, stores
	// the AuthResponse under a fresh one-time code, and returns that code (NOT the JWT).
	HandleCallback(ctx context.Context, in OIDCCallbackInput) (oneTimeCode string, err error)
	// ExchangeCode atomically consumes the one-time code and returns the AuthResponse
	// (single-use).
	ExchangeCode(ctx context.Context, oneTimeCode string) (*AuthResponse, error)
	// SealFlowState AEAD-encrypts the per-login flow state into an opaque blob for
	// the flow cookie (the AES key stays in the service layer).
	SealFlowState(fs OIDCFlowState) (string, error)
	// OpenFlowState decrypts and parses a sealed flow-state blob.
	OpenFlowState(enc string) (OIDCFlowState, error)
}
