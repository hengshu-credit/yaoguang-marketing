# Omnichannel Material Templates and Client Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a country-aware channel catalogue, versioned omnichannel material templates, server-rendered multi-client previews, and signed generic Webhook delivery for newly added channels while preserving existing Email, SMS and Push behavior.

**Architecture:** The backend owns a static channel registry, typed generic content model, channel validation, Liquid rendering and signed delivery compilation. New templates persist in versioned `content JSONB`; the console consumes catalogue capabilities to render focused editors and structured simulated client previews. Existing native providers remain in place, while every new delivery channel resolves through an encrypted `channel_webhook` integration.

**Tech Stack:** Go 1.x, PostgreSQL custom Workspace migrations, stdlib `http.ServeMux`, LiquidGo through `notifuse_mjml.ProcessLiquidTemplate`, React 18, TypeScript, Ant Design, TanStack Query, LinguiJS, Vitest, Playwright.

**Spec:** `docs/superpowers/specs/2026-08-31-omnichannel-material-template-preview-design.md`

## Global Constraints

- Preserve all unrelated dirty-worktree changes; inspect overlapping files before every edit.
- Do not create a branch, stage files, commit, push, deploy or publish without a later explicit instruction.
- Existing Email, Web, SMS and Push request and persistence payloads remain backward compatible.
- Existing native Email providers, Twilio SMS and FCM Push remain unchanged.
- New third-party channels deliver only through `channel_webhook` in this phase.
- Every user-facing console string uses `useLingui` or `Trans`; regenerate and compile catalogues after edits.
- Preview never silently truncates content and must label third-party client surfaces as simulations.
- Sensitive endpoint addresses, secrets and rendered customer content never enter public JSON or logs.
- Every touched backend implementation file has a focused test in the same layer.

## File map

- `internal/domain/channel_catalog.go`: stable channel, country-pack, capability and preview-profile registry.
- `internal/domain/channel_template_content.go`: typed version-one generic material schema and validation.
- `internal/service/channel_catalog_service.go`: authenticated catalogue read boundary.
- `internal/http/channel_catalog_handler.go`: `/api/channels.catalog` transport.
- `internal/migrations/v54.go`: template columns and distributed receipt-nonce table.
- `internal/repository/template_postgres.go`: generic content persistence alongside legacy columns.
- `internal/service/template_preview_service.go`: shared language resolution and existing SMS/Push preview routing.
- `internal/service/channel_template_preview.go`: generic recursive Liquid renderer and diagnostics.
- `console/src/services/api/channels.ts`: catalogue types and client.
- `console/src/components/templates/ChannelPicker.tsx`: country recommendations and channel selection.
- `console/src/components/templates/OmnichannelTemplateDrawer.tsx`: generic material editor and live preview orchestration.
- `console/src/components/templates/OmnichannelFields.tsx`: capability-aware typed field groups.
- `console/src/components/templates/OmnichannelPreview.tsx`: preview-profile selector and renderer routing.
- `console/src/components/templates/channelPreviews/*.tsx`: focused client-surface renderers.
- `internal/domain/channel_webhook.go`: encrypted integration settings, envelope and response contracts.
- `internal/service/channel_webhook_provider.go`: canonical signing and bounded HTTP provider.
- `internal/repository/channel_webhook_nonce_postgres.go`: cross-process nonce reservation.
- `internal/service/channel_webhook_receipt.go`: signed receipt verification and normalization.
- `console/src/components/integrations/ChannelWebhookIntegration.tsx`: owner configuration UI.

---

### Task 1: Channel catalogue, country packs and content locales

**Files:**
- Create: `internal/domain/channel_catalog.go`
- Create: `internal/domain/channel_catalog_test.go`
- Modify: `internal/domain/languages.go`
- Modify: `internal/domain/languages_test.go`
- Create: `internal/service/channel_catalog_service.go`
- Create: `internal/service/channel_catalog_service_test.go`
- Create: `internal/http/channel_catalog_handler.go`
- Create: `internal/http/channel_catalog_handler_test.go`
- Modify: `internal/app/app.go`

**Interfaces:**
- Produces: `ContentFamily`, `ListChannelDefinitions() []ChannelDefinition`, `FindChannelDefinition(string) (ChannelDefinition, bool)`, `IsRegisteredChannel(string) bool`, `RecommendedChannelIDs(string) []string`.
- Produces: `ChannelCatalogService.List(context.Context, string) ([]ChannelDefinition, error)` and authenticated `GET /api/channels.catalog?workspace_id=...`.

- [ ] **Step 1: Add failing registry and language tests**

```go
func TestChannelDefinitionsAreUniqueAndComplete(t *testing.T) {
    definitions := ListChannelDefinitions()
    require.NotEmpty(t, definitions)
    seen := map[string]bool{}
    for _, definition := range definitions {
        require.False(t, seen[definition.ID], definition.ID)
        seen[definition.ID] = true
        require.NotEmpty(t, definition.ContentFamilies)
        require.NotEmpty(t, definition.PreviewProfiles)
    }
    for _, id := range []string{"email", "sms", "push", "whatsapp", "telegram", "line", "zalo", "viber", "wechat_official_account", "wechat_mini_program", "wecom", "dingtalk", "feishu", "messenger", "instagram", "kakao", "rcs", "in_app", "webhook"} {
        require.True(t, seen[id], id)
    }
}

func TestTargetMarketRecommendations(t *testing.T) {
    assert.ElementsMatch(t, []string{"line", "messenger", "whatsapp"}, RecommendedChannelIDs("TH"))
    assert.Contains(t, RecommendedChannelIDs("VN"), "zalo")
    assert.Contains(t, RecommendedChannelIDs("PH"), "viber")
}
```

Extend `TestSupportedLanguages` to assert `kk`, `uz`, `fil` and `ur`.

- [ ] **Step 2: Run the focused domain tests and confirm failure**

Run: `go test ./internal/domain -run 'Test(ChannelDefinitions|TargetMarketRecommendations|SupportedLanguages)' -count=1`

Expected: compilation fails because catalogue functions are undefined and target locales are absent.

- [ ] **Step 3: Implement immutable registry values**

```go
type ChannelDefinition struct {
    ID                string           `json:"id"`
    LabelKey          string           `json:"label_key"`
    Regions           []string         `json:"regions"`
    RecommendedIn     []string         `json:"recommended_in"`
    ContentFamilies   []ContentFamily  `json:"content_families"`
    PreviewProfiles   []PreviewProfile `json:"preview_profiles"`
    DeliveryModes     []string         `json:"delivery_modes"`
    Limits            ChannelLimits    `json:"limits"`
}

type ContentFamily string

const (
    ContentFamilyText             ContentFamily = "text"
    ContentFamilyNotification     ContentFamily = "notification"
    ContentFamilyRichCard         ContentFamily = "rich_card"
    ContentFamilyCarousel         ContentFamily = "carousel"
    ContentFamilyExternalTemplate ContentFamily = "external_template"
    ContentFamilyWorkMessage      ContentFamily = "work_message"
    ContentFamilyWebhookPayload   ContentFamily = "webhook_payload"
)

type PreviewProfile struct {
    ID       string `json:"id"`
    LabelKey string `json:"label_key"`
    Surface  string `json:"surface"`
}
```

Return defensive copies from `ListChannelDefinitions`; normalize country lookups with `strings.ToUpper(strings.TrimSpace(country))`. Add `kk`, `uz`, `fil`, and `ur` to `SupportedLanguages`.

- [ ] **Step 4: Add authenticated service and handler tests**

```go
func TestChannelCatalogServiceRequiresWorkspaceAccess(t *testing.T) {
    auth := mocks.NewMockAuthService(ctrl)
    auth.EXPECT().AuthenticateUserForWorkspace(gomock.Any(), "ws1").Return(nil, nil, nil, errors.New("denied"))
    _, err := NewChannelCatalogService(auth).List(context.Background(), "ws1")
    require.ErrorContains(t, err, "authenticate channel catalogue")
}

func TestChannelCatalogHandlerReturnsDeterministicCatalogue(t *testing.T) {
    stub := &channelCatalogHTTPStub{definitions: []domain.ChannelDefinition{{ID: "line"}, {ID: "zalo"}}}
    handler := NewChannelCatalogHandler(stub, func() ([]byte, error) { return []byte("test-secret"), nil }, logger.NewLogger())
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ctx := context.WithValue(r.Context(), domain.UserIDKey, "user-1")
        handler.handleList(w, r.WithContext(ctx))
    }))
    defer server.Close()
    response, err := http.Get(server.URL + "?workspace_id=ws1")
    require.NoError(t, err)
    defer response.Body.Close()
    require.Equal(t, http.StatusOK, response.StatusCode)
    var payload struct{ Channels []domain.ChannelDefinition `json:"channels"` }
    require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
    assert.Equal(t, []string{"line", "zalo"}, []string{payload.Channels[0].ID, payload.Channels[1].ID})
}
```

- [ ] **Step 5: Implement service, handler and app wiring**

```go
type ChannelCatalogService interface {
    List(context.Context, string) ([]ChannelDefinition, error)
}

func (s *channelCatalogService) List(ctx context.Context, workspaceID string) ([]domain.ChannelDefinition, error) {
    if strings.TrimSpace(workspaceID) == "" { return nil, domain.NewValidationError("workspace_id is required") }
    if _, _, _, err := s.auth.AuthenticateUserForWorkspace(ctx, workspaceID); err != nil {
        return nil, fmt.Errorf("authenticate channel catalogue: %w", err)
    }
    return domain.ListChannelDefinitions(), nil
}
```

Register the handler beside `templateHandler` in `internal/app/app.go` and expose only GET.

- [ ] **Step 6: Run and inspect**

Run: `go test ./internal/domain ./internal/service ./internal/http -run 'Test(ChannelCatalog|ChannelDefinitions|TargetMarketRecommendations|SupportedLanguages)' -count=1`

Expected: PASS. Then run `git diff --check` and inspect only files listed in this task.

---

### Task 2: Typed generic material schema and validation

**Files:**
- Create: `internal/domain/channel_template_content.go`
- Create: `internal/domain/channel_template_content_test.go`
- Modify: `internal/domain/template.go`
- Modify: `internal/domain/template_test.go`

**Interfaces:**
- Consumes: `FindChannelDefinition` and `ContentFamily` from Task 1.
- Produces: `ChannelTemplateContent.ValidateForChannel(string) error`, `Template.Content`, `Template.ContentSchemaVersion`, and `TemplateTranslation.Content`.

- [ ] **Step 1: Add failing content-family tests**

```go
func TestChannelTemplateContentValidation(t *testing.T) {
    tests := []struct{name, channel string; content ChannelTemplateContent; want string}{
        {"whatsapp external template", "whatsapp", ChannelTemplateContent{Family: ContentFamilyExternalTemplate, ExternalTemplate: &ExternalTemplateReference{ID: "order_update", Language: "es_MX"}}, ""},
        {"telegram text", "telegram", ChannelTemplateContent{Family: ContentFamilyText, Body: "Hello {{ contact.first_name }}"}, ""},
        {"unsupported carousel", "webhook", ChannelTemplateContent{Family: ContentFamilyCarousel, Cards: []ChannelCard{{Title: "one"}, {Title: "two"}}}, "does not support carousel"},
        {"unknown channel", "not-real", ChannelTemplateContent{Family: ContentFamilyText, Body: "hello"}, "unknown template channel"},
    }
    for _, tt := range tests {
        err := tt.content.ValidateForChannel(tt.channel)
        if tt.want == "" { require.NoError(t, err) } else { require.ErrorContains(t, err, tt.want) }
    }
}
```

Add legacy template tests proving an SMS template with `Content == nil` still validates and a WhatsApp template requires schema version 1 plus matching `Content`.

- [ ] **Step 2: Verify the tests fail**

Run: `go test ./internal/domain -run 'Test(ChannelTemplateContent|Template.*Generic|Template.*Legacy)' -count=1`

Expected: compilation fails for undefined generic content types.

- [ ] **Step 3: Implement bounded typed content**

```go
const ChannelTemplateContentSchemaVersion = 1

type ChannelTemplateContent struct {
    Family           ContentFamily              `json:"family"`
    Title            string                     `json:"title,omitempty"`
    Body             string                     `json:"body,omitempty"`
    Footer           string                     `json:"footer,omitempty"`
    Media            *ChannelMedia              `json:"media,omitempty"`
    Actions          []ChannelAction            `json:"actions,omitempty"`
    Cards            []ChannelCard              `json:"cards,omitempty"`
    ExternalTemplate *ExternalTemplateReference `json:"external_template,omitempty"`
    Data             MapOfAny                   `json:"data,omitempty"`
    Webhook          *WebhookPayloadTemplate    `json:"webhook,omitempty"`
}
```

Use explicit limits: body 10,000 runes, title 512, footer 1,024, at most 10 actions, at most 10 cards, recursion depth 16, encoded custom data 128 KiB. Require HTTPS media URLs and one action target (`url`, `deep_link`, `reply`, or `phone`) per action. Validate allowed families through the registry.

- [ ] **Step 4: Extend Template validation without changing legacy semantics**

For `email`, `web`, `sms`, and `push`, call the existing typed validators and reject non-nil generic content. For every other registered channel, require `ContentSchemaVersion == 1`, non-nil `Content`, nil legacy content objects, and `Content.ValidateForChannel(Channel)`.

- [ ] **Step 5: Run and inspect**

Run: `go test ./internal/domain -run 'Test(ChannelTemplateContent|Template)' -count=1`

Expected: PASS, including the existing template test suite.

---

### Task 3: Migration v54 and generic content persistence

**Files:**
- Create: `internal/migrations/v54.go`
- Create: `internal/migrations/v54_test.go`
- Modify: `config/config.go`
- Modify: `internal/database/init.go`
- Modify: `internal/repository/template_postgres.go`
- Modify: `internal/repository/template_postgres_test.go`
- Modify: migration tests that pin `config.VERSION`

**Interfaces:**
- Consumes: `Template.Content` and `Template.ContentSchemaVersion` from Task 2.
- Produces: Workspace columns `content`, `content_schema_version`; table `channel_webhook_nonces`; repository round trips for content and translated content. The existing uncommitted Audience v53 migration remains unchanged.

- [ ] **Step 1: Add failing migration and repository tests**

```go
func TestV54MigrationUpdatesWorkspace(t *testing.T) {
    mock.ExpectExec("ALTER TABLE templates ADD COLUMN IF NOT EXISTS content JSONB").WillReturnResult(sqlmock.NewResult(0, 0))
    mock.ExpectExec("ALTER TABLE templates ADD COLUMN IF NOT EXISTS content_schema_version INTEGER").WillReturnResult(sqlmock.NewResult(0, 0))
    mock.ExpectExec("CREATE TABLE IF NOT EXISTS channel_webhook_nonces").WillReturnResult(sqlmock.NewResult(0, 0))
    require.NoError(t, (&V54Migration{}).UpdateWorkspace(ctx, cfg, workspace, mock))
}
```

Extend repository column and scan tests with `content` JSON and schema version 1; assert a translated generic content object round trips.

- [ ] **Step 2: Verify focused failure**

Run: `go test ./internal/migrations ./internal/repository -run 'TestV54|TestTemplateRepository' -count=1`

Expected: migration type and repository columns are missing.

- [ ] **Step 3: Implement additive v54 and fresh schema**

```sql
ALTER TABLE templates ADD COLUMN IF NOT EXISTS content JSONB;
ALTER TABLE templates ADD COLUMN IF NOT EXISTS content_schema_version INTEGER;
CREATE TABLE IF NOT EXISTS channel_webhook_nonces (
    integration_id VARCHAR(255) NOT NULL,
    nonce VARCHAR(128) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (integration_id, nonce)
);
CREATE INDEX IF NOT EXISTS idx_channel_webhook_nonces_expiry
    ON channel_webhook_nonces (expires_at);
```

Set the existing `config.VERSION` value from `53.0` to `54.0`, register v54, and update every migration-version assertion from `53.0` to `54.0` without changing the user's v53 migration body.

- [ ] **Step 4: Add content columns to every repository statement**

Update create, latest-version get, list, update, row scan and test fixtures in `template_postgres.go`. Marshal generic content directly through its typed pointer; use nullable schema version so legacy rows return zero in Go.

- [ ] **Step 5: Run migration and repository suites**

Run: `go test ./internal/migrations ./internal/repository -run 'Test(V54|Template)' -count=1`

Expected: PASS. Run `git diff --check`.

---

### Task 4: Generic server preview and platform diagnostics

**Files:**
- Create: `internal/service/channel_template_preview.go`
- Create: `internal/service/channel_template_preview_test.go`
- Modify: `internal/service/template_preview_service.go`
- Modify: `internal/service/template_service_test.go`
- Modify: `internal/domain/template.go`
- Modify: `internal/domain/template_test.go`
- Modify: `internal/http/template_handler_test.go`
- Modify: `openapi/components/schemas/template.yaml`

**Interfaces:**
- Consumes: typed content and registry from Tasks 1-2.
- Produces: `RenderedChannelMessage`, `GenericChannelPreview`, `PreviewTemplateRequest.Content`, `.ContentSchemaVersion`, `.Profile`, and `PreviewTemplateResponse.ChannelPreview`.

- [ ] **Step 1: Add failing preview tests**

```go
func TestRenderGenericChannelPreviewUsesLiquidAndProfile(t *testing.T) {
    request := domain.PreviewTemplateRequest{
        WorkspaceID: "ws1", Channel: "telegram", ContentSchemaVersion: 1,
        Content: &domain.ChannelTemplateContent{Family: domain.ContentFamilyText, Body: "Hi {{ customer.name }}"},
        Profile: "telegram_mobile", TestData: domain.MapOfAny{"customer": domain.MapOfAny{"name": "Ada"}},
    }
    preview, err := service.PreviewTemplate(systemContext(ctx), request)
    require.NoError(t, err)
    assert.Equal(t, "Ada", strings.TrimPrefix(preview.ChannelPreview.Message.Body, "Hi "))
    assert.Equal(t, "telegram_mobile", preview.ChannelPreview.Profile)
}

func TestPreviewReportsFallbackAndRTL(t *testing.T) {
    request := domain.PreviewTemplateRequest{
        WorkspaceID: "ws1", Channel: "telegram", ContentSchemaVersion: 1,
        Content: &domain.ChannelTemplateContent{Family: domain.ContentFamilyText, Body: "Default"},
        Translations: map[string]domain.TemplateTranslation{
            "ur": {Content: &domain.ChannelTemplateContent{Family: domain.ContentFamilyText, Body: "خوش آمدید"}},
        },
        Language: "ur", Profile: "telegram_mobile",
    }
    preview, err := service.PreviewTemplate(systemContext(ctx), request)
    require.NoError(t, err)
    assert.Equal(t, "rtl", preview.ChannelPreview.Direction)
    assert.False(t, preview.FallbackUsed)
}
```

Add table cases for WhatsApp external template, LINE card, Zalo external template, Webhook JSON payload, unsupported profile, over-limit warning, invalid Liquid and depth overflow.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/service ./internal/http -run 'Test(RenderGeneric|Preview.*Generic|Preview.*RTL|TemplateHandler.*Preview)' -count=1`

Expected: generic preview fields or renderer are undefined.

- [ ] **Step 3: Implement recursive rendering and structured response**

```go
type GenericChannelPreview struct {
    Profile      string                 `json:"profile"`
    Direction    string                 `json:"direction"`
    PayloadBytes int                    `json:"payload_bytes"`
    Message      RenderedChannelMessage `json:"message"`
    Warnings     []PreviewWarning       `json:"warnings"`
}
```

Render all visible strings through existing `renderPreviewString`, recursively render allowed data up to depth 16, calculate exact encoded JSON bytes, and return warnings defined by the selected channel/profile. Preserve existing SMS and Push responses unchanged.

- [ ] **Step 4: Extend translation resolution**

`Template.ResolveChannelContent(contactLanguage, workspaceDefaultLanguage)` returns translated generic content only when the translation contains `Content`; otherwise it returns default content and marks fallback explicitly. `ur` returns `rtl`; all current target locales return `ltr`.

- [ ] **Step 5: Run service and HTTP tests**

Run: `go test ./internal/domain ./internal/service ./internal/http -run 'Test(Preview|RenderGeneric|ChannelTemplate)' -count=1`

Expected: PASS with legacy SMS and Push preview tests unchanged.

---

### Task 5: Console catalogue client and country-aware channel picker

**Files:**
- Create: `console/src/services/api/channels.ts`
- Create: `console/src/services/api/channels.test.ts`
- Modify: `console/src/services/api/template.ts`
- Create: `console/src/components/templates/ChannelPicker.tsx`
- Create: `console/src/components/templates/ChannelPicker.test.tsx`

**Interfaces:**
- Consumes: `/api/channels.catalog` and generic template API fields.
- Produces: `ChannelDefinition`, `ChannelTemplateContent`, `GenericChannelPreview`, and `ChannelPicker` callback `onSelect(channelId: string)`.

- [ ] **Step 1: Add failing client and picker tests**

```tsx
it('shows Vietnam recommendations before all channels', async () => {
  render(<ChannelPicker definitions={definitions} country="VN" onSelect={onSelect} />)
  expect(screen.getByRole('button', { name: /Zalo/ })).toBeVisible()
  await user.click(screen.getByRole('tab', { name: /All channels/ }))
  expect(screen.getByRole('button', { name: /Telegram/ })).toBeVisible()
})
```

Assert the API client sends `workspace_id` and preserves server order. Assert delivery-mode badges distinguish native and signed Webhook.

- [ ] **Step 2: Verify Vitest failure**

Run from `console`: `npm test -- --run src/services/api/channels.test.ts src/components/templates/ChannelPicker.test.tsx`

Expected: imports fail because files do not exist.

- [ ] **Step 3: Implement strict TypeScript contracts and query client**

```ts
export interface ChannelDefinition {
  id: string
  label_key: string
  recommended_in: string[]
  content_families: ContentFamily[]
  preview_profiles: PreviewProfile[]
  delivery_modes: Array<'native' | 'signed_webhook'>
  limits: ChannelLimits
}
```

Do not use `any`. Extend `Template`, create/update/preview request and response with optional typed `content`, `content_schema_version`, `profile` and `channel_preview`.

- [ ] **Step 4: Implement picker with explicit states**

Render recommended/all tabs, search, channel cards, disabled state for empty catalogue and signed-Webhook badge. All copy uses Lingui macros.

- [ ] **Step 5: Run tests**

Run from `console`: `npm test -- --run src/services/api/channels.test.ts src/components/templates/ChannelPicker.test.tsx`

Expected: PASS.

---

### Task 6: Capability-aware omnichannel editor and live-preview lifecycle

**Files:**
- Create: `console/src/components/templates/OmnichannelFields.tsx`
- Create: `console/src/components/templates/OmnichannelFields.test.tsx`
- Create: `console/src/components/templates/OmnichannelTemplateDrawer.tsx`
- Create: `console/src/components/templates/OmnichannelTemplateDrawer.test.tsx`

**Interfaces:**
- Consumes: definitions and template API types from Task 5.
- Produces: create/edit/clone flows for all generic channels and a debounced preview request with stale-response suppression.

- [ ] **Step 1: Add failing editor tests**

```tsx
it('only exposes fields supported by the selected family', async () => {
  render(<OmnichannelFields definition={whatsApp} family="external_template" />)
  expect(screen.getByLabelText(/Platform template ID/)).toBeVisible()
  expect(screen.queryByLabelText(/Carousel cards/)).not.toBeInTheDocument()
})

it('does not let an older preview replace a newer draft', async () => {
  // Resolve the second mocked preview before the first and assert the second body remains visible.
})
```

Cover required field paths, invalid custom-data JSON, language tabs, translation fallback indicator, create payload and base-version update payload.

- [ ] **Step 2: Verify failure**

Run from `console`: `npm test -- --run src/components/templates/OmnichannelFields.test.tsx src/components/templates/OmnichannelTemplateDrawer.test.tsx`

Expected: imports fail.

- [ ] **Step 3: Implement focused field groups**

Use Ant Design `Form.List` for actions and cards, with registry limits controlling add buttons. Keep external template ID/language/parameters separate from normal rich content. Expose raw JSON only for `data` and `webhook_payload.body`.

- [ ] **Step 4: Implement debounced preview lifecycle**

```ts
const generation = ++previewGeneration.current
const result = await templatesApi.preview(payload)
if (generation === previewGeneration.current) setPreview(result)
```

Use the existing `use-debounce` dependency at 300ms, invalidate immediately on channel/family/language/profile change, and keep save independent from preview errors.

- [ ] **Step 5: Run editor tests**

Run from `console`: `npm test -- --run src/components/templates/OmnichannelFields.test.tsx src/components/templates/OmnichannelTemplateDrawer.test.tsx`

Expected: PASS.

---

### Task 7: Structured multi-client preview renderers

**Files:**
- Create: `console/src/components/templates/OmnichannelPreview.tsx`
- Create: `console/src/components/templates/OmnichannelPreview.test.tsx`
- Create: `console/src/components/templates/channelPreviews/PhoneChatPreview.tsx`
- Create: `console/src/components/templates/channelPreviews/DesktopChatPreview.tsx`
- Create: `console/src/components/templates/channelPreviews/NotificationPreview.tsx`
- Create: `console/src/components/templates/channelPreviews/EnterpriseMessagePreview.tsx`
- Create: `console/src/components/templates/channelPreviews/WebhookRequestPreview.tsx`
- Create: `console/src/components/templates/channelPreviews/clientProfiles.ts`
- Create: `console/src/components/templates/channelPreviews/clientProfiles.test.ts`
- Modify: `console/src/components/templates/ChannelMessagePreview.tsx`
- Modify: `console/src/components/templates/TemplatePreviewDrawer.tsx`
- Modify: `console/src/components/templates/PhonePreview.tsx`
- Create: `console/src/components/templates/EmailClientPreview.test.tsx`

**Interfaces:**
- Consumes: `GenericChannelPreview` and registry preview profiles.
- Produces: profile-routed simulated surfaces for all channels listed in the spec.

- [ ] **Step 1: Add failing renderer matrix tests**

```tsx
it.each([
  ['email', 'email_mobile'], ['email', 'email_desktop'],
  ['push', 'ios_lock_screen'], ['push', 'android_notification'], ['push', 'huawei_notification'],
  ['whatsapp', 'whatsapp_android'], ['telegram', 'telegram_desktop'],
  ['line', 'line_mobile'], ['zalo', 'zalo_mobile'], ['viber', 'viber_desktop'],
  ['wechat_official_account', 'wechat_mobile'], ['wecom', 'wecom_desktop'],
  ['dingtalk', 'dingtalk_mobile'], ['feishu', 'feishu_desktop'],
  ['messenger', 'messenger_web'], ['instagram', 'instagram_mobile'],
  ['kakao', 'kakao_mobile'], ['rcs', 'google_messages_rcs'], ['webhook', 'http_request']
])('renders %s profile %s', (channel, profile) => {
  render(<OmnichannelPreview definition={definition(channel)} preview={preview(profile)} />)
  expect(screen.getByText(/Simulated preview/)).toBeVisible()
})
```

Add assertions for long unbroken text, RTL body direction, missing media warning, carousel overflow, narrow width and profile switch callbacks.

- [ ] **Step 2: Verify failure**

Run from `console`: `npm test -- --run src/components/templates/OmnichannelPreview.test.tsx src/components/templates/channelPreviews/clientProfiles.test.ts src/components/templates/EmailClientPreview.test.tsx`

Expected: imports fail.

- [ ] **Step 3: Implement renderer primitives**

Render body strings as text, never `dangerouslySetInnerHTML`. Share structured primitives for bubbles, media, buttons, cards and carousels while keeping client chrome in small profile definitions. Use CSS logical properties so RTL changes content direction without mirroring non-text client controls.

- [ ] **Step 4: Route every declared profile**

`clientProfiles.ts` must export one `PreviewRendererKind` for every registry profile ID. A test compares the backend fixture list mirrored in the API test with the frontend map and fails on an unhandled profile.

Extend the existing Email preview drawer with `email_mobile` and `email_desktop` selectors around the same server-compiled Email HTML, and extend Push routing with iOS, Android, Web, Huawei, Honor, Xiaomi, OPPO and vivo chrome. The chrome is simulated; compiled message content remains authoritative.

- [ ] **Step 5: Run renderer tests**

Run from `console`: `npm test -- --run src/components/templates/OmnichannelPreview.test.tsx src/components/templates/channelPreviews/clientProfiles.test.ts src/components/templates/EmailClientPreview.test.tsx`

Expected: PASS with no clipping assertions failing in jsdom structure tests.

---

### Task 8: Unified Templates Page workflow

**Files:**
- Modify: `console/src/pages/TemplatesPage.tsx`
- Create: `console/src/pages/TemplatesPage.omnichannel.test.tsx`
- Modify: `console/src/components/templates/index.ts`
- Modify: `console/src/i18n/locales/zh-CN.po`
- Modify: generated locale files produced by Lingui commands
- Create or Modify: `console/e2e/features/templates.spec.ts`

**Interfaces:**
- Consumes: ChannelPicker, legacy Email drawer, omnichannel drawer and preview components.
- Produces: one create entry, channel/country filters, edit/clone routing and preserved legacy Email behavior.

- [ ] **Step 1: Add failing page tests**

```tsx
it('uses one create action and routes email to the legacy editor', async () => {
  renderPage()
  await user.click(screen.getByRole('button', { name: /Create template/ }))
  await user.click(screen.getByRole('button', { name: /Email/ }))
  expect(screen.getByText(/Create Email Template/)).toBeVisible()
  expect(screen.queryByRole('button', { name: /Create SMS \/ Push/ })).not.toBeInTheDocument()
})
```

Cover filtered empty state, channel badge, country recommendation filter, generic edit, clone and permission-disabled create.

- [ ] **Step 2: Verify failure**

Run from `console`: `npm test -- --run src/pages/TemplatesPage.omnichannel.test.tsx`

Expected: current page still has separate create buttons.

- [ ] **Step 3: Integrate one creation flow**

Keep the mature `CreateTemplateDrawer` for Email after selection. Route SMS and Push to their legacy-compatible shared editor and every generic channel to `OmnichannelTemplateDrawer`. Replace the three hard-coded list calls with one `templatesApi.list({ workspace_id: workspaceId, category })` request whose empty channel filter returns all registered channels.

- [ ] **Step 4: Regenerate translations and add E2E path**

Run from `console`: `npm run lingui:extract` then translate new `zh-CN.po` entries and run `npm run lingui:compile`.

Playwright flow creates a Telegram template, types Liquid-backed content, switches mobile/desktop profiles, saves, reopens, filters to an empty country/channel combination and verifies the reset action.

- [ ] **Step 5: Run frontend regression**

Run from `console`: `npm test -- --run src/pages/TemplatesPage.omnichannel.test.tsx src/components/templates`

Expected: PASS.

---

### Task 9: Encrypted signed-channel Webhook integration

**Files:**
- Create: `internal/domain/channel_webhook.go`
- Create: `internal/domain/channel_webhook_test.go`
- Modify: `internal/domain/workspace.go`
- Modify: `internal/domain/workspace_test.go`
- Modify: `console/src/services/api/workspace.ts`

**Interfaces:**
- Consumes: registered channel IDs.
- Produces: `IntegrationTypeChannelWebhook`, `ChannelWebhookSettings`, encrypted secret lifecycle and update-preservation behavior.

- [ ] **Step 1: Add failing domain tests**

```go
func TestChannelWebhookSettingsSecretLifecycle(t *testing.T) {
    settings := ChannelWebhookSettings{EndpointURL: "https://bridge.example/send", Secret: "plain-secret", Channels: []string{"telegram", "zalo"}, TimeoutSeconds: 5}
    require.NoError(t, settings.Validate("master-key"))
    require.NoError(t, settings.EncryptSecretKeys("master-key"))
    assert.Empty(t, settings.Secret)
    assert.NotEmpty(t, settings.EncryptedSecret)
    require.NoError(t, settings.DecryptSecretKeys("master-key"))
    assert.Equal(t, "plain-secret", settings.Secret)
}
```

Add cases for HTTP URL rejection, embedded credentials, legacy/native channel disallow list, unknown channel, duplicate channel, timeout outside 1-30 seconds, secret redaction and omitted-secret update preservation.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/domain -run 'Test(ChannelWebhook|Integration.*ChannelWebhook)' -count=1`

Expected: types are undefined.

- [ ] **Step 3: Implement settings and Workspace integration hooks**

```go
type ChannelWebhookSettings struct {
    EndpointURL    string            `json:"endpoint_url"`
    Secret         string            `json:"secret,omitempty"`
    EncryptedSecret string           `json:"encrypted_secret,omitempty"`
    Channels       []string          `json:"channels"`
    TimeoutSeconds int               `json:"timeout_seconds"`
    Headers        map[string]string `json:"headers,omitempty"`
}
```

Wire Validate, BeforeSave, AfterLoad, Redact, credential hints and `PreserveOmitted` through all create/update/service paths. Forbid `Authorization`, `Cookie`, `Host`, signature header names and case-insensitive duplicates in custom headers.

- [ ] **Step 4: Extend TypeScript integration contracts**

Add `'channel_webhook'` to `IntegrationType`, `channel_webhook_settings` to Integration/Create/Update requests, and a redacted credential hint only; never type a plaintext secret on response objects as required.

- [ ] **Step 5: Run domain tests**

Run: `go test ./internal/domain -run 'Test(ChannelWebhook|Integration|CreateIntegration|UpdateIntegration)' -count=1`

Expected: PASS.

---

### Task 10: Generic endpoints and bounded signed Webhook provider

**Files:**
- Modify: `internal/domain/contact_endpoint.go`
- Modify: `internal/domain/contact_endpoint_test.go`
- Modify: `internal/domain/channel_provider.go`
- Create: `internal/service/channel_webhook_provider.go`
- Create: `internal/service/channel_webhook_provider_test.go`
- Modify: `internal/service/channel_message_service.go`
- Modify: `internal/service/channel_message_service_test.go`

**Interfaces:**
- Consumes: rendered generic message and channel Webhook settings.
- Produces: provider `channel_webhook`, canonical envelope signing, bounded response parsing and generic endpoint selection.

- [ ] **Step 1: Add failing endpoint and provider tests**

```go
func TestGenericChannelWebhookEndpoint(t *testing.T) {
    endpoint, err := (ContactEndpointMutation{Operation: "upsert", EndpointID: "tg-1", Channel: "telegram", Provider: "channel_webhook", Platform: "telegram_mobile", Address: "chat-123"}).Validate()
    require.NoError(t, err)
    assert.Equal(t, "telegram", endpoint.Channel)
}

func TestSignedWebhookProviderCanonicalSignature(t *testing.T) {
    // Fixed clock and nonce; assert exact body bytes and v1 HMAC header against an independently calculated digest.
}
```

Provider table cases cover accepted, rejected, retryable with bounded `Retry-After`, 5xx, oversized response, cross-origin redirect, timeout before connection and ambiguous EOF after request transmission.

- [ ] **Step 2: Verify failure**

Run: `go test ./internal/domain ./internal/service -run 'Test(GenericChannelWebhookEndpoint|SignedWebhookProvider)' -count=1`

Expected: generic endpoint is rejected and provider is undefined.

- [ ] **Step 3: Extend endpoint validation**

Keep existing Twilio/FCM/APNs/Web Push branches byte-for-byte semantically equivalent. For provider `channel_webhook`, require a registered non-native-new channel, profile compatibility when a platform is provided, and opaque address length 1-4096.

- [ ] **Step 4: Implement canonical provider**

```go
func SignChannelWebhook(secret string, timestamp int64, nonce string, body []byte) string {
    canonical := strconv.FormatInt(timestamp, 10) + "." + nonce + "." + string(body)
    mac := hmac.New(sha256.New, []byte(secret))
    _, _ = mac.Write([]byte(canonical))
    return "v1=" + hex.EncodeToString(mac.Sum(nil))
}
```

Use exact marshaled body bytes for signing and sending. Set `X-Yaoguang-Timestamp`, `X-Yaoguang-Nonce`, `X-Yaoguang-Effect-Key`, and `X-Yaoguang-Signature`. Cap response to 64 KiB, disable redirects, use configured timeout, and map only explicit response states.

- [ ] **Step 5: Generalize resolver and delivery request**

Extend `ChannelDeliveryRequest` with `Generic *RenderedChannelMessage`, template/version, platform, locale and bounded metadata. `WorkspaceChannelProviderResolver.Resolve` returns the signed provider only when integration type, allowed channel and decrypted settings match.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/domain ./internal/service -run 'Test(ContactEndpoint|SignedWebhookProvider|ChannelMessageService)' -count=1`

Expected: PASS including existing Twilio and FCM tests.

---

### Task 11: Generic send path, distributed Nonce reservation and signed receipts

**Files:**
- Create: `internal/repository/channel_webhook_nonce_postgres.go`
- Create: `internal/repository/channel_webhook_nonce_postgres_test.go`
- Create: `internal/service/channel_webhook_receipt.go`
- Create: `internal/service/channel_webhook_receipt_test.go`
- Modify: `internal/domain/delivery_receipt.go`
- Modify: `internal/domain/channel_message.go`
- Modify: `internal/service/channel_message_service.go`
- Modify: `internal/service/channel_message_service_test.go`
- Modify: `internal/http/delivery_receipt_handler.go`
- Modify: `internal/http/delivery_receipt_handler_test.go`
- Modify: `internal/app/app.go`
- Modify: `console/src/services/api/channel_messages.ts`

**Interfaces:**
- Consumes: v54 nonce table, generic provider and content preview.
- Produces: all registered generic channels through `/api/channelMessages.send`; public `POST /webhooks/delivery/channel`; replay-safe receipt normalization.

- [ ] **Step 1: Add failing nonce repository tests**

```go
func TestReserveChannelWebhookNonce(t *testing.T) {
    mock.ExpectExec("INSERT INTO channel_webhook_nonces").WithArgs("bridge-1", "nonce-1", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
    reserved, err := repo.Reserve(ctx, "ws1", "bridge-1", "nonce-1", now.Add(5*time.Minute))
    require.NoError(t, err)
    assert.True(t, reserved)
}
```

Add duplicate false result and bounded expired-row cleanup tests.

- [ ] **Step 2: Add failing send and receipt tests**

Generic send test loads a Telegram template, endpoint and Webhook integration, asserts the server preview is used, reserves the stable effect key and writes provider/message history. Receipt tests independently sign exact bytes, assert accepted normalization, stale timestamp rejection, invalid signature, repeated Nonce conflict and receipt payload-hash conflict.

- [ ] **Step 3: Verify failure**

Run: `go test ./internal/repository ./internal/service ./internal/http -run 'Test(ChannelWebhookNonce|ChannelWebhookReceipt|ChannelMessageService.*Generic|DeliveryReceiptHandler.*Channel)' -count=1`

Expected: repository and receipt route are undefined; generic send validation rejects the channel.

- [ ] **Step 4: Generalize request validation and rendering**

`SendChannelMessageRequest.Validate` accepts every registered sendable channel. SMS and Push keep their current content branches. New channels pass stored `Template.Content` and endpoint platform through `PreviewTemplate`, then supply `ChannelPreview.Message` to the provider. The stable request hash includes channel, resolved endpoint, template version and language as it does today.

- [ ] **Step 5: Implement nonce repository and receipt verification**

Reserve the Nonce in the Workspace database only after timestamp and signature checks and before receipt application. Verify with `hmac.Equal`; accept at most five minutes of clock skew; parse at most 256 KiB; map `accepted`, `sent`, `delivered`, `opened`, `clicked`, and `failed` to existing receipt events.

- [ ] **Step 6: Register route and dependencies**

Add `/webhooks/delivery/channel?workspace_id=...&integration_id=...` to `DeliveryReceiptHandler`. Apply existing receipt rate limiting by IP and Workspace. Wire nonce repository and receipt verifier in `internal/app/app.go` without changing Twilio route construction.

- [ ] **Step 7: Run layer tests**

Run: `go test ./internal/repository ./internal/service ./internal/http -run 'Test(ChannelWebhook|ChannelMessage|DeliveryReceipt)' -count=1`

Expected: PASS, including existing Twilio receipt behavior.

---

### Task 12: Channel Webhook settings UI and final acceptance

**Files:**
- Create: `console/src/components/integrations/ChannelWebhookIntegration.tsx`
- Create: `console/src/components/integrations/ChannelWebhookIntegration.test.tsx`
- Modify: `console/src/components/settings/Integrations.tsx`
- Modify: `console/src/services/api/permissions.ts`
- Modify: `console/src/i18n/locales/zh-CN.po`
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Modify: `openapi.json` or its generated source files according to the repository OpenAPI workflow

**Interfaces:**
- Consumes: catalogue client and Workspace integration API.
- Produces: owner configuration, edit-with-secret-preservation, channel allow-list and delivery-mode visibility.

- [ ] **Step 1: Add failing settings tests**

```tsx
it('edits a channel Webhook without sending a blank replacement secret', async () => {
  render(<ChannelWebhookIntegration integration={existing} definitions={definitions} />)
  await user.clear(screen.getByLabelText(/Name/))
  await user.type(screen.getByLabelText(/Name/), 'Regional bridge')
  await user.click(screen.getByRole('button', { name: /Save/ }))
  expect(workspacesApi.updateIntegration).toHaveBeenCalledWith(expect.not.objectContaining({
    channel_webhook_settings: expect.objectContaining({ secret: '' })
  }))
})
```

Cover create, HTTPS validation, timeout, allowed-channel selection, credential hint and non-owner read-only behavior.

- [ ] **Step 2: Verify failure**

Run from `console`: `npm test -- --run src/components/integrations/ChannelWebhookIntegration.test.tsx`

Expected: component is undefined.

- [ ] **Step 3: Implement focused integration card and drawer**

Keep `Integrations.tsx` responsible only for routing/open state; put all Webhook form behavior in the new component. List only catalogue channels whose delivery modes include `signed_webhook`. Omit `secret` entirely on edit unless the user enters a replacement.

- [ ] **Step 4: Align permissions, docs and translations**

Update API permission descriptions so `channelMessages.send` names generic channels and the external side effect. Document simulated previews and signed-Webhook first-phase semantics in README and a new unreleased v54 feature bullet without altering the existing v53 Audience entry. Run Lingui extraction, translate new Simplified Chinese entries, compile catalogues, then run `make openapi-bundle` and `make openapi-lint` after updating the source YAML.

- [ ] **Step 5: Run focused backend suites**

Run: `go test ./internal/domain ./internal/service ./internal/repository ./internal/http ./internal/migrations -count=1`

Expected: PASS.

- [ ] **Step 6: Run full backend verification**

Run: `make test-unit`

Expected: PASS. Run `make test-integration` when the repository's PostgreSQL/Redis/RabbitMQ integration environment is available; report an environment gate separately from test failures.

- [ ] **Step 7: Run complete frontend verification**

From `console`, run:

```powershell
npm run lingui:extract
npm run lingui:compile
npm test -- --run
npm run build
npm run lint
```

Expected: every command exits 0.

- [ ] **Step 8: Perform real browser acceptance**

Start the existing development stack without overwriting the user's `.env` or `.air.toml`. In a real browser verify:

1. one create action opens country/channel selection;
2. Thailand recommends LINE, Vietnam recommends Zalo and Philippines recommends Viber;
3. create and preview Telegram text in mobile and desktop profiles;
4. create WhatsApp external-template material and see approval-reference diagnostics;
5. switch Push among iOS, Android, Web and domestic OEM profiles;
6. enter Urdu and confirm RTL content;
7. save, reopen, edit and clone a generic template;
8. configure a signed channel Webhook while the secret remains redacted on reopen;
9. exercise filtered-empty and preview-error states;
10. inspect narrow viewport screenshots for overlap, clipping and inaccessible controls.

- [ ] **Step 9: Final diff and safety audit**

Run:

```powershell
git diff --check
git status --short
git diff --stat
```

Confirm no unrelated dirty file was overwritten, no secret or rendered customer content appears in fixtures/logs, no branch or commit was created, and distinguish focused tests, full tests, integration gates and browser acceptance in the final report.
