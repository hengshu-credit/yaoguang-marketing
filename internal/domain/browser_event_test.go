package domain

import (
	"testing"
	"time"
)

func TestBrowserEventCredentialBoundToSessionOriginAndExpiry(t *testing.T) {
	now := time.Now().UTC()
	claims := BrowserEventClaims{WorkspaceID: "w", CustomerID: "11111111-1111-4111-8111-111111111111", SessionID: "session", Origin: "https://shop.example", EventCodes: []string{"login"}, ExpiresAt: now.Add(time.Minute).Unix()}
	token, err := SignBrowserEventCredential(claims, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = VerifyBrowserEventCredential(token, "secret", "w", "session", "https://shop.example", now); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []struct {
		session, origin string
		at              time.Time
	}{{"other", claims.Origin, now}, {claims.SessionID, "https://evil.example", now}, {claims.SessionID, claims.Origin, now.Add(time.Hour)}} {
		if _, err = VerifyBrowserEventCredential(token, "secret", "w", bad.session, bad.origin, bad.at); err == nil {
			t.Fatal("credential boundary ignored")
		}
	}
}
