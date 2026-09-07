package domain

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecutionSnapshotTemplateContextKeepsDeletedSourceReadable(t *testing.T) {
	snapshot, err := NewExecutionSnapshot("journey", "instance", map[string]any{}, []*Template{{ID: "t1", Version: 3, Email: &EmailTemplate{Subject: "Old"}}})
	require.NoError(t, err)
	ctx, err := WithExecutionSnapshot(context.Background(), snapshot)
	require.NoError(t, err)
	template, bound, err := ExecutionTemplateFromContext(ctx, "t1", 0)
	require.NoError(t, err)
	require.True(t, bound)
	require.Equal(t, "Old", template.Email.Subject)
	_, bound, err = ExecutionTemplateFromContext(ctx, "new-template", 0)
	require.True(t, bound)
	require.Error(t, err)
}

func TestExecutionSnapshotTemplateContextSelectsNewestCapturedVersion(t *testing.T) {
	snapshot, err := NewExecutionSnapshot("journey", "mixed", map[string]any{}, []*Template{{ID: "t", Version: 1}, {ID: "t", Version: 2}})
	require.NoError(t, err)
	ctx, err := WithExecutionSnapshot(context.Background(), snapshot)
	require.NoError(t, err)
	latest, _, err := ExecutionTemplateFromContext(ctx, "t", 0)
	require.NoError(t, err)
	require.Equal(t, int64(2), latest.Version)
	pinned, _, err := ExecutionTemplateFromContext(ctx, "t", 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), pinned.Version)
	_, _, err = ExecutionTemplateFromContext(ctx, "t", 99)
	require.Error(t, err)
}
