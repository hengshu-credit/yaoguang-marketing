package service

import (
	"context"
	"encoding/json"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type runtimeArchiveStub struct {
	domain.ExecutionSnapshotRepository
	ciphertext string
	raw        json.RawMessage
}

func (s *runtimeArchiveStub) SaveRuntimeNodes(_ context.Context, _, _ string, raw json.RawMessage, encrypted string, _ bool) error {
	s.ciphertext = encrypted
	s.raw = raw
	return nil
}
func (s *runtimeArchiveStub) RuntimeNodes(context.Context, string, string) (string, error) {
	return s.ciphertext, nil
}
func (s *runtimeArchiveStub) LatestRuntimeReference(context.Context, string, string) (string, error) {
	return "private-reference", nil
}

func TestExecutionSnapshotCampaignCapturesProviderBeforeSettingsEdit(t *testing.T) {
	repo := &runtimeArchiveStub{}
	workspace := &domain.Workspace{Settings: domain.WorkspaceSettings{SecretKey: "runtime-secret-key", MarketingEmailProviderID: "smtp"}, Integrations: domain.Integrations{{ID: "smtp", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, SMTP: &domain.SMTPSettings{Host: "old.example.org", Password: "old-password"}}}}}
	svc := &ExecutionSnapshotService{repository: repo, workspaces: &mutableRuntimeWorkspace{workspace: workspace}}
	snapshot, err := domain.NewExecutionSnapshot("campaign", "run-before-edit", map[string]any{"name": "Campaign"}, nil)
	require.NoError(t, err)
	require.NoError(t, svc.captureCampaignProviders(context.Background(), "ws", snapshot))
	require.NotContains(t, repo.ciphertext, "old-password")
	require.NotContains(t, string(snapshot.Dependencies), "old-password")
	workspace.Integrations[0].EmailProvider.SMTP.Password = "new-password"
	bound, err := svc.bindRuntimeProviders(context.Background(), "ws", snapshot)
	require.NoError(t, err)
	provider, integration, boundOK, err := domain.ExecutionEmailProviderFromContext(bound)
	require.NoError(t, err)
	require.True(t, boundOK)
	require.Equal(t, "smtp", integration)
	require.Equal(t, "old-password", provider.SMTP.Password)
	next, err := domain.NewExecutionSnapshot("campaign", "run-after-edit", map[string]any{"name": "Campaign"}, nil)
	require.NoError(t, err)
	require.NoError(t, svc.captureCampaignProviders(context.Background(), "ws", next))
	bound, err = svc.bindRuntimeProviders(context.Background(), "ws", next)
	require.NoError(t, err)
	provider, _, _, err = domain.ExecutionEmailProviderFromContext(bound)
	require.NoError(t, err)
	require.Equal(t, "new-password", provider.SMTP.Password)
}

type runtimeWorkspaceStub struct{ domain.WorkspaceRepository }

func (*runtimeWorkspaceStub) GetByID(context.Context, string) (*domain.Workspace, error) {
	return &domain.Workspace{Settings: domain.WorkspaceSettings{SecretKey: "runtime-secret-key"}}, nil
}
func TestExecutionSnapshotRuntimeCredentialsAreEncryptedAndRemainOld(t *testing.T) {
	repo := &runtimeArchiveStub{}
	s := &ExecutionSnapshotService{repository: repo}
	s.SetWorkspaceRepository(&runtimeWorkspaceStub{})
	automation := &domain.Automation{ID: "a", Nodes: []*domain.AutomationNode{{ID: "webhook", Config: map[string]interface{}{"secret": "original-token", "url": "https://example.org"}}}}
	require.NoError(t, s.archiveRuntimeNodes(context.Background(), "ws", automation))
	require.NotContains(t, repo.ciphertext, "original-token")
	snapshot, err := domain.NewExecutionSnapshot("journey", "j", automation, nil)
	require.NoError(t, err)
	snapshot.Dependencies = json.RawMessage(`{"runtime_nodes_ref":"private-reference"}`)
	automation.Nodes[0].Config["secret"] = "changed-token"
	var frozen domain.Automation
	require.NoError(t, json.Unmarshal(snapshot.Configuration, &frozen))
	require.Equal(t, "[REDACTED]", frozen.Nodes[0].Config["secret"])
	require.NoError(t, s.hydrateRuntimeNodes(context.Background(), "ws", snapshot, &frozen))
	require.Equal(t, "original-token", frozen.Nodes[0].Config["secret"])
	require.NotContains(t, string(snapshot.Configuration), "original-token")
}

func TestExecutionSnapshotArchivesReferencedProviderUntilNextActivation(t *testing.T) {
	repo := &runtimeArchiveStub{}
	workspace := &domain.Workspace{Settings: domain.WorkspaceSettings{SecretKey: "runtime-secret-key"}, Integrations: domain.Integrations{{ID: "sms", Name: "original", Type: domain.IntegrationTypeSMS, SMSProvider: &domain.SMSProvider{Twilio: &domain.TwilioSettings{AuthToken: "original-secret"}}}}}
	source := &mutableRuntimeWorkspace{workspace: workspace}
	s := &ExecutionSnapshotService{repository: repo, workspaces: source}
	a := &domain.Automation{ID: "a", Nodes: []*domain.AutomationNode{{ID: "sms-node", Config: map[string]interface{}{"integration_id": "sms"}}}}
	require.NoError(t, s.archiveRuntimeNodes(context.Background(), "ws", a))
	require.NotEmpty(t, repo.ciphertext)
	require.NotContains(t, repo.ciphertext, "original-secret")
	snapshot, err := domain.NewExecutionSnapshot("journey", "j", a, nil)
	require.NoError(t, err)
	snapshot.Dependencies = json.RawMessage(`{"runtime_nodes_ref":"private-reference"}`)
	workspace.Integrations[0].SMSProvider.Twilio.AuthToken = "changed-secret"
	workspace.Integrations[0].Name = "changed"
	bound, err := s.bindRuntimeProviders(context.Background(), "ws", snapshot)
	require.NoError(t, err)
	frozen := domain.ExecutionProviderWorkspace(bound, workspace)
	require.Equal(t, "original-secret", frozen.Integrations[0].SMSProvider.Twilio.AuthToken)
	require.Equal(t, "original", frozen.Integrations[0].Name)
	require.Equal(t, "changed-secret", workspace.Integrations[0].SMSProvider.Twilio.AuthToken)
	require.NoError(t, s.archiveRuntimeNodes(context.Background(), "ws", a))
	bound, err = s.bindRuntimeProviders(context.Background(), "ws", snapshot)
	require.NoError(t, err)
	require.Equal(t, "changed-secret", domain.ExecutionProviderWorkspace(bound, workspace).Integrations[0].SMSProvider.Twilio.AuthToken)
}

type mutableRuntimeWorkspace struct {
	domain.WorkspaceRepository
	workspace *domain.Workspace
}

func (w *mutableRuntimeWorkspace) GetByID(context.Context, string) (*domain.Workspace, error) {
	return w.workspace, nil
}

// The server locator is a persisted delivery, not a client-provided execution ID.
func (s *runtimeArchiveStub) SnapshotForChannelSend(context.Context, string, string, string, string, string) (*domain.ExecutionSnapshot, error) {
	return &domain.ExecutionSnapshot{Dependencies: json.RawMessage(`{"runtime_nodes_ref":"private-reference"}`)}, nil
}
func TestExecutionSnapshotTwilioCallbackUsesOriginalProviderCredential(t *testing.T) {
	repo := &runtimeArchiveStub{}
	workspace := &domain.Workspace{Settings: domain.WorkspaceSettings{SecretKey: "runtime-secret-key"}, Integrations: domain.Integrations{{ID: "sms", Type: domain.IntegrationTypeSMS, SMSProvider: &domain.SMSProvider{Kind: domain.SMSProviderKindTwilio, Twilio: &domain.TwilioSettings{AuthToken: "original-token"}}}}}
	source := &mutableRuntimeWorkspace{workspace: workspace}
	snapshots := &ExecutionSnapshotService{repository: repo, workspaces: source}
	require.NoError(t, snapshots.archiveRuntimeNodes(context.Background(), "ws", &domain.Automation{ID: "a", Nodes: []*domain.AutomationNode{{ID: "send", Config: map[string]interface{}{"integration_id": "sms"}}}}))
	workspace.Integrations[0].SMSProvider.Twilio.AuthToken = "changed-token"
	receipts := &receiptRepositoryStub{results: []domain.DeliveryReceiptRecordResult{{Applied: true}}}
	svc := &DeliveryReceiptService{repo: receipts, workspaceRepo: source, executionSnapshots: snapshots, now: time.Now}
	form := map[string][]string{"MessageSid": {"SM1"}, "MessageStatus": {"delivered"}}
	callback := domain.TwilioDeliveryCallback{WorkspaceID: "ws", IntegrationID: "sms", MessageID: "message", EffectKey: "effect", CallbackURL: "https://notify.example.org/callback", Form: form}
	callback.Signature = signTwilioTestRequest(callback.CallbackURL, form, "original-token")
	result, err := svc.ProcessTwilioCallback(context.Background(), callback)
	require.NoError(t, err)
	require.True(t, result.Applied)
	callback.Signature = signTwilioTestRequest(callback.CallbackURL, form, "changed-token")
	_, err = svc.ProcessTwilioCallback(context.Background(), callback)
	require.ErrorIs(t, err, ErrInvalidTwilioSignature)
}
