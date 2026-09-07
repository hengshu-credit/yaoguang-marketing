package service

import (
	"context"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

type archiveWorkspaceStub struct {
	domain.WorkspaceCursorRepository
	limit int
}

func (s *archiveWorkspaceStub) NextWorkspaceIDs(_ context.Context, _ string, limit int) ([]string, error) {
	s.limit = limit
	return []string{"ws"}, nil
}

type archiveBacklogStub struct{ limit int }

func (s *archiveBacklogStub) UnarchivedAutomationTemplates(_ context.Context, _ string, limit int) ([]*domain.Template, error) {
	s.limit = limit
	return []*domain.Template{{ID: "old-active", Channel: "email"}}, nil
}
func TestExecutionMaterialBackfillBoundsExistingActiveWork(t *testing.T) {
	workspaces := &archiveWorkspaceStub{}
	backlog := &archiveBacklogStub{}
	repo := &snapshotArchiveStub{}
	worker := NewExecutionMaterialBackfillWorker(workspaces, backlog, &ExecutionSnapshotService{repository: repo}, nil)
	n, err := worker.ProcessOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.Equal(t, 10, workspaces.limit)
	require.Equal(t, 25, backlog.limit)
	require.JSONEq(t, `[]`, string(repo.manifest))
}
