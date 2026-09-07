package database

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/migrations"
	"github.com/hengshu-credit/yaoguang-marketing/pkg/postgresdriver"
	"github.com/stretchr/testify/require"
	"net/url"
	"os"
	"testing"
)

func TestConfigurableWorkspaceUpgradeMatchesFreshSchema(t *testing.T) {
	dsn := os.Getenv("SNAPSHOT_POSTGRES_ADMIN_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	admin, err := sql.Open(postgresdriver.Name, dsn)
	require.NoError(t, err)
	defer admin.Close()
	create := func() *sql.DB {
		name := "configurable_upgrade_" + uuid.NewString()[:8]
		_, err := admin.Exec(`CREATE DATABASE "` + name + `"`)
		require.NoError(t, err)
		parsed, err := url.Parse(dsn)
		require.NoError(t, err)
		parsed.Path = "/" + name
		db, err := sql.Open(postgresdriver.Name, parsed.String())
		require.NoError(t, err)
		t.Cleanup(func() {
			db.Close()
			cleanup, err := sql.Open(postgresdriver.Name, dsn)
			if err == nil {
				defer cleanup.Close()
				_, err = cleanup.Exec(`DROP DATABASE "` + name + `" WITH (FORCE)`)
				require.NoError(t, err)
			}
		})
		return db
	}
	fresh, upgraded := create(), create()
	require.NoError(t, InitializeWorkspaceDatabase(fresh))
	require.NoError(t, initializeWorkspaceDatabaseBase(upgraded))
	customer := uuid.NewString()
	_, err = upgraded.Exec(`INSERT INTO customers(id,customer_no,external_user_id) VALUES($1,'legacy-customer','external-before-upgrade')`, customer)
	require.NoError(t, err)
	workspace := &domain.Workspace{}
	workspace.Settings.CustomFieldLabels = map[string]string{"custom_string_1": "Existing business label"}
	migrate := &migrations.V57Migration{}
	require.NoError(t, migrate.UpdateWorkspace(context.Background(), nil, workspace, upgraded))
	require.NoError(t, migrate.UpdateWorkspace(context.Background(), nil, workspace, upgraded))
	queries := []string{
		`SELECT json_agg(json_build_array(table_name,column_name,data_type,is_nullable,column_default) ORDER BY table_name,ordinal_position)::text FROM information_schema.columns WHERE table_schema='public'`,
		`SELECT json_agg(json_build_array(c.relname,co.conname,pg_get_constraintdef(co.oid)) ORDER BY c.relname,co.conname)::text FROM pg_constraint co JOIN pg_class c ON c.oid=co.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'`,
		`SELECT json_agg(json_build_array(c.relname,t.tgname,pg_get_triggerdef(t.oid)) ORDER BY c.relname,t.tgname)::text FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE NOT t.tgisinternal AND n.nspname='public'`,
		`SELECT json_agg(json_build_array(p.proname,pg_get_functiondef(p.oid)) ORDER BY p.proname,p.oid::regprocedure::text)::text FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.prokind='f'`,
	}
	for _, query := range queries {
		var a, b string
		require.NoError(t, fresh.QueryRow(query).Scan(&a))
		require.NoError(t, upgraded.QueryRow(query).Scan(&b))
		require.Equal(t, a, b, query)
	}
	var name string
	require.NoError(t, upgraded.QueryRow(`SELECT name FROM business_definitions WHERE kind='customer_attribute' AND key='custom_string_1'`).Scan(&name))
	require.Equal(t, "Existing business label", name)
	require.NoError(t, upgraded.QueryRow(`SELECT external_user_id FROM customers WHERE id=$1`, customer).Scan(&name))
	require.Equal(t, "external-before-upgrade", name)
}
