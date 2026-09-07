package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"time"
)

type BrowserEventClaims struct {
	WorkspaceID string   `json:"workspace_id"`
	CustomerID  string   `json:"customer_id"`
	SessionID   string   `json:"session_id"`
	Origin      string   `json:"origin"`
	EventCodes  []string `json:"event_codes"`
	ExpiresAt   int64    `json:"expires_at"`
}

func SignBrowserEventCredential(c BrowserEventClaims, secret string) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("browser credential signing secret is required")
	}
	if _, err := uuid.Parse(c.CustomerID); err != nil {
		return "", fmt.Errorf("customer_id must be a UUID")
	}
	body, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("browser-events:v1:" + payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func VerifyBrowserEventCredential(token, secret, workspace, session, origin string, now time.Time) (*BrowserEventClaims, error) {
	fail := fmt.Errorf("invalid browser identity credential")
	if len(token) > 8192 || secret == "" {
		return nil, fail
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, fail
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fail
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("browser-events:v1:" + parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, fail
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fail
	}
	var c BrowserEventClaims
	if err = json.Unmarshal(payload, &c); err != nil {
		return nil, fail
	}
	if c.WorkspaceID != workspace || c.SessionID != session || c.Origin != origin || c.ExpiresAt <= now.Unix() || c.ExpiresAt > now.Add(15*time.Minute).Unix() || c.CustomerID == "" {
		return nil, fail
	}
	return &c, nil
}

type BrowserTrackEventRequest struct {
	TouchpointToken string         `json:"touchpoint_token,omitempty"`
	WorkspaceID     string         `json:"workspace_id"`
	SessionID       string         `json:"session_id"`
	Credential      string         `json:"credential,omitempty"`
	Event           map[string]any `json:"event"`
}
