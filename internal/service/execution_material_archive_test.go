package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	domainmocks "github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/notifuse_mjml"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"os"
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

func TestStarterTemplateArchivesWithoutExternalImageServices(t *testing.T) {
	data, err := os.ReadFile("../../console/src/components/templates/email-template.json")
	require.NoError(t, err)
	var starter struct {
		EmailTree json.RawMessage `json:"emailTree"`
	}
	require.NoError(t, json.Unmarshal(data, &starter))
	tree, err := notifuse_mjml.UnmarshalEmailBlock(starter.EmailTree)
	require.NoError(t, err)
	repo := &snapshotArchiveStub{}
	requests := 0
	s := &ExecutionSnapshotService{repository: repo, client: &http.Client{Transport: snapshotRoundTripper(func(r *http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("external image service unavailable")
	})}}
	template := &domain.Template{ID: "starter", Name: "Starter", Channel: "email", Category: "transactional", Email: &domain.EmailTemplate{Subject: "Welcome", SenderID: "sender", VisualEditorTree: tree}}
	require.NoError(t, s.PrepareTemplate(context.Background(), "ws", template))
	require.Zero(t, requests, "the built-in starter must save without third-party image downloads")
	require.JSONEq(t, "[]", string(repo.manifest))
	ctrl := gomock.NewController(t)
	templates := domainmocks.NewMockTemplateRepository(ctrl)
	auth := domainmocks.NewMockAuthService(ctrl)
	ctx := context.Background()
	auth.EXPECT().AuthenticateUserForWorkspace(ctx, "ws").Return(ctx, &domain.User{ID: "user"}, &domain.UserWorkspace{
		Permissions: domain.UserPermissions{domain.PermissionResourceTemplates: {Read: true, Write: true}},
	}, nil).Times(2)
	service := &TemplateService{repo: templates, authService: auth, executionSnapshots: s}
	templates.EXPECT().CreateTemplate(ctx, "ws", template).Return(nil)
	require.NoError(t, service.CreateTemplate(ctx, "ws", template))
	existing := *template
	templates.EXPECT().GetTemplateByID(ctx, "ws", template.ID, int64(0)).Return(&existing, nil)
	templates.EXPECT().UpdateTemplate(ctx, "ws", template, int64(1)).Return(nil)
	require.NoError(t, service.UpdateTemplate(ctx, "ws", template))
	require.Zero(t, requests)
	// Actual user-provided media still needs to be archived; failures are not swallowed.
	template.Settings = domain.MapOfAny{"image_url": "https://cdn.example.org/customer.png"}
	require.ErrorContains(t, s.PrepareTemplate(context.Background(), "ws", template), "external image service unavailable")
}
