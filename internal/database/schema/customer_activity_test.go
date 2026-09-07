package schema

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// This test creates all objects inside a new schema in a transaction and rolls
// it back. It cannot mutate existing workspace data or dispatch real messages.
func TestCustomerActivityPostgres(t *testing.T) {
	if os.Getenv("CUSTOMER_ACTIVITY_TEST") != "1" {
		t.Skip("set CUSTOMER_ACTIVITY_TEST=1 with a local PostgreSQL connection")
	}
	host := os.Getenv("CUSTOMER_ACTIVITY_TEST_HOST")
	if host == "" {
		host = "postgres:5432"
	}
	u := &url.URL{Scheme: "postgres", Host: host, Path: "/postgres", User: url.UserPassword(os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD")), RawQuery: "sslmode=disable"}
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	defer db.Close()
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	schemaName := "test_activity_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	exec := func(query string) { t.Helper(); _, err := tx.Exec(query); require.NoError(t, err) }
	exec(`CREATE SCHEMA "` + schemaName + `"`)
	exec(`SET LOCAL search_path = "` + schemaName + `", public`)
	exec(`CREATE TABLE customers (id UUID PRIMARY KEY);
	CREATE TABLE contacts (email TEXT PRIMARY KEY, customer_id UUID, first_name TEXT, future_field JSONB, created_at TIMESTAMPTZ DEFAULT NOW(), updated_at TIMESTAMPTZ DEFAULT NOW());
	CREATE TABLE lists (id TEXT PRIMARY KEY, name TEXT);
	CREATE TABLE contact_lists (email TEXT, customer_id UUID, list_id TEXT, status TEXT, deleted_at TIMESTAMPTZ, created_at TIMESTAMPTZ DEFAULT NOW(), updated_at TIMESTAMPTZ DEFAULT NOW(), PRIMARY KEY(email,list_id));
	CREATE TABLE customer_profiles (customer_id UUID PRIMARY KEY, status TEXT, language TEXT, timezone TEXT, attributes JSONB DEFAULT '{}', version BIGINT DEFAULT 1, created_at TIMESTAMPTZ DEFAULT NOW(), updated_at TIMESTAMPTZ DEFAULT NOW());
	CREATE TABLE customer_tags (customer_id UUID, tag TEXT, created_at TIMESTAMPTZ DEFAULT NOW(), PRIMARY KEY(customer_id,tag));
	CREATE TABLE contact_timeline (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), origin_event_id UUID DEFAULT gen_random_uuid(), email TEXT NOT NULL, customer_id UUID, operation TEXT, entity_type TEXT, kind TEXT, entity_id TEXT, changes JSONB, created_at TIMESTAMPTZ, db_created_at TIMESTAMPTZ DEFAULT NOW());
	CREATE TABLE event_idempotency(id UUID PRIMARY KEY, received_at TIMESTAMPTZ, payload_hash TEXT);
	CREATE TABLE event_ledger(id UUID, event_type TEXT, subject_type TEXT, subject_id TEXT, customer_id UUID, contact_email TEXT, source TEXT, schema_version INT, occurred_at TIMESTAMPTZ, received_at TIMESTAMPTZ, properties JSONB, context JSONB, timeline_id UUID);
	CREATE TABLE event_outbox(id UUID, event_id UUID, topic TEXT, routing_key TEXT, payload JSONB, headers JSONB, UNIQUE(event_id,topic,routing_key));`)
	for _, statement := range CustomerActivityTimelineDefinitions() {
		exec(statement)
	}
	// Rerunning a migration must be safe, with a single trigger per source.
	for _, statement := range CustomerActivityTimelineDefinitions() {
		exec(statement)
	}
	exec(`CREATE TRIGGER contact_changes_trigger AFTER INSERT OR UPDATE ON contacts FOR EACH ROW EXECUTE FUNCTION track_contact_changes()`)
	exec(TimelineEventBridgeFunction())
	exec(`CREATE TRIGGER bridge AFTER INSERT ON contact_timeline FOR EACH ROW EXECUTE FUNCTION notifuse_capture_timeline_event()`)
	const cid = "11111111-1111-4111-8111-111111111111"
	exec(`INSERT INTO customers VALUES ('` + cid + `'); INSERT INTO contacts(email,customer_id) VALUES ('alice@example.com','` + cid + `'); INSERT INTO lists VALUES ('vip','VIP customers');`)
	count := func(where string) int {
		t.Helper()
		var n int
		require.NoError(t, tx.QueryRow("SELECT COUNT(*) FROM contact_timeline WHERE "+where).Scan(&n))
		return n
	}
	assertSQL := func(query string) {
		t.Helper()
		var ok bool
		require.NoError(t, tx.QueryRow(query).Scan(&ok))
		require.True(t, ok, query)
	}
	exec(`INSERT INTO contact_lists(email,customer_id,list_id,status) VALUES ('alice@example.com','` + cid + `','vip','active')`)
	exec(`UPDATE contact_lists SET deleted_at=NOW() WHERE list_id='vip'`)
	require.Equal(t, 1, count("kind='list.removed' AND operation='delete'"))
	exec(`UPDATE contact_lists SET deleted_at=NOW()+INTERVAL '1 second' WHERE list_id='vip'`)
	require.Equal(t, 1, count("kind='list.removed'"), "repeated soft removal is not a new removal")
	exec(`UPDATE contact_lists SET deleted_at=NULL WHERE list_id='vip'`)
	require.Equal(t, 1, count("kind='list.resubscribed'"), "same-status restoration must be recorded")
	exec(`DELETE FROM contact_lists WHERE list_id='vip'`)
	require.Equal(t, 2, count("kind='list.removed'"))
	assertSQL(`SELECT bool_and(changes->'list_name'->>'new'='VIP customers') FROM contact_timeline WHERE kind='list.removed'`)
	exec(`INSERT INTO customer_profiles(customer_id,status,attributes) VALUES ('` + cid + `','active','{"score":88,"retire":true}')`)
	exec(`UPDATE customer_profiles SET attributes='{"score":96,"nested":{"level":2},"zero":0,"flag":false}', version=2 WHERE customer_id='` + cid + `'`)
	assertSQL(`SELECT changes->'attributes'->'score' = '{"old":88,"new":96}'::jsonb AND changes->'attributes'->'retire' = '{"old":true,"new":null}'::jsonb AND changes->'attributes'->'flag'->'new'='false'::jsonb FROM contact_timeline WHERE kind='contact.profile_updated'`)
	before := count("TRUE")
	exec(`UPDATE customer_profiles SET version=3,updated_at=NOW() WHERE customer_id='` + cid + `'`)
	require.Equal(t, before, count("TRUE"), "version-only writes must not produce false attribute edits")
	exec(`INSERT INTO customer_tags(customer_id,tag) VALUES ('` + cid + `','VIP'); DELETE FROM customer_tags WHERE customer_id='` + cid + `'`)
	require.Equal(t, 1, count("kind='contact.tagged'"))
	require.Equal(t, 1, count("kind='contact.untagged'"))
	exec(`UPDATE contacts SET future_field='{"items":[1,2]}', first_name='Alice' WHERE email='alice@example.com'`)
	assertSQL(`SELECT changes->'future_field'->'new'='{"items":[1,2]}'::jsonb AND changes->'first_name'->>'new'='Alice' FROM contact_timeline WHERE kind='contact.updated'`)
	// A customer without an email projection still records its authoritative ID.
	exec(`INSERT INTO customers VALUES ('22222222-2222-4222-8222-222222222222'); INSERT INTO customer_profiles(customer_id,attributes) VALUES ('22222222-2222-4222-8222-222222222222','{"score":12}')`)
	require.Equal(t, 1, count("email='' AND customer_id='22222222-2222-4222-8222-222222222222'"))
	assertSQL(`SELECT (SELECT COUNT(*) FROM contact_timeline)=(SELECT COUNT(*) FROM event_ledger) AND (SELECT COUNT(*) FROM contact_timeline)=(SELECT COUNT(*) FROM event_outbox)`)
	before = count("TRUE")
	exec(`SAVEPOINT rollback_edit; UPDATE customer_profiles SET status='changed' WHERE customer_id='` + cid + `'; ROLLBACK TO SAVEPOINT rollback_edit`)
	require.Equal(t, before, count("TRUE"))
	assertSQL(fmt.Sprintf(`SELECT COUNT(*)=%d FROM event_outbox`, before))
}
