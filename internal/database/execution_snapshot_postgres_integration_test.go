package database_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/repository"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	pkgcrypto "github.com/hengshu-credit/yaoguang-marketing/pkg/crypto"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/postgresdriver"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestExecutionSnapshotPostgresImmutableRunsAndJourney(t *testing.T) {
	dsn := os.Getenv("SNAPSHOT_POSTGRES_ADMIN_DSN")
	if dsn == "" {
		t.Skip("SNAPSHOT_POSTGRES_ADMIN_DSN is not configured")
	}
	ctx := context.Background()
	admin, err := sql.Open(postgresdriver.Name, dsn)
	require.NoError(t, err)
	defer admin.Close()
	require.NoError(t, admin.PingContext(ctx))
	name := "notifuse_snapshots_test_" + uuid.NewString()[:8]
	_, err = admin.Exec(`CREATE DATABASE "` + name + `"`)
	require.NoError(t, err)
	defer func() {
		_, _ = admin.Exec(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1`, name)
		_, _ = admin.Exec(`DROP DATABASE "` + name + `"`)
	}()
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	parsed.Path = "/" + name
	db, err := sql.Open(postgresdriver.Name, parsed.String())
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, database.InitializeWorkspaceDatabase(db))
	for _, query := range append(append(schema.ExecutionSnapshotTableDefinitions(), schema.ExecutionRuntimeSnapshotDefinitions()...), schema.ExecutionEventMetadataDefinitions()...) {
		_, err = db.Exec(query)
		require.NoError(t, err, query)
	}
	_, err = db.Exec(`INSERT INTO lists(id,name,is_double_optin,is_public) VALUES('snapshot-list','Snapshot list',false,false)`)
	require.NoError(t, err)
	campaignRepo := repository.NewCampaignRepositoryWithDB(db)
	snapshotRepo := repository.NewExecutionSnapshotRepositoryWithDB(db)
	campaignID := uuid.NewString()
	config := domain.CampaignConfiguration{ListID: "snapshot-list", Channel: "email", Variants: []domain.CampaignVariant{{ID: "a", WeightBP: 10000}}, Settings: json.RawMessage(`{"filters":{"product":"A"},"metadata":{"tracking_plan":{"subject_type":"loan","events":["borrow"],"states":[],"window_seconds":3600}}}`)}
	require.NoError(t, campaignRepo.CreateCurrentCampaign(ctx, "ws", domain.Campaign{ID: campaignID, Name: "Current", CreatedAt: time.Now().UTC()}, config))
	starter, err := service.NewCampaignSnapshotService(campaignRepo, 100)
	require.NoError(t, err)
	starter.SetBusinessDefinitions(&snapshotTrackingDefinitions{})
	first, err := starter.StartAsync(ctx, "ws", campaignID, 0)
	require.NoError(t, err)
	config.Settings = json.RawMessage(`{"filters":{"product":"B"}}`)
	require.NoError(t, campaignRepo.SaveCurrentCampaignConfiguration(ctx, "ws", campaignID, "Edited", config))
	second, err := starter.StartAsync(ctx, "ws", campaignID, 0)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	original, err := snapshotRepo.Get(ctx, "ws", first.ID)
	require.NoError(t, err)
	require.Contains(t, string(original.Configuration), `"product": "A"`)
	changed, err := snapshotRepo.Get(ctx, "ws", second.ID)
	require.NoError(t, err)
	require.Contains(t, string(changed.Configuration), `"product": "B"`)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM campaign_versions WHERE campaign_id=$1`, campaignID).Scan(&count))
	require.Zero(t, count)
	_, err = db.Exec(`UPDATE execution_config_snapshots SET configuration='{}' WHERE execution_id=$1`, first.ID)
	require.Error(t, err)
	customerID := uuid.NewString()
	_, err = db.Exec(`INSERT INTO customers(id,customer_no) VALUES($1,'snapshot-customer')`, customerID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO contacts(email,customer_id) VALUES('snapshot@example.org',$1)`, customerID)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO templates(id,name,version,channel,email,category) VALUES('snapshot-template','Original',1,'email','{"subject":"Original","compiled_html":"<p>Hello</p>"}','marketing')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO automations(id,workspace_id,name,status,trigger_config,root_node_id,nodes,version) VALUES('snapshot-automation','ws','Original journey','live','{"event_kind":"contact.updated","frequency":"once"}','email-node','[{"id":"email-node","type":"email","config":{"template_id":"snapshot-template","template_version":0}}]',1)`)
	require.NoError(t, err)
	archivedTemplate := &domain.Template{ID: "snapshot-template", Channel: "email", Email: &domain.EmailTemplate{Subject: "Original", CompiledPreview: `<img src="https://cdn.example.org/original.png">`}, Translations: map[string]domain.TemplateTranslation{}}
	templateEmail, _ := json.Marshal(archivedTemplate.Email)
	_, err = db.Exec(`UPDATE templates SET email=$1,translations='{}' WHERE id='snapshot-template'`, templateEmail)
	require.NoError(t, err)
	backlog, backlogErr := snapshotRepo.UnarchivedAutomationTemplates(ctx, "ws", 25)
	require.NoError(t, backlogErr)
	require.Len(t, backlog, 1)
	require.Equal(t, "snapshot-template", backlog[0].ID)
	var rejectedOutcome, rejectedID string
	var rejectedRetry sql.NullTime
	err = db.QueryRow(`SELECT outcome,contact_automation_id,retry_at FROM automation_enroll_customer('snapshot-automation',$1,'snapshot@example.org','email-node','once',NULL,'{"enabled":false}',1,'realtime')`, customerID).Scan(&rejectedOutcome, &rejectedID, &rejectedRetry)
	require.ErrorContains(t, err, "must be archived")
	bytes := []byte("original immutable image")
	hash := sha256.Sum256(bytes)
	hashString := hex.EncodeToString(hash[:])
	require.NoError(t, snapshotRepo.SaveAsset(ctx, "ws", domain.ExecutionAsset{Hash: hashString, MediaType: "image/png", Data: bytes}))
	assetManifest, _ := json.Marshal([]map[string]string{{"source_url": "https://cdn.example.org/original.png", "hash": hashString, "media_type": "image/png"}})
	require.NoError(t, snapshotRepo.SaveTemplateMaterialArchive(ctx, "ws", archivedTemplate, assetManifest))
	runtimeConfig := json.RawMessage(`{"email-node":{"template_id":"snapshot-template","template_version":0}}`)
	runtimeCipher, err := pkgcrypto.EncryptString(`{"email-node":{"template_id":"snapshot-template","template_version":0},"__execution_providers":{"integrations":[],"marketing_email_provider_id":"original-provider"}}`, "snapshot-secret")
	require.NoError(t, err)
	require.NoError(t, snapshotRepo.SaveRuntimeNodes(ctx, "ws", "snapshot-automation", runtimeConfig, runtimeCipher, true))
	var outcome, contactAutomationID string
	var retry sql.NullTime
	require.NoError(t, db.QueryRow(`SELECT outcome,contact_automation_id,retry_at FROM automation_enroll_customer('snapshot-automation',$1,'snapshot@example.org','email-node','once',NULL,'{"enabled":false}',1,'realtime')`, customerID).Scan(&outcome, &contactAutomationID, &retry))
	require.Equal(t, "enrolled", outcome)
	journey, err := snapshotRepo.Get(ctx, "ws", contactAutomationID)
	require.NoError(t, err)
	require.Contains(t, string(journey.Materials), "Original")
	var providerDeps map[string]any
	require.NoError(t, json.Unmarshal(journey.Dependencies, &providerDeps))
	oldProviderRef := providerDeps["runtime_nodes_ref"].(string)
	newCipher, err := pkgcrypto.EncryptString(`{"__execution_providers":{"integrations":[],"marketing_email_provider_id":"new-provider"}}`, "snapshot-secret")
	require.NoError(t, err)
	require.NoError(t, snapshotRepo.SaveRuntimeNodes(ctx, "ws", "snapshot-automation", runtimeConfig, newCipher, true))
	oldCipher, err := snapshotRepo.RuntimeNodes(ctx, "ws", oldProviderRef)
	require.NoError(t, err)
	require.Equal(t, runtimeCipher, oldCipher, "waiting run retains immutable credentials after another activation")
	var latestProviderRef string
	require.NoError(t, db.QueryRow(`SELECT id::text FROM execution_runtime_credentials WHERE automation_id='snapshot-automation' ORDER BY created_at DESC,id DESC LIMIT 1`).Scan(&latestProviderRef))
	require.NotEqual(t, oldProviderRef, latestProviderRef)
	_, err = db.Exec(`INSERT INTO automations(id,workspace_id,name,status,trigger_config,root_node_id,nodes,version) VALUES('missing-pinned','ws','Missing pinned version','live','{"event_kind":"contact.updated","frequency":"once"}','one','[{"id":"one","type":"delay","config":{"template_id":"snapshot-template","template_version":0}},{"id":"two","type":"delay","config":{"template_id":"snapshot-template","template_version":99}}]',1)`)
	require.NoError(t, err)
	err = db.QueryRow(`SELECT outcome,contact_automation_id,retry_at FROM automation_enroll_customer('missing-pinned',$1,'snapshot@example.org','one','once',NULL,'{"enabled":false}',1,'realtime')`, customerID).Scan(&outcome, &contactAutomationID, &retry)
	require.ErrorContains(t, err, "template snapshot is incomplete")
	_, err = db.Exec(`UPDATE automations SET nodes='[]',version=2 WHERE id='snapshot-automation'`)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM templates WHERE id='snapshot-template'`)
	require.NoError(t, err)
	persisted, err := snapshotRepo.Get(ctx, "ws", contactAutomationID)
	require.NoError(t, err)
	require.Contains(t, string(persisted.Materials), "Original")
	require.Equal(t, "complete", persisted.Completeness)
	require.Contains(t, string(persisted.Materials), hashString)
	archived, archiveErr := snapshotRepo.GetAsset(ctx, "ws", hashString)
	require.NoError(t, archiveErr)
	require.Equal(t, bytes, archived.Data)
	automationRepo := repository.NewAutomationRepositoryWithDB(db, nil)
	claim, acquired, err := automationRepo.ClaimContactAutomation(ctx, "ws", contactAutomationID, "snapshot-test", time.Now().UTC(), time.Minute)
	require.NoError(t, err)
	require.True(t, acquired, "waiting instances with snapshots must survive workflow edits")
	require.NotNil(t, claim)
	payload, _ := json.Marshal(domain.EmailQueuePayload{ExecutionID: journey.ExecutionID, Subject: "Hello Alice", HTMLContent: "<p>Actual personalized content</p>", ProviderSettings: map[string]any{"password": "SECRET"}})
	_, err = db.Exec(`INSERT INTO email_queue(id,source_type,source_id,integration_id,provider_kind,contact_email,message_id,template_id,payload) VALUES('snapshot-queue','automation','snapshot-automation','provider','smtp','snapshot@example.org','message-1','snapshot-template',$1)`, payload)
	require.NoError(t, err)
	evidence, _, err := snapshotRepo.ListEvidence(ctx, "ws", journey.ExecutionID, "", 100)
	require.NoError(t, err)
	require.Len(t, evidence, 2)
	require.Contains(t, string(evidence[1].Payload), "Hello Alice")
	require.NotContains(t, string(evidence[1].Payload), "SECRET")
	require.Equal(t, "pending", evidence[1].Status)
	require.NoError(t, repository.NewEmailQueueRepositoryWithDB(db).MarkAsSent(ctx, "ws", "snapshot-queue"))
	evidence, _, err = snapshotRepo.ListEvidence(ctx, "ws", journey.ExecutionID, "", 100)
	require.NoError(t, err)
	require.Equal(t, "confirmed", evidence[len(evidence)-1].Status)
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM email_queue WHERE id='snapshot-queue'`).Scan(&count))
	require.Zero(t, count, "confirmed queue entry must be removed")
	causationID := uuid.NewString()
	metadata, _ := json.Marshal(map[string]any{"metadata": map[string]any{"execution_id": journey.ExecutionID, "causation_id": causationID}})
	_, err = db.Exec(`INSERT INTO message_history(id,contact_email,automation_id,template_id,template_version,channel,message_data,sent_at) VALUES('causal-message','snapshot@example.org','snapshot-automation','snapshot-template',1,'email',$1,CURRENT_TIMESTAMP)`, metadata)
	require.NoError(t, err)
	var eventExecution, eventCausation sql.NullString
	require.NoError(t, db.QueryRow(`SELECT properties->>'execution_id',context->>'causation_id' FROM event_ledger WHERE subject_type='message_history' AND subject_id='causal-message' ORDER BY received_at DESC LIMIT 1`).Scan(&eventExecution, &eventCausation))
	require.Equal(t, journey.ExecutionID, eventExecution.String)
	require.Equal(t, causationID, eventCausation.String)
	var envelopeCause, envelopeExecution string
	require.NoError(t, db.QueryRow(`SELECT payload->>'causation_id',payload->'data'->>'execution_id' FROM event_outbox WHERE payload->'subject'->>'id'='causal-message' LIMIT 1`).Scan(&envelopeCause, &envelopeExecution))
	require.Equal(t, causationID, envelopeCause)
	require.Equal(t, journey.ExecutionID, envelopeExecution)
	unrelatedID := uuid.NewString()
	_, err = db.Exec(`INSERT INTO event_ledger(id,event_type,subject_type,subject_id,source,schema_version,occurred_at,received_at,properties,context) VALUES($1,'loan.submitted','loan','loan-1','reporting_api',1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'{"amount":5}','{"request":"original"}')`, unrelatedID)
	require.NoError(t, err)
	var unrelatedProperties, unrelatedContext string
	require.NoError(t, db.QueryRow(`SELECT properties::text,context::text FROM event_ledger WHERE id=$1`, unrelatedID).Scan(&unrelatedProperties, &unrelatedContext))
	require.JSONEq(t, `{"amount":5}`, unrelatedProperties)
	require.JSONEq(t, `{"request":"original"}`, unrelatedContext)
	_, err = db.Exec(`INSERT INTO campaign_recipient_snapshots(run_id,ordinal,customer_id,variant) VALUES($1,1,$2,'a')`, first.ID, customerID)
	require.NoError(t, err)
	firstEvidence, _, err := snapshotRepo.ListEvidence(ctx, "ws", first.ID, "", 100)
	require.NoError(t, err)
	require.Len(t, firstEvidence, 1)
	require.Equal(t, "audience", firstEvidence[0].Kind)
	require.Contains(t, string(firstEvidence[0].Payload), "selected")
	intentMetadata, _ := json.Marshal(map[string]any{"execution_id": first.ID, "touchpoint_id": "rejected-message"})
	_, err = db.Exec(`INSERT INTO delivery_intents(effect_key,request_hash,source_type,source_id,source_version,customer_id,channel,node_or_phase,occurrence,variant,status,suppression_reason,metadata) VALUES(repeat('a',64),repeat('b',64),'campaign',$1,'1',$2,'email','main','1','a','suppressed','unsubscribed',$3)`, campaignID, customerID, intentMetadata)
	require.NoError(t, err)
	firstEvidence, _, err = snapshotRepo.ListEvidence(ctx, "ws", first.ID, "", 100)
	require.NoError(t, err)
	require.Len(t, firstEvidence, 2)
	require.Equal(t, "eligibility", firstEvidence[1].Kind)
	require.Contains(t, string(firstEvidence[1].Payload), "unsubscribed")
	for _, eventID := range []string{uuid.NewString(), uuid.NewString()} {
		_, err = db.Exec(`INSERT INTO event_ledger(id,event_type,subject_type,subject_id,customer_id,source,schema_version,occurred_at,received_at,properties,context) VALUES($1,'borrow','loan','loan-1',$2,'reporting_api',1,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'{}','{}')`, eventID, customerID)
		require.NoError(t, err)
	}
	observations, err := snapshotRepo.Observations(ctx, "ws", original, time.Now().UTC())
	require.NoError(t, err)
	require.Len(t, observations.Events, 1)
	require.Equal(t, 2, observations.Events[0].Observed.Occurrences)
	require.Equal(t, 1, observations.Events[0].Observed.Customers)
	require.Equal(t, 1, observations.Events[0].Observed.Objects)
	require.Zero(t, observations.Events[0].Attributed.Occurrences)
	for _, query := range schema.CustomerChannelDefinitions() {
		_, err = db.Exec(query)
		require.NoError(t, err, query)
	}
	noEmailID := uuid.NewString()
	_, err = db.Exec(`INSERT INTO customers(id,customer_no) VALUES($1,'no-email')`, noEmailID)
	require.NoError(t, err)
	encrypted, err := pkgcrypto.EncryptString("+15557654321", "snapshot-secret")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO contact_endpoints(endpoint_id,customer_id,email,channel,provider,platform,address_ciphertext,address_fingerprint) VALUES('sms-no-email',$1,NULL,'sms','twilio','phone',$2,repeat('c',64))`, noEmailID, encrypted)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO customer_consents(id,customer_id,purpose,channel,status) VALUES($1,$2,'marketing','sms','granted')`, uuid.NewString(), noEmailID)
	require.NoError(t, err)
	endpointWriter, err := repository.NewContactEndpointRepositoryWithDB(db, "snapshot-secret")
	require.NoError(t, err)
	granted := true
	require.NoError(t, endpointWriter.SaveCustomerEndpoint(ctx, &domain.CustomerEndpointRequest{WorkspaceID: "ws", CustomerID: noEmailID, Endpoint: domain.ContactEndpointMutation{Operation: "upsert", EndpointID: "sms-via-api", Channel: "sms", Provider: "twilio", Platform: "phone", Address: "+15557654321"}, MarketingConsent: &granted}))
	publicEndpoints, err := endpointWriter.ListCustomerEndpoints(ctx, "ws", noEmailID, "sms")
	require.NoError(t, err)
	require.Len(t, publicEndpoints, 2)
	for _, endpoint := range publicEndpoints {
		require.Empty(t, endpoint.Address)
	}
	require.ErrorContains(t, endpointWriter.SaveCustomerEndpoint(ctx, &domain.CustomerEndpointRequest{WorkspaceID: "ws", CustomerID: customerID, Endpoint: domain.ContactEndpointMutation{Operation: "upsert", EndpointID: "sms-via-api", Channel: "sms", Provider: "twilio", Platform: "phone", Address: "+15550001234"}}), "belongs to another customer")
	workspaceReader := &snapshotChannelWorkspace{db: db}
	endpointRepo, err := repository.NewContactEndpointRepositoryWithDB(db, "snapshot-secret")
	require.NoError(t, err)
	sendRepo, err := repository.NewChannelSendRepository(workspaceReader)
	require.NoError(t, err)
	provider := &snapshotSMSProvider{}
	channel, err := service.NewChannelMessageService(service.ChannelMessageServiceConfig{Auth: &snapshotNoCallAuth{}, ContactRepo: &snapshotNoCallContact{}, WorkspaceRepo: workspaceReader, EndpointRepo: endpointRepo, SendRepo: sendRepo, TemplateService: &snapshotSMSTemplates{}, ProviderResolver: &snapshotSMSResolver{provider}, APIEndpoint: "https://notify.example.org"})
	require.NoError(t, err)
	channelSnapshot, err := domain.NewExecutionSnapshot("journey", "sms-execution", map[string]any{}, nil)
	require.NoError(t, err)
	require.NoError(t, snapshotRepo.Save(ctx, "ws", channelSnapshot))
	channelArchiver, err := service.NewExecutionSnapshotService(snapshotRepo, &snapshotNoCallAuth{})
	require.NoError(t, err)
	channel.SetExecutionSnapshotService(channelArchiver)
	channelCtx, err := domain.WithExecutionSnapshot(context.WithValue(ctx, domain.SystemCallKey, true), channelSnapshot)
	require.NoError(t, err)
	response, err := channel.Send(channelCtx, &domain.SendChannelMessageRequest{WorkspaceID: "ws", CustomerID: noEmailID, Channel: "sms", EffectKey: "no-email-effect", IntegrationID: "twilio", TemplateID: "sms-template"})
	require.NoError(t, err)
	require.Equal(t, domain.ChannelSendConfirmed, response.Execution.Status)
	require.Equal(t, "+15557654321", provider.address)
	matchedSnapshot, err := snapshotRepo.SnapshotForChannelSend(ctx, "ws", "twilio", response.Execution.MessageID, "no-email-effect", "SM-no-email")
	require.NoError(t, err)
	require.Equal(t, "sms-execution", matchedSnapshot.ExecutionID)
	_, err = snapshotRepo.SnapshotForChannelSend(ctx, "ws", "another-integration", response.Execution.MessageID, "no-email-effect", "SM-no-email")
	require.ErrorIs(t, err, domain.ErrExecutionSnapshotNotFound)
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM execution_evidence WHERE execution_id='sms-execution' AND evidence_key='no-email-effect:confirmed' AND status='confirmed'`).Scan(&count))
	require.Equal(t, 1, count)
	var storedCustomer string
	var storedEmail sql.NullString
	require.NoError(t, db.QueryRow(`SELECT customer_id::text,contact_email FROM message_history WHERE id=$1`, response.Execution.MessageID).Scan(&storedCustomer, &storedEmail))
	require.Equal(t, noEmailID, storedCustomer)
	require.False(t, storedEmail.Valid)
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM event_ledger WHERE customer_id=$1 AND event_type='sms.sent'`, noEmailID).Scan(&count))
	require.Equal(t, 1, count)
	_, err = db.Exec(`UPDATE customer_consents SET revoked_at=CURRENT_TIMESTAMP WHERE customer_id=$1`, noEmailID)
	require.NoError(t, err)
	_, err = channel.Send(context.WithValue(ctx, domain.SystemCallKey, true), &domain.SendChannelMessageRequest{WorkspaceID: "ws", CustomerID: noEmailID, Channel: "sms", EffectKey: "no-consent", IntegrationID: "twilio", TemplateID: "sms-template"})
	require.ErrorContains(t, err, "no active matching contact endpoint")
	secretAutomation := &domain.Automation{ID: "secret-automation", Nodes: []*domain.AutomationNode{{ID: "webhook-node", Type: domain.NodeTypeWebhook, Config: map[string]interface{}{"secret": "original-webhook-token", "url": "https://example.org/hook"}}}}
	nodesJSON, _ := json.Marshal(secretAutomation.Nodes)
	_, err = db.Exec(`INSERT INTO automations(id,workspace_id,name,status,trigger_config,root_node_id,nodes,version) VALUES('secret-automation','ws','Secret workflow','live','{"event_kind":"contact.updated","frequency":"once"}','webhook-node',$1,1)`, nodesJSON)
	require.NoError(t, err)
	err = db.QueryRow(`SELECT outcome,contact_automation_id,retry_at FROM automation_enroll_customer('secret-automation',$1,'snapshot@example.org','webhook-node','once',NULL,'{"enabled":false}',1,'realtime')`, customerID).Scan(&outcome, &contactAutomationID, &retry)
	require.ErrorContains(t, err, "Runtime credentials must be archived")
	archiver, err := service.NewExecutionSnapshotService(snapshotRepo, &snapshotNoCallAuth{})
	require.NoError(t, err)
	archiver.SetWorkspaceRepository(workspaceReader)
	archiver.SetTemplateRepository(&snapshotNoCallTemplates{})
	require.NoError(t, archiver.PrepareAutomation(ctx, "ws", secretAutomation))
	require.NoError(t, db.QueryRow(`SELECT outcome,contact_automation_id,retry_at FROM automation_enroll_customer('secret-automation',$1,'snapshot@example.org','webhook-node','once',NULL,'{"enabled":false}',1,'realtime')`, customerID).Scan(&outcome, &contactAutomationID, &retry))
	secretSnapshot, err := snapshotRepo.Get(ctx, "ws", contactAutomationID)
	require.NoError(t, err)
	require.NotContains(t, string(secretSnapshot.Configuration), "original-webhook-token")
	var secretDeps map[string]interface{}
	require.NoError(t, json.Unmarshal(secretSnapshot.Dependencies, &secretDeps))
	ciphertext, err := snapshotRepo.RuntimeNodes(ctx, "ws", secretDeps["runtime_nodes_ref"].(string))
	require.NoError(t, err)
	plaintext, err := pkgcrypto.DecryptFromHexString(ciphertext, "snapshot-secret")
	require.NoError(t, err)
	require.Contains(t, plaintext, "original-webhook-token")
	providerAutomation := &domain.Automation{ID: "provider-refresh", Nodes: []*domain.AutomationNode{{ID: "send-sms", Type: domain.NodeTypeSMS, Config: map[string]interface{}{"integration_id": "sms-provider"}}}}
	providerNodes, _ := json.Marshal(providerAutomation.Nodes)
	_, err = db.Exec(`INSERT INTO automations(id,workspace_id,name,status,trigger_config,root_node_id,nodes,version) VALUES('provider-refresh','ws','Provider workflow','live','{"event_kind":"contact.updated","frequency":"every_time"}','send-sms',$1,1)`, providerNodes)
	require.NoError(t, err)
	workspaceReader.current = &domain.Workspace{ID: "ws", Settings: domain.WorkspaceSettings{SecretKey: "snapshot-secret"}, Integrations: domain.Integrations{{ID: "sms-provider", Type: domain.IntegrationTypeSMS, SMSProvider: &domain.SMSProvider{Twilio: &domain.TwilioSettings{AuthToken: "old-provider-token"}}}}}
	require.NoError(t, archiver.PrepareAutomation(ctx, "ws", providerAutomation))
	enrollProvider := func() *domain.ExecutionSnapshot {
		t.Helper()
		require.NoError(t, db.QueryRow(`SELECT outcome,contact_automation_id,retry_at FROM automation_enroll_customer('provider-refresh',$1,'snapshot@example.org','send-sms','every_time',$2,'{"enabled":false}',1,'realtime')`, customerID, uuid.NewString()).Scan(&outcome, &contactAutomationID, &retry))
		require.Equal(t, "enrolled", outcome)
		snap, err := snapshotRepo.Get(ctx, "ws", contactAutomationID)
		require.NoError(t, err)
		return snap
	}
	providerBefore := enrollProvider()
	require.NoError(t, archiver.UpdateWorkspaceProviders(ctx, "ws", func() error {
		workspaceReader.current.Integrations[0].SMSProvider.Twilio.AuthToken = "new-provider-token"
		return nil
	}))
	providerAfter := enrollProvider()
	for _, test := range []struct {
		snapshot *domain.ExecutionSnapshot
		token    string
	}{{providerBefore, "old-provider-token"}, {providerAfter, "new-provider-token"}} {
		var deps map[string]any
		require.NoError(t, json.Unmarshal(test.snapshot.Dependencies, &deps))
		cipher, err := snapshotRepo.RuntimeNodes(ctx, "ws", deps["runtime_nodes_ref"].(string))
		require.NoError(t, err)
		plain, err := pkgcrypto.DecryptFromHexString(cipher, "snapshot-secret")
		require.NoError(t, err)
		require.Contains(t, plain, test.token)
	}
	// Failed preparation blocks future enrollment and the durable backfill can repair it.
	require.NoError(t, snapshotRepo.SetProviderRefreshPending(ctx, "ws", true))
	err = db.QueryRow(`SELECT outcome,contact_automation_id,retry_at FROM automation_enroll_customer('provider-refresh',$1,'snapshot@example.org','send-sms','every_time',$2,'{"enabled":false}',1,'realtime')`, customerID, uuid.NewString()).Scan(&outcome, &contactAutomationID, &retry)
	require.ErrorContains(t, err, "awaiting preparation")
	require.NoError(t, archiver.RetryProviderRefresh(ctx, "ws"))
	enrollProvider()
	mediaCtx, err := domain.WithExecutionSnapshot(ctx, journey)
	require.NoError(t, err)
	mediaCtx = domain.WithExecutionMediaArchive(mediaCtx, archiver.ArchiveRenderedMedia)
	queue := repository.NewEmailQueueRepositoryWithDB(db)
	require.NoError(t, queue.Enqueue(mediaCtx, "ws", []*domain.EmailQueueEntry{{ID: "rendered-media-queue", SourceType: domain.EmailQueueSourceAutomation, SourceID: "snapshot-automation", IntegrationID: "provider", ProviderKind: domain.EmailProviderKindSMTP, ContactEmail: "snapshot@example.org", MessageID: "rendered-media-message", TemplateID: "snapshot-template", Payload: domain.EmailQueuePayload{ExecutionID: journey.ExecutionID, HTMLContent: `<img src="https://cdn.example.org/original.png">`}}}))
	mediaEvidence, err := snapshotRepo.DeliveryMediaReferences(ctx, "ws", journey.ExecutionID, "rendered-media-message")
	require.NoError(t, err)
	require.Contains(t, string(mediaEvidence), hashString)
	var mediaCustomer string
	require.NoError(t, db.QueryRow(`SELECT customer_id::text FROM execution_evidence WHERE execution_id=$1 AND evidence_key='media:rendered-media-message'`, journey.ExecutionID).Scan(&mediaCustomer))
	require.Equal(t, customerID, mediaCustomer)
	foundMedia, err := snapshotRepo.ExecutionReferencesAsset(ctx, "ws", journey.ExecutionID, hashString)
	require.NoError(t, err)
	require.True(t, foundMedia)
	err = queue.Enqueue(mediaCtx, "ws", []*domain.EmailQueueEntry{{ID: "unsupported-media-queue", MessageID: "unsupported-media-message", Payload: domain.EmailQueuePayload{ExecutionID: journey.ExecutionID, HTMLContent: `<img src="/relative.png">`}}})
	require.ErrorContains(t, err, "absolute")
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM email_queue WHERE id='unsupported-media-queue'`).Scan(&count))
	require.Zero(t, count)
	// Campaign provider references are immutable per run too, without publishing a version.
	workspaceReader.current.Settings.MarketingEmailProviderID = "campaign-smtp"
	workspaceReader.current.Integrations = append(workspaceReader.current.Integrations, domain.Integration{ID: "campaign-smtp", Type: domain.IntegrationTypeEmail, EmailProvider: domain.EmailProvider{Kind: domain.EmailProviderKindSMTP, SMTP: &domain.SMTPSettings{Password: "campaign-old-password"}}})
	archiver.SetTemplateRepository(&snapshotCampaignTemplates{})
	campaignBefore, err := archiver.Build(ctx, "ws", "campaign", uuid.NewString(), "campaign-source", map[string]any{"name": "Current campaign"}, []service.ExecutionTemplateReference{{ID: "email"}})
	require.NoError(t, err)
	require.NoError(t, snapshotRepo.Save(ctx, "ws", campaignBefore))
	workspaceReader.current.Integrations[len(workspaceReader.current.Integrations)-1].EmailProvider.SMTP.Password = "campaign-new-password"
	campaignAfter, err := archiver.Build(ctx, "ws", "campaign", uuid.NewString(), "campaign-source", map[string]any{"name": "Current campaign"}, []service.ExecutionTemplateReference{{ID: "email"}})
	require.NoError(t, err)
	require.NoError(t, snapshotRepo.Save(ctx, "ws", campaignAfter))
	for _, test := range []struct {
		snapshot *domain.ExecutionSnapshot
		password string
	}{{campaignBefore, "campaign-old-password"}, {campaignAfter, "campaign-new-password"}} {
		provider, err := archiver.ResolveExecutionIntegration(ctx, "ws", test.snapshot.ExecutionID, "campaign-smtp", workspaceReader.current)
		require.NoError(t, err)
		require.Equal(t, test.password, provider.EmailProvider.SMTP.Password)
		require.NotContains(t, string(test.snapshot.Dependencies), test.password)
	}
	for i := 0; i < 51; i++ {
		entry, createErr := domain.NewExecutionSnapshot("decision", uuid.NewString(), map[string]any{"index": i}, nil)
		require.NoError(t, createErr)
		entry.SourceID = "history-pagination"
		require.NoError(t, snapshotRepo.Save(ctx, "ws", entry))
	}
	historyPage, err := snapshotRepo.HistoryPage(ctx, "ws", "history-pagination", "")
	require.NoError(t, err)
	require.Len(t, historyPage.Items, 50)
	require.NotEmpty(t, historyPage.Next)
	older, err := snapshotRepo.HistoryPage(ctx, "ws", "history-pagination", historyPage.Next)
	require.NoError(t, err)
	require.Len(t, older.Items, 1)
	require.Empty(t, older.Next)
}

type snapshotChannelWorkspace struct {
	domain.WorkspaceRepository
	db      *sql.DB
	current *domain.Workspace
}

func (s *snapshotChannelWorkspace) GetConnection(context.Context, string) (*sql.DB, error) {
	return s.db, nil
}
func (s *snapshotChannelWorkspace) GetByID(context.Context, string) (*domain.Workspace, error) {
	if s.current != nil {
		return s.current, nil
	}
	return &domain.Workspace{ID: "ws", Settings: domain.WorkspaceSettings{SecretKey: "snapshot-secret", DefaultLanguage: "en"}}, nil
}

type snapshotNoCallAuth struct{ domain.AuthService }
type snapshotCampaignTemplates struct{ domain.TemplateRepository }

func (*snapshotCampaignTemplates) GetTemplateByID(context.Context, string, string, int64) (*domain.Template, error) {
	return &domain.Template{ID: "email", Version: 1, Channel: "email", Email: &domain.EmailTemplate{Subject: "Saved subject"}}, nil
}

type snapshotNoCallContact struct{ domain.ContactRepository }
type snapshotSMSTemplates struct{ domain.TemplateService }

func (*snapshotSMSTemplates) GetTemplateByID(context.Context, string, string, int64) (*domain.Template, error) {
	return &domain.Template{ID: "sms-template", Version: 1, Channel: "sms", SMS: &domain.SMSTemplate{Body: "Hello customer"}}, nil
}
func (*snapshotSMSTemplates) PreviewTemplate(context.Context, domain.PreviewTemplateRequest) (*domain.PreviewTemplateResponse, error) {
	return &domain.PreviewTemplateResponse{ResolvedLanguage: "en", SMS: &domain.SMSPreview{Body: "Hello customer"}}, nil
}

type snapshotSMSProvider struct {
	domain.ChannelProvider
	address string
}

func (s *snapshotSMSProvider) Send(_ context.Context, r domain.ChannelDeliveryRequest) (*domain.ChannelDeliveryResult, error) {
	s.address = r.Recipient
	return &domain.ChannelDeliveryResult{Provider: "twilio", ProviderMessageID: "SM-no-email", Status: "queued"}, nil
}

type snapshotSMSResolver struct{ p *snapshotSMSProvider }

func (s *snapshotSMSResolver) Resolve(*domain.Workspace, string, string) (domain.ChannelProvider, error) {
	return s.p, nil
}

type snapshotTrackingDefinitions struct {
	domain.BusinessDefinitionRepository
}

func (*snapshotTrackingDefinitions) Get(_ context.Context, _, kind, key string) (*domain.BusinessDefinition, error) {
	return &domain.BusinessDefinition{Kind: kind, Key: key, Status: "active", Settings: map[string]any{"subject_type": "loan"}}, nil
}

type snapshotNoCallTemplates struct{ domain.TemplateRepository }
