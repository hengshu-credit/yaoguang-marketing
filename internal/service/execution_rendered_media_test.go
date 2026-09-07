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

type renderedMediaRepo struct {
	snapshotArchiveStub
	snapshot *domain.ExecutionSnapshot
	evidence domain.ExecutionEvidence
}

func (r *renderedMediaRepo) Get(context.Context, string, string) (*domain.ExecutionSnapshot, error) {
	return r.snapshot, nil
}
func (r *renderedMediaRepo) AppendEvidence(_ context.Context, _ string, e domain.ExecutionEvidence) error {
	r.evidence = e
	return nil
}
func (r *renderedMediaRepo) DeliveryMediaReferences(context.Context, string, string, string) (json.RawMessage, error) {
	return r.evidence.Payload, nil
}
func TestExecutionSnapshotRenderedMediaFreezesDynamicAssetsAndCSSDependencies(t *testing.T) {
	repo := &renderedMediaRepo{snapshot: &domain.ExecutionSnapshot{Materials: json.RawMessage(`{"assets":[{"source_url":"https://cdn.example.org/static.png","hash":"original-hash","media_type":"image/png"}]}`)}}
	calls := []string{}
	svc := &ExecutionSnapshotService{repository: repo, client: &http.Client{Transport: snapshotRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.URL.String())
		kind, body := "image/png", "original-personalized-image"
		if strings.HasSuffix(r.URL.Path, ".css") {
			kind, body = "text/css", `@font-face {src:url('../font.woff2')}`
		} else if strings.HasSuffix(r.URL.Path, ".woff2") {
			kind, body = "font/woff2", "original-font"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{kind}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	content := map[string]any{"html": "<img src=\"https://cdn.example.org/static.png\"><img srcset=\"https://cdn.example.org/customer-42.png 1x\"><link href=\"https://cdn.example.org/css/main.css\"><img src=\"https://notify.example.org/opens?wid=ws&mid=m\">"}
	require.NoError(t, svc.ArchiveRenderedMedia(context.Background(), "ws", "execution", "m", content))
	require.ElementsMatch(t, []string{"https://cdn.example.org/customer-42.png", "https://cdn.example.org/css/main.css", "https://cdn.example.org/font.woff2"}, calls)
	require.Len(t, repo.assets, 3)
	require.Contains(t, string(repo.evidence.Payload), "original-hash")
	require.Equal(t, "delivery_media", repo.evidence.Kind)
	require.NoError(t, svc.ArchiveRenderedMedia(context.Background(), "ws", "execution", "m", content))
	require.Len(t, calls, 3, "retry reuses immutable rendered media manifest")
	_, err := executionMediaReferences([]byte(`{"ImageURL":"/relative.png"}`), false)
	require.Error(t, err)
}
