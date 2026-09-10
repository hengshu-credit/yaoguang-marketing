//go:build integration

package repository

import (
	"context"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type configurationOwner struct{ domain.AuthService }
type configurationWorkspace struct{ domain.WorkspaceRepository }

func (configurationWorkspace) GetByID(context.Context, string) (*domain.Workspace, error) {
	return &domain.Workspace{ID: "ws"}, nil
}

func (configurationOwner) AuthenticateUserForWorkspace(ctx context.Context, _ string) (context.Context, *domain.User, *domain.UserWorkspace, error) {
	return ctx, &domain.User{}, &domain.UserWorkspace{Role: "owner"}, nil
}

func TestConfigurationEventFieldsAndStateInspectionIntegration(t *testing.T) {
	db := trackedTestDB(t)
	ctx := context.Background()
	definitions := NewBusinessConfigRepositoryWithDB(db)
	for _, ddl := range append(schema.SubjectAttributeHistoryTableDefinitions(), schema.FunnelTableDefinitions()...) {
		_, err := db.Exec(ddl)
		require.NoError(t, err)
	}
	customer := uuid.NewString()
	_, err := db.Exec(`INSERT INTO customers(id,customer_no,external_user_id) VALUES($1,'C1','configuration-user')`, customer)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO customer_profiles(customer_id,attributes) VALUES($1,'{"score":700,"device.fingerprint":"fp"}')`, customer)
	require.NoError(t, err)
	_, err = definitions.Save(ctx, "ws", &domain.BusinessDefinition{Kind: "event", Key: "checkout.completed", Name: "Checkout completed", Status: "active", Schema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}, Settings: map[string]any{"subject_type": "customer", "expiry_mode": "never", "unmatched_customer": "reject"}})
	require.NoError(t, err)
	extension := map[string]any{"type": "object", "properties": map[string]any{"device_fingerprint": map[string]any{"type": "string"}}}
	config := service.NewBusinessConfigService(definitions, configurationOwner{})
	updated, err := config.ExtendEvents(ctx, "ws", []string{"sms.sent", "checkout.completed"}, extension)
	require.NoError(t, err)
	require.Len(t, updated, 2)
	for _, ddl := range schema.BusinessConfigTableDefinitions() {
		_, err = db.Exec(ddl)
		require.NoError(t, err)
	}
	native, err := definitions.Get(ctx, "ws", "event", "sms.sent")
	require.NoError(t, err)
	require.Contains(t, native.Schema["properties"], "device_fingerprint")
	require.Equal(t, true, native.Settings["builtin"])
	require.Equal(t, 2, native.Revision)
	conflict := map[string]any{"type": "object", "properties": map[string]any{"device_fingerprint": map[string]any{"type": "number"}}}
	_, err = config.ExtendEvents(ctx, "ws", []string{"contact.created", "checkout.completed"}, conflict)
	require.Error(t, err)
	unchanged, err := definitions.Get(ctx, "ws", "event", "contact.created")
	require.NoError(t, err)
	require.NotContains(t, unchanged.Schema, "properties")
	events := service.NewTrackedEventService(NewTrackedEventRepositoryWithDB(db, "0123456789abcdef0123456789abcdef"), definitions, configurationOwner{}, configurationWorkspace{})
	report := domain.TrackEventRequest{WorkspaceID: "ws", Source: "app", Event: map[string]any{"report_id": "config-event-1", "event_code": "checkout.completed", "customer_attributes": map[string]any{"external_user_id": "configuration-user"}, "data": map[string]any{"device_fingerprint": "fp"}, "triggered_at": time.Now().UTC().Format(time.RFC3339Nano)}}
	_, err = events.Track(ctx, report)
	require.NoError(t, err)
	report.Event["report_id"] = "bad-event"
	report.Event["data"] = map[string]any{"device_fingerprint": 12}
	_, err = events.Track(ctx, report)
	require.Error(t, err)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM event_ledger`).Scan(&count))
	before := count
	_, err = config.Save(ctx, "ws", &domain.BusinessDefinition{Kind: "state", Key: "eligible", Name: "Eligible", Status: "active", Schema: map[string]any{"type": "object"}, Settings: map[string]any{"subject_type": "customer", "rule": map[string]any{"kind": "all", "children": []any{map[string]any{"kind": "event", "event": "checkout.completed"}, map[string]any{"kind": "attribute", "field": "score", "operator": "gte", "operand": 600}}}}})
	require.NoError(t, err)
	states := service.NewSubjectStateService(NewSubjectStateRepositoryWithDB(db), NewFunnelRepositoryWithDB(db), definitions, configurationOwner{})
	inspect := service.SubjectStateInspectRequest{WorkspaceID: "ws", SubjectType: "customer", SubjectID: customer}
	results, err := states.Inspect(ctx, inspect)
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Equal(t, domain.StateTrue, results[0].Value)
	_, err = db.Exec(`UPDATE customer_profiles SET attributes='{"score":100}' WHERE customer_id=$1`, customer)
	require.NoError(t, err)
	results, err = states.Inspect(ctx, inspect)
	require.NoError(t, err)
	require.NotEmpty(t, results)
	require.Equal(t, domain.StateFalse, results[0].Value)
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM event_ledger`).Scan(&count))
	require.Equal(t, before, count, "inspection must not emit marketing-triggering state events")
}
