package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stateWorkspaceCursor struct {
	workspaces []string
	err        error
}

func (s stateWorkspaceCursor) NextWorkspaceIDs(context.Context, string, int) ([]string, error) {
	return s.workspaces, s.err
}

type expiryRecorder struct {
	seen   []string
	failed string
}

func (r *expiryRecorder) Tick(_ context.Context, w string, _ time.Time) error {
	r.seen = append(r.seen, w)
	if w == r.failed {
		return errors.New("busy workspace")
	}
	return nil
}

func TestStateMaintenanceContinuesAfterOneWorkspaceFails(t *testing.T) {
	states := &expiryRecorder{failed: "one"}
	worker := NewSubjectStateMaintenanceWorker(stateWorkspaceCursor{workspaces: []string{"one", "two"}}, states, nil)
	count, err := worker.ProcessOnce(context.Background())
	require.ErrorContains(t, err, "one")
	require.Equal(t, 1, count)
	require.Equal(t, []string{"one", "two"}, states.seen)
}

func TestStateMaintenanceNeverProjectsWithoutWorkspaceCursor(t *testing.T) {
	states := &expiryRecorder{}
	worker := NewSubjectStateMaintenanceWorker(stateWorkspaceCursor{err: errors.New("database unavailable")}, states, nil)
	_, err := worker.ProcessOnce(context.Background())
	require.Error(t, err)
	require.Empty(t, states.seen)
}
