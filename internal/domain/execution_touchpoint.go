package domain

import (
	"encoding/json"
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/crypto"
	"strings"
	"time"
)

// A v2 email-link credential proves a marketing touchpoint, never a customer
// identity. Legacy v1 identity tokens continue through their existing API.
func BuildWebIdentifyTokenWithTouchpoint(touchpointID, secretKey string, ttl time.Duration, now time.Time) (string, error) {
	if secretKey == "" || strings.TrimSpace(touchpointID) == "" || len(touchpointID) > 100 || ttl <= 0 {
		return "", errors.New("touchpoint id, workspace secret and positive expiry are required")
	}
	body, err := json.Marshal(struct {
		Touchpoint string `json:"t"`
		ExpiresAt  int64  `json:"x"`
		Version    int    `json:"v"`
	}{touchpointID, now.Add(ttl).Unix(), 2})
	if err != nil {
		return "", err
	}
	token, err := crypto.EncryptString(string(body), secretKey)
	if err != nil {
		return "", err
	}
	if len(token) > WebTrackMaxIdentifyTokenLength {
		return "", errors.New("touchpoint credential exceeds identity token bounds")
	}
	return token, nil
}
func ResolveWebTouchpoint(token, secretKey string, now time.Time) (string, bool) {
	if secretKey == "" || len(token) > WebTrackMaxIdentifyTokenLength {
		return "", false
	}
	body, err := crypto.DecryptFromHexString(token, secretKey)
	if err != nil {
		return "", false
	}
	var payload struct {
		Touchpoint string `json:"t"`
		ExpiresAt  int64  `json:"x"`
		Version    int    `json:"v"`
	}
	if json.Unmarshal([]byte(body), &payload) != nil || payload.Version != 2 || payload.ExpiresAt <= now.Unix() || payload.Touchpoint == "" || len(payload.Touchpoint) > 100 {
		return "", false
	}
	return payload.Touchpoint, true
}
