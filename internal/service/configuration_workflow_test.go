package service

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEventExtensionAndStateInspectionEnforcePermissions(t *testing.T) {
	denied := businessTestAuth{member: &domain.UserWorkspace{Role: "member"}}
	_, err := NewBusinessConfigService(nil, denied).ExtendEvents(context.Background(), "ws", []string{"email.sent"}, map[string]any{"type": "object"})
	require.Error(t, err)
	_, err = NewSubjectStateService(nil, nil, nil, denied).Inspect(context.Background(), SubjectStateInspectRequest{WorkspaceID: "ws", SubjectType: "customer", SubjectID: "user@example.com"})
	require.Error(t, err)
}
