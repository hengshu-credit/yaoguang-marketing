package domain

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestExecutionSnapshotTouchpointTokenDoesNotAuthenticateRecipient(t *testing.T) {
	now := time.Now().UTC()
	token, err := BuildWebIdentifyTokenWithTouchpoint("message-1", "workspace-secret", time.Hour, now)
	require.NoError(t, err)
	touchpoint, ok := ResolveWebTouchpoint(token, "workspace-secret", now)
	require.True(t, ok)
	require.Equal(t, "message-1", touchpoint)
	_, ok = ResolveWebIdentity(&WebTrackPayload{IdentifyToken: &token}, "workspace-secret", now)
	require.False(t, ok)
	_, ok = ResolveWebTouchpoint(token, "wrong-workspace", now)
	require.False(t, ok)
	_, ok = ResolveWebTouchpoint(token, "workspace-secret", now.Add(2*time.Hour))
	require.False(t, ok)
}
