package service

import (
	"context"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

type snapshotArchiveStub struct {
	domain.ExecutionSnapshotRepository
	assets   []domain.ExecutionAsset
	manifest json.RawMessage
}

func (s *snapshotArchiveStub) SaveAsset(_ context.Context, _ string, a domain.ExecutionAsset) error {
	s.assets = append(s.assets, a)
	return nil
}
func (s *snapshotArchiveStub) SaveTemplateMaterialArchive(_ context.Context, _ string, _ *domain.Template, assets json.RawMessage) error {
	s.manifest = assets
	return nil
}

type snapshotRoundTripper func(*http.Request) (*http.Response, error)

func (f snapshotRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestExecutionSnapshotArchivesBeforeTemplateCanBePublished(t *testing.T) {
	repo := &snapshotArchiveStub{}
	s := &ExecutionSnapshotService{repository: repo, client: &http.Client{Transport: snapshotRoundTripper(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(strings.NewReader("original-image-bytes"))}, nil
	})}}
	template := &domain.Template{ID: "template", Email: &domain.EmailTemplate{Subject: "Original"}, Settings: domain.MapOfAny{"image_url": "https://cdn.example.org/logo.png"}}
	require.NoError(t, s.PrepareTemplate(context.Background(), "ws", template))
	require.Len(t, repo.assets, 1)
	require.Equal(t, "original-image-bytes", string(repo.assets[0].Data))
	require.Contains(t, string(repo.manifest), repo.assets[0].Hash)
}
