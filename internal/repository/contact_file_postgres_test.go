package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContactFilePostgresIntegration(t *testing.T) {
	if os.Getenv("CONTACT_FILE_TEST_DATABASE") != "1" {
		t.Skip("set CONTACT_FILE_TEST_DATABASE=1 to exercise local PostgreSQL temporary tables")
	}
	user, password := os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD")
	if user == "" {
		user = "postgres"
	}
	if password == "" {
		password = "postgres"
	}
	db, err := sql.Open("postgres", fmt.Sprintf("host=postgres port=5432 user=%s password=%s dbname=postgres sslmode=disable", user, password))
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TEMP TABLE contacts(email text, customer_id text);
	 CREATE TEMP TABLE customer_profiles(customer_id text, status text, attributes jsonb);
	 CREATE TEMP TABLE contact_profiles(email text, status text, attributes jsonb);
	 CREATE TEMP TABLE customer_tags(customer_id text, tag text);
	 CREATE TEMP TABLE contact_tags(email text, tag text);
	 CREATE TEMP TABLE business_definitions(kind text, key text, name text, schema jsonb, settings jsonb, status text);
	 INSERT INTO contacts VALUES ('file-test@example.com','customer-1');
	 INSERT INTO customer_profiles VALUES ('customer-1','active','{"score":0,"vip":false,"direct":"00123"}');
	 INSERT INTO contact_profiles VALUES ('file-test@example.com','old','{"score":99,"legacy":"keep"}');
	 INSERT INTO customer_tags VALUES ('customer-1','member');
	 INSERT INTO business_definitions VALUES ('customer_attribute','unused','Not yet populated','{"type":"integer"}','{}','active');`)
	require.NoError(t, err)
	workspaces := mocks.NewMockWorkspaceRepository(gomock.NewController(t))
	workspace := &domain.Workspace{Settings: domain.WorkspaceSettings{CustomFieldLabels: map[string]string{"custom_string_1": "Before"}}}
	workspaces.EXPECT().GetByID(gomock.Any(), "ws").Return(workspace, nil).AnyTimes()
	workspaces.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil).AnyTimes()
	repo := NewContactFileRepository(workspaces)
	first, err := repo.Fields(context.Background(), "ws")
	require.NoError(t, err)
	byKey := map[string]domain.ContactFileField{}
	for _, field := range first {
		byKey[field.Key] = field
	}
	assert.Equal(t, "integer", byKey["profile.attributes.unused"].Type)
	assert.Equal(t, "string", byKey["profile.attributes.direct"].Type)
	assert.Equal(t, "boolean", byKey["profile.attributes.vip"].Type)
	_, err = db.Exec(`INSERT INTO business_definitions VALUES ('customer_attribute','new','New field','{"type":"number"}','{}','draft')`)
	require.NoError(t, err)
	workspace.Settings.CustomFieldLabels["custom_string_1"] = "After"
	next, err := repo.Fields(context.Background(), "ws")
	require.NoError(t, err)
	assert.Len(t, next, len(first)+1)
	for _, field := range next {
		if field.Key == "custom_string_1" {
			assert.Equal(t, "After (custom_string_1)", field.Header)
		}
	}
	contacts := []*domain.Contact{{Email: "file-test@example.com"}}
	require.NoError(t, repo.LoadProfiles(context.Background(), "ws", contacts))
	assert.Equal(t, float64(0), contacts[0].Profile.Attributes["score"])
	assert.Equal(t, "keep", contacts[0].Profile.Attributes["legacy"])
	assert.Equal(t, []string{"member"}, contacts[0].Profile.Tags)
}

func TestContactFileRepositoryIncludesDefinitionsAndObservedKeys(t *testing.T) {
	db, sql, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	workspaces := mocks.NewMockWorkspaceRepository(gomock.NewController(t))
	workspaces.EXPECT().GetByID(gomock.Any(), "ws").Return(&domain.Workspace{Settings: domain.WorkspaceSettings{CustomFieldLabels: map[string]string{"custom_string_1": "会员编号"}}}, nil)
	workspaces.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil)
	sql.ExpectQuery("WITH observed AS").WillReturnRows(sqlmock.NewRows([]string{"key", "name", "type", "writable"}).AddRow("score", "评分", "number", true).AddRow("freeform", "freeform", "boolean", true).AddRow("newField", "新增未赋值字段", "object", true))
	fields, err := NewContactFileRepository(workspaces).Fields(context.Background(), "ws")
	require.NoError(t, err)
	byKey := map[string]domain.ContactFileField{}
	for _, field := range fields {
		byKey[field.Key] = field
	}
	assert.Equal(t, "会员编号 (custom_string_1)", byKey["custom_string_1"].Header)
	assert.Equal(t, "json", byKey["profile.attributes.newField"].Type)
	assert.Equal(t, "boolean", byKey["profile.attributes.freeform"].Type)
	assert.True(t, byKey["created_at"].ReadOnly)
	require.NoError(t, sql.ExpectationsWereMet())
}

func TestContactFileRepositoryLoadsProfilesInOneBatch(t *testing.T) {
	db, sql, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	workspaces := mocks.NewMockWorkspaceRepository(gomock.NewController(t))
	workspaces.EXPECT().GetConnection(gomock.Any(), "ws").Return(db, nil)
	sql.ExpectQuery("SELECT c.email, COALESCE").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"email", "status", "attributes", "tags"}).AddRow("user@example.com", "active", `{"score":0,"vip":false}`, `["会员"]`))
	contacts := []*domain.Contact{{Email: "user@example.com"}}
	err = NewContactFileRepository(workspaces).LoadProfiles(context.Background(), "ws", contacts)
	require.NoError(t, err)
	assert.Equal(t, float64(0), contacts[0].Profile.Attributes["score"])
	assert.Equal(t, false, contacts[0].Profile.Attributes["vip"])
	assert.Equal(t, []string{"会员"}, contacts[0].Profile.Tags)
	require.NoError(t, sql.ExpectationsWereMet())
}
