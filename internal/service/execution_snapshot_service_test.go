package service

import (
	"context"
	"errors"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecutionSnapshotServiceRequiresAuthorization(t *testing.T) {
	_, err := NewExecutionSnapshotService(nil, nil)
	require.Error(t, err)
}
func TestExecutionSnapshotMaterialReferences(t *testing.T) {
	urls := executionMediaURLs([]byte(`{"email":{"compiled":"<img src=\"https://cdn.example.org/a.png\"><a href=\"https://example.org/landing\">go</a>"},"attachments":[{"url":"https://cdn.example.org/report.pdf"}],"image_url":"https://cdn.example.org/b.png"}`))
	require.ElementsMatch(t, []string{"https://cdn.example.org/a.png", "https://cdn.example.org/report.pdf", "https://cdn.example.org/b.png"}, urls)
}
func TestExecutionSnapshotReadDoesNotReplaceMissingHistory(t *testing.T) {
	repo := &executionReadStub{}
	s := &ExecutionSnapshotService{repository: repo}
	_, err := s.GetInternal(context.Background(), "ws", "old-run")
	require.True(t, errors.Is(err, domain.ErrExecutionSnapshotNotFound))
}

type executionReadStub struct {
	domain.ExecutionSnapshotRepository
}

func (*executionReadStub) Get(context.Context, string, string) (*domain.ExecutionSnapshot, error) {
	return nil, domain.ErrExecutionSnapshotNotFound
}

func TestExecutionSnapshotMediaReferencesIncludeSrcsetAndRejectUnresolvedSources(t *testing.T) {
	urls, err := executionMediaReferences([]byte(`{"html":"<picture><source srcset=\"https://cdn.example.org/a.png 1x, https://cdn.example.org/b.png 2x\"><img src=\"{{ customer.image }}\"></picture><video poster=\"https://cdn.example.org/poster.jpg\"></video>"}`), true)
	require.NoError(t, err)
	require.Len(t, urls, 3)
	_, err = executionMediaReferences([]byte(`{"html":"<img src=\"/relative.png\">"}`), true)
	require.ErrorContains(t, err, "absolute")
	_, err = executionMediaReferences([]byte(`{"html":"<img src=\"{{ customer.image }}\">"}`), false)
	require.ErrorContains(t, err, "not resolved")
}
