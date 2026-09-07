//go:build integration

package repository

import (
	"context"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/database/schema"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestNativeBusinessEventCataloguePersistence(t *testing.T) {
	db := trackedTestDB(t)
	repo := NewBusinessConfigRepositoryWithDB(db)
	for _, channel := range []string{"sms", "push", "whatsapp", "webhook"} {
		for _, suffix := range []string{"sent", "delivered", "opened", "clicked", "failed", "bounced", "complained", "unsubscribed", "updated"} {
			d, err := repo.Get(context.Background(), "w", "event", channel+"."+suffix)
			require.NoError(t, err)
			require.Equal(t, "active", d.Status)
			require.Equal(t, true, d.Settings["builtin"])
			require.Equal(t, false, d.Settings["browser_enabled"])
			require.Equal(t, "customer", d.Settings["subject_type"])
		}
	}
	listed, total, err := repo.List(context.Background(), "w", domain.BusinessDefinitionFilter{Kind: "event", Status: "active", Search: "sms.", Limit: 100})
	require.NoError(t, err)
	require.Equal(t, 9, total)
	require.Len(t, listed, 9)
	// Reapplying initialization must not invent new revisions for unchanged defaults.
	for _, ddl := range schema.BusinessConfigTableDefinitions() {
		_, err = db.Exec(ddl)
		require.NoError(t, err)
	}
	var revision int
	require.NoError(t, db.QueryRow(`SELECT max(revision) FROM business_definitions WHERE kind='event' AND key LIKE 'sms.%'`).Scan(&revision))
	require.Equal(t, 1, revision)
	// An upgrade protects a previously custom-claimed system code while retaining
	// its old revision for historical configuration readers.
	_, err = db.Exec(`DELETE FROM business_definition_revisions WHERE definition_id IN(SELECT id FROM business_definitions WHERE kind='event' AND key='sms.sent')`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE business_definitions SET name='Old custom claim',settings='{"subject_type":"customer","browser_enabled":true}' WHERE kind='event' AND key='sms.sent'`)
	require.NoError(t, err)
	for _, ddl := range schema.BusinessConfigTableDefinitions() {
		_, err = db.Exec(ddl)
		require.NoError(t, err)
	}
	var oldName string
	require.NoError(t, db.QueryRow(`SELECT r.document->>'name' FROM business_definition_revisions r JOIN business_definitions d ON d.id=r.definition_id WHERE d.key='sms.sent' AND r.revision=1`).Scan(&oldName))
	require.Equal(t, "Old custom claim", oldName)
	restored, err := repo.Get(context.Background(), "w", "event", "sms.sent")
	require.NoError(t, err)
	require.Equal(t, 2, restored.Revision)
	require.Equal(t, true, restored.Settings["builtin"])
	require.Equal(t, false, restored.Settings["browser_enabled"])
}

func TestCustomerErasureRemovesOnlyUnreferencedRenderedAssets(t *testing.T) {
	db := trackedTestDB(t)
	// Use the production table definitions without unrelated campaign initialization.
	for _, ddl := range schema.ExecutionSnapshotTableDefinitions() {
		for _, table := range []string{"execution_config_snapshots", "execution_evidence", "execution_archived_assets", "execution_template_material_archives"} {
			if strings.HasPrefix(ddl, "CREATE TABLE IF NOT EXISTS "+table) {
				_, err := db.Exec(ddl)
				require.NoError(t, err)
			}
		}
	}
	customer, other := uuid.NewString(), uuid.NewString()
	_, err := db.Exec(`INSERT INTO customers(id,customer_no) VALUES($1,'erase'),($2,'keep')`, customer, other)
	require.NoError(t, err)
	hashes := []string{strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64), strings.Repeat("d", 64), strings.Repeat("e", 64)}
	for _, hash := range hashes {
		_, err = db.Exec(`INSERT INTO execution_archived_assets(hash,media_type,data) VALUES($1,'image/png','personal bytes')`, hash)
		require.NoError(t, err)
	}
	_, err = db.Exec(`INSERT INTO execution_config_snapshots(id,execution_id,kind,configuration,materials,content_hash,completeness) VALUES($1,'run','campaign','{}',jsonb_build_object('assets',jsonb_build_array(jsonb_build_object('hash',$2::text))),'hash','complete')`, uuid.NewString(), hashes[2])
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO execution_template_material_archives(template_id,content_hash,content,assets) VALUES('future','hash','{}',jsonb_build_array(jsonb_build_object('hash',$1::text)))`, hashes[3])
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO execution_evidence(id,execution_id,evidence_key,kind,customer_id,status,payload,payload_hash) VALUES($1,'run','erase','delivery_media',$2,'ready',jsonb_build_object('assets',jsonb_build_array(jsonb_build_object('hash',$3::text),jsonb_build_object('hash',$4::text),jsonb_build_object('hash',$5::text),jsonb_build_object('hash',$6::text))),'hash'),($7,'run','keep','delivery_media',$8,'ready',jsonb_build_object('assets',jsonb_build_array(jsonb_build_object('hash',$4::text))),'hash')`, uuid.NewString(), customer, hashes[0], hashes[1], hashes[2], hashes[3], uuid.NewString(), other)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM customers WHERE id=$1`, customer)
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM execution_archived_assets WHERE hash=$1`, hashes[0]).Scan(&count))
	require.Zero(t, count, "erased personal media bytes retained")
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM execution_archived_assets`).Scan(&count))
	require.Equal(t, 4, count, "shared/static/future or unrelated in-flight assets removed")
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM execution_evidence`).Scan(&count))
	require.Equal(t, 1, count)
}
