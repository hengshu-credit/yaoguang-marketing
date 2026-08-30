package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang/mock/gomock"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain/mocks"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var customerAggregateColumns = []string{
	"id", "customer_no", "external_user_id", "merged_into_id", "version", "created_at", "updated_at",
}

func TestCustomerRepositoryGetSupportsEveryWorkspaceLocalLocator(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	tests := []struct {
		name      string
		locator   domain.CustomerLocator
		query     string
		args      []driver.Value
		secretKey string
	}{
		{name: "uuid", locator: domain.CustomerLocator{CustomerID: "11111111-1111-4111-8111-111111111111"}, query: `WHERE c.id = \$1`, args: []driver.Value{"11111111-1111-4111-8111-111111111111"}},
		{name: "customer number", locator: domain.CustomerLocator{CustomerNo: "U0042202608300902030811111111111141118111111111111111"}, query: `WHERE c.customer_no = \$1`, args: []driver.Value{"U0042202608300902030811111111111141118111111111111111"}},
		{name: "external user ID", locator: domain.CustomerLocator{ExternalUserID: "crm-42"}, query: `WHERE c.external_user_id = \$1`, args: []driver.Value{"crm-42"}},
		{name: "normalized identity", locator: domain.CustomerLocator{Identity: &domain.CustomerIdentityLocator{Type: domain.CustomerIdentityEmail, Value: " Alice@Example.COM "}}, query: `JOIN customer_identities ci.*ci.identity_type = \$1.*ci.lookup_fingerprint = \$2`, secretKey: "secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secretKey := tt.secretKey
			if secretKey == "" {
				secretKey = "secret"
			}
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			require.NoError(t, err)
			defer db.Close()
			ctrl := gomock.NewController(t)
			workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace1").Return(db, nil)

			args := tt.args
			if tt.locator.Identity != nil {
				normalized, normalizeErr := domain.NormalizeCustomerIdentity(domain.CustomerIdentityInput{Type: tt.locator.Identity.Type, Value: tt.locator.Identity.Value})
				require.NoError(t, normalizeErr)
				fingerprint, fingerprintErr := domain.CustomerIdentityFingerprintForWorkspace(secretKey, "workspace1", normalized)
				require.NoError(t, fingerprintErr)
				args = []driver.Value{string(domain.CustomerIdentityEmail), fingerprint}
			}
			mock.ExpectQuery(tt.query).WithArgs(args...).WillReturnRows(sqlmock.NewRows(customerAggregateColumns).
				AddRow("11111111-1111-4111-8111-111111111111", "U0042202608300902030811111111111141118111111111111111", "crm-42", nil, 3, now, now))
			expectCustomerAggregateChildren(mock, now)

			repo, err := NewCustomerRepository(workspaceRepo, secretKey)
			require.NoError(t, err)
			customer, err := repo.Get(context.Background(), "workspace1", tt.locator)
			require.NoError(t, err)
			assert.Equal(t, "11111111-1111-4111-8111-111111111111", customer.ID)
			assert.Equal(t, "crm-42", *customer.ExternalUserID)
			assert.Empty(t, customer.Identities)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCustomerRepositoryGetRedirectsMergedSource(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace1").Return(db, nil)

	mock.ExpectQuery(`WHERE c.id = \$1`).WithArgs("11111111-1111-4111-8111-111111111111").
		WillReturnRows(sqlmock.NewRows(customerAggregateColumns).AddRow(
			"11111111-1111-4111-8111-111111111111", "U0042202608300902030811111111111141118111111111111111", nil,
			"22222222-2222-4222-8222-222222222222", 2, now, now,
		))
	mock.ExpectQuery(`WHERE c.id = \$1`).WithArgs("22222222-2222-4222-8222-222222222222").
		WillReturnRows(sqlmock.NewRows(customerAggregateColumns).AddRow(
			"22222222-2222-4222-8222-222222222222", "U0042202608300902030822222222222242228222222222222222", "known-1", nil, 4, now, now,
		))
	expectCustomerAggregateChildren(mock, now)

	repo, err := NewCustomerRepository(workspaceRepo, "secret")
	require.NoError(t, err)
	customer, err := repo.Get(context.Background(), "workspace1", domain.CustomerLocator{CustomerID: "11111111-1111-4111-8111-111111111111"})
	require.NoError(t, err)
	assert.Equal(t, "22222222-2222-4222-8222-222222222222", customer.ID)
	require.NotNil(t, customer.ResolvedFromCustomerID)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", *customer.ResolvedFromCustomerID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCustomerRepositoryGetPopulatesProfileCustomerID(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	customerID := "11111111-1111-4111-8111-111111111111"
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace1").Return(db, nil)
	mock.ExpectQuery(`WHERE c.id = \$1`).WithArgs(customerID).
		WillReturnRows(sqlmock.NewRows(customerAggregateColumns).AddRow(
			customerID, "U0042202608300902030811111111111141118111111111111111", "crm-42", nil, 3, now, now,
		))
	mock.ExpectQuery(`SELECT status, language, timezone, attributes, version, created_at, updated_at FROM customer_profiles`).
		WithArgs(customerID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "language", "timezone", "attributes", "version", "created_at", "updated_at"}).
			AddRow("active", "zh-CN", "Asia/Shanghai", []byte(`{"tier":"gold"}`), 2, now, now))
	mock.ExpectQuery(`SELECT id, identity_type, display_hint, verified, is_primary, enabled, metadata, created_at, updated_at FROM customer_identities`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "identity_type", "display_hint", "verified", "is_primary", "enabled", "metadata", "created_at", "updated_at"}))
	mock.ExpectQuery(`SELECT tag FROM customer_tags`).WillReturnRows(sqlmock.NewRows([]string{"tag"}))
	mock.ExpectQuery(`SELECT list_id, status, created_at, updated_at FROM customer_list_memberships`).
		WillReturnRows(sqlmock.NewRows([]string{"list_id", "status", "created_at", "updated_at"}))

	repo, err := NewCustomerRepository(workspaceRepo, "secret")
	require.NoError(t, err)
	customer, err := repo.Get(context.Background(), "workspace1", domain.CustomerLocator{CustomerID: customerID})
	require.NoError(t, err)
	require.NotNil(t, customer.Profile)
	assert.Equal(t, customerID, customer.Profile.CustomerID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCustomerRepositoryListUsesStableKeysetPaginationAndExcludesMergedByDefault(t *testing.T) {
	now := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace1").Return(db, nil)

	rows := sqlmock.NewRows(customerListColumns()).
		AddRow("11111111-1111-4111-8111-111111111111", "U0001202608301600000811111111111141118111111111111111", "crm-1", nil, 3, "active", "zh-CN", "Asia/Shanghai", []byte(`{"name":"Alice"}`), 2, now, now, now, now).
		AddRow("22222222-2222-4222-8222-222222222222", "U0001202608301559000822222222222242228222222222222222", nil, nil, 1, nil, nil, nil, nil, nil, nil, nil, now.Add(-time.Minute), now.Add(-time.Minute)).
		AddRow("33333333-3333-4333-8333-333333333333", "U0001202608301558000833333333333343338333333333333333", nil, nil, 1, nil, nil, nil, nil, nil, nil, nil, now.Add(-2*time.Minute), now.Add(-2*time.Minute))
	mock.ExpectQuery(`FROM customers c LEFT JOIN customer_profiles cp ON cp.customer_id = c.id WHERE c.merged_into_id IS NULL ORDER BY c.created_at DESC, c.id DESC LIMIT \$1`).
		WithArgs(3).
		WillReturnRows(rows)
	mock.ExpectQuery(`FROM customer_identities WHERE customer_id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"customer_id", "id", "identity_type", "display_hint", "verified", "is_primary", "enabled", "metadata", "created_at", "updated_at"}).
			AddRow("11111111-1111-4111-8111-111111111111", "identity-1", "email", "a***@example.com", true, true, true, []byte(`{}`), now, now))
	mock.ExpectQuery(`SELECT customer_id, tag FROM customer_tags WHERE customer_id = ANY\(\$1\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"customer_id", "tag"}).AddRow("11111111-1111-4111-8111-111111111111", "vip"))

	repo, err := NewCustomerRepository(workspaceRepo, "secret")
	require.NoError(t, err)
	response, err := repo.List(context.Background(), "workspace1", domain.CustomerListRequest{WorkspaceID: "workspace1", Limit: 2})
	require.NoError(t, err)
	require.Len(t, response.Customers, 2)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", response.Customers[0].ID)
	assert.Equal(t, "vip", response.Customers[0].Tags[0])
	assert.Equal(t, "a***@example.com", response.Customers[0].Identities[0].DisplayHint)
	require.NotEmpty(t, response.NextCursor)
	cursor, err := domain.DecodeCustomerListCursor(response.NextCursor)
	require.NoError(t, err)
	assert.Equal(t, "22222222-2222-4222-8222-222222222222", cursor.CustomerID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCustomerRepositoryListFindsExactEmailByWorkspaceFingerprint(t *testing.T) {
	now := time.Date(2026, 8, 30, 8, 0, 0, 0, time.UTC)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace1").Return(db, nil)
	normalized, err := domain.NormalizeCustomerIdentity(domain.CustomerIdentityInput{Type: domain.CustomerIdentityEmail, Value: "alice@example.com"})
	require.NoError(t, err)
	fingerprint, err := domain.CustomerIdentityFingerprintForWorkspace("secret", "workspace1", normalized)
	require.NoError(t, err)

	mock.ExpectQuery(`ci.identity_type = 'email' AND ci.lookup_fingerprint = \$2`).
		WithArgs("%alice@example.com%", fingerprint, 2).
		WillReturnRows(sqlmock.NewRows(customerListColumns()).AddRow(
			"11111111-1111-4111-8111-111111111111", "U0001202608301600000811111111111141118111111111111111", "crm-1", nil, 3,
			"active", nil, nil, []byte(`{"name":"Alice"}`), 2, now, now, now, now,
		))
	mock.ExpectQuery(`FROM customer_identities WHERE customer_id = ANY\(\$1\)`).
		WillReturnRows(sqlmock.NewRows([]string{"customer_id", "id", "identity_type", "display_hint", "verified", "is_primary", "enabled", "metadata", "created_at", "updated_at"}))
	mock.ExpectQuery(`SELECT customer_id, tag FROM customer_tags WHERE customer_id = ANY\(\$1\)`).
		WillReturnRows(sqlmock.NewRows([]string{"customer_id", "tag"}))

	repo, err := NewCustomerRepository(workspaceRepo, "secret")
	require.NoError(t, err)
	response, err := repo.List(context.Background(), "workspace1", domain.CustomerListRequest{WorkspaceID: "workspace1", Search: "alice@example.com", Limit: 1})
	require.NoError(t, err)
	require.Len(t, response.Customers, 1)
	assert.Equal(t, "crm-1", *response.Customers[0].ExternalUserID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func customerListColumns() []string {
	return []string{
		"id", "customer_no", "external_user_id", "merged_into_id", "version",
		"status", "language", "timezone", "attributes", "profile_version", "profile_created_at", "profile_updated_at",
		"created_at", "updated_at",
	}
}

func TestCustomerRepositoryGetMapsMissingRows(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	workspaceRepo.EXPECT().GetConnection(gomock.Any(), "workspace1").Return(db, nil)
	mock.ExpectQuery(`WHERE c.external_user_id = \$1`).WithArgs("missing").WillReturnRows(sqlmock.NewRows(customerAggregateColumns))

	repo, err := NewCustomerRepository(workspaceRepo, "secret")
	require.NoError(t, err)
	_, err = repo.Get(context.Background(), "workspace1", domain.CustomerLocator{ExternalUserID: "missing"})
	var notFound *domain.ErrCustomerNotFound
	assert.ErrorAs(t, err, &notFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCustomerRepositoryUpsertCreatesExternalOnlyCustomerAndReplays(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	expectWorkspaceTransaction(workspaceRepo, db, "workspace1")

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO customer_idempotency`).
		WithArgs("customer.upsert", "idem-1", "hash-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`WHERE c.external_user_id = \$1.*FOR UPDATE`).WithArgs("crm-42").
		WillReturnRows(sqlmock.NewRows(customerAggregateColumns))
	mock.ExpectQuery(`INSERT INTO customers.*RETURNING version`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "crm-42", now, now).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
	mock.ExpectExec(`UPDATE customer_idempotency SET customer_id`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "customer.upsert", "idem-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo, err := NewCustomerRepository(workspaceRepo, "secret")
	require.NoError(t, err)
	repo.now = func() time.Time { return now }
	result, err := repo.Upsert(context.Background(), domain.CustomerUpsertCommand{
		WorkspaceID: "workspace1", WorkspaceSequence: 42, IdempotencyKey: "idem-1", PayloadHash: "hash-1",
		Input: domain.CustomerUpsertInput{ExternalUserID: pointerTo("crm-42")},
	})
	require.NoError(t, err)
	assert.Equal(t, "created", result.Action)
	assert.Equal(t, int64(1), result.Version)
	assert.False(t, result.Replayed)
	assert.Equal(t, "crm-42", *result.ExternalUserID)
	assert.Regexp(t, `^U0042\d{14}08[0-9a-f]{32}$`, result.CustomerNo)
	require.NoError(t, mock.ExpectationsWereMet())

	expectWorkspaceTransaction(workspaceRepo, db, "workspace1")
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO customer_idempotency`).
		WithArgs("customer.upsert", "idem-1", "hash-1").WillReturnResult(sqlmock.NewResult(0, 0))
	response, err := json.Marshal(result)
	require.NoError(t, err)
	mock.ExpectQuery(`SELECT payload_hash, response FROM customer_idempotency`).
		WithArgs("customer.upsert", "idem-1").
		WillReturnRows(sqlmock.NewRows([]string{"payload_hash", "response"}).AddRow("hash-1", response))
	mock.ExpectCommit()

	replayed, err := repo.Upsert(context.Background(), domain.CustomerUpsertCommand{
		WorkspaceID: "workspace1", WorkspaceSequence: 42, IdempotencyKey: "idem-1", PayloadHash: "hash-1",
		Input: domain.CustomerUpsertInput{ExternalUserID: pointerTo("crm-42")},
	})
	require.NoError(t, err)
	assert.True(t, replayed.Replayed)
	assert.Equal(t, result.CustomerID, replayed.CustomerID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCustomerRepositoryUpsertRejectsIdempotencyPayloadConflict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	expectWorkspaceTransaction(workspaceRepo, db, "workspace1")
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO customer_idempotency`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT payload_hash, response FROM customer_idempotency`).
		WillReturnRows(sqlmock.NewRows([]string{"payload_hash", "response"}).AddRow("old-hash", []byte(`{}`)))
	mock.ExpectRollback()

	repo, err := NewCustomerRepository(workspaceRepo, "secret")
	require.NoError(t, err)
	_, err = repo.Upsert(context.Background(), domain.CustomerUpsertCommand{
		WorkspaceID: "workspace1", WorkspaceSequence: 42, IdempotencyKey: "idem-1", PayloadHash: "new-hash",
		Input: domain.CustomerUpsertInput{ExternalUserID: pointerTo("crm-42")},
	})
	var conflict *domain.ErrCustomerIdempotencyConflict
	assert.ErrorAs(t, err, &conflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCustomerRepositoryUpsertRejectsCrossCustomerOwners(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	customerID := "11111111-1111-4111-8111-111111111111"
	customerNo := "U0042202608300902030811111111111141118111111111111111"

	t.Run("external ID", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer db.Close()
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		expectWorkspaceTransaction(workspaceRepo, db, "workspace1")
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO customer_idempotency`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(`WHERE c.id = \$1.*FOR UPDATE`).WithArgs(customerID).
			WillReturnRows(sqlmock.NewRows(customerAggregateColumns).AddRow(customerID, customerNo, "crm-old", nil, 3, now, now))
		mock.ExpectQuery(`SELECT id FROM customers WHERE external_user_id`).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("22222222-2222-4222-8222-222222222222"))
		mock.ExpectRollback()

		repo, err := NewCustomerRepository(workspaceRepo, "secret")
		require.NoError(t, err)
		_, err = repo.Upsert(context.Background(), domain.CustomerUpsertCommand{
			WorkspaceID: "workspace1", WorkspaceSequence: 42, IdempotencyKey: "idem-ext", PayloadHash: "hash-ext",
			Input: domain.CustomerUpsertInput{Locator: &domain.CustomerLocator{CustomerID: customerID}, ExternalUserID: pointerTo("crm-taken")},
		})
		var conflict *domain.ErrCustomerExternalIDConflict
		assert.ErrorAs(t, err, &conflict)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("identity", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer db.Close()
		ctrl := gomock.NewController(t)
		workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
		expectWorkspaceTransaction(workspaceRepo, db, "workspace1")
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO customer_idempotency`).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(`WHERE c.id = \$1.*FOR UPDATE`).WithArgs(customerID).
			WillReturnRows(sqlmock.NewRows(customerAggregateColumns).AddRow(customerID, customerNo, nil, nil, 3, now, now))
		mock.ExpectQuery(`SELECT customer_id FROM customer_identities`).
			WillReturnRows(sqlmock.NewRows([]string{"customer_id"}).AddRow("22222222-2222-4222-8222-222222222222"))
		mock.ExpectRollback()

		repo, err := NewCustomerRepository(workspaceRepo, "secret")
		require.NoError(t, err)
		_, err = repo.Upsert(context.Background(), domain.CustomerUpsertCommand{
			WorkspaceID: "workspace1", WorkspaceSequence: 42, IdempotencyKey: "idem-identity", PayloadHash: "hash-identity",
			Input: domain.CustomerUpsertInput{Locator: &domain.CustomerLocator{CustomerID: customerID}, Identities: []domain.CustomerIdentityInput{{Type: domain.CustomerIdentityEmail, Value: "alice@example.com"}}},
		})
		var conflict *domain.ErrCustomerIdentityConflict
		assert.ErrorAs(t, err, &conflict)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestCustomerRepositoryUpsertDoesNotReplaceExternalIDWhenIdentityResolvesWithoutLocator(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	customerID := "11111111-1111-4111-8111-111111111111"
	customerNo := "U0042202608300902030811111111111141118111111111111111"
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	expectWorkspaceTransaction(workspaceRepo, db, "workspace1")
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO customer_idempotency`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`WHERE c.external_user_id = \$1.*FOR UPDATE`).WithArgs("crm-new").
		WillReturnRows(sqlmock.NewRows(customerAggregateColumns))
	normalized, err := domain.NormalizeCustomerIdentity(domain.CustomerIdentityInput{Type: domain.CustomerIdentityEmail, Value: "alice@example.com"})
	require.NoError(t, err)
	fingerprint, err := domain.CustomerIdentityFingerprintForWorkspace("secret", "workspace1", normalized)
	require.NoError(t, err)
	mock.ExpectQuery(`JOIN customer_identities ci.*FOR UPDATE`).WithArgs(domain.CustomerIdentityEmail, fingerprint).
		WillReturnRows(sqlmock.NewRows(customerAggregateColumns).AddRow(customerID, customerNo, "crm-old", nil, 3, now, now))
	mock.ExpectRollback()

	repo, err := NewCustomerRepository(workspaceRepo, "secret")
	require.NoError(t, err)
	_, err = repo.Upsert(context.Background(), domain.CustomerUpsertCommand{
		WorkspaceID: "workspace1", WorkspaceSequence: 42, IdempotencyKey: "idem-identity-new-ext", PayloadHash: "hash-identity-new-ext",
		Input: domain.CustomerUpsertInput{
			ExternalUserID: pointerTo("crm-new"),
			Identities:     []domain.CustomerIdentityInput{{Type: domain.CustomerIdentityEmail, Value: "alice@example.com"}},
		},
	})
	var conflict *domain.ErrCustomerExternalIDConflict
	assert.ErrorAs(t, err, &conflict)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMapCustomerMutationErrorUsesConstraintNames(t *testing.T) {
	tests := []struct {
		constraint string
		assertType func(t *testing.T, err error)
	}{
		{constraint: "uq_customers_external_user_id", assertType: func(t *testing.T, err error) {
			var target *domain.ErrCustomerExternalIDConflict
			assert.ErrorAs(t, err, &target)
		}},
		{constraint: "uq_customer_identities_lookup", assertType: func(t *testing.T, err error) {
			var target *domain.ErrCustomerIdentityConflict
			assert.ErrorAs(t, err, &target)
		}},
		{constraint: "uq_customers_customer_no", assertType: func(t *testing.T, err error) {
			var target *domain.ErrCustomerNumberConflict
			assert.ErrorAs(t, err, &target)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			err := mapCustomerMutationError(&pq.Error{Code: "23505", Constraint: tt.constraint, Message: "localized text is irrelevant"}, domain.CustomerIdentityEmail)
			tt.assertType(t, err)
		})
	}
}

func TestCustomerRepositoryMergeMovesAnonymousAggregateIntoKnownTarget(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	sourceID := "22222222-2222-4222-8222-222222222222"
	targetID := "11111111-1111-4111-8111-111111111111"
	sourceNo := "U0042202608300902030822222222222242228222222222222222"
	targetNo := "U0042202608300902030811111111111141118111111111111111"
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	expectWorkspaceTransaction(workspaceRepo, db, "workspace1")

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO customer_idempotency`).
		WithArgs("customer.merge", "merge-1", "merge-hash").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`WHERE c.id = \$1$`).WithArgs(sourceID).
		WillReturnRows(sqlmock.NewRows(customerAggregateColumns).AddRow(sourceID, sourceNo, nil, nil, 2, now, now))
	mock.ExpectQuery(`WHERE c.id = \$1$`).WithArgs(targetID).
		WillReturnRows(sqlmock.NewRows(customerAggregateColumns).AddRow(targetID, targetNo, "known-1", nil, 7, now, now))
	// Target UUID sorts first, so it must be locked first regardless of request order.
	mock.ExpectQuery(`WHERE c.id = \$1 FOR UPDATE`).WithArgs(targetID).
		WillReturnRows(sqlmock.NewRows(customerAggregateColumns).AddRow(targetID, targetNo, "known-1", nil, 7, now, now))
	mock.ExpectQuery(`WHERE c.id = \$1 FOR UPDATE`).WithArgs(sourceID).
		WillReturnRows(sqlmock.NewRows(customerAggregateColumns).AddRow(sourceID, sourceNo, nil, nil, 2, now, now))
	expectCustomerAggregateChildren(mock, now)
	mock.ExpectExec(`INSERT INTO customer_profiles.*SELECT \$1`).WithArgs(targetID, sourceID, now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM customer_profiles`).WithArgs(sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM customer_identities source_identity`).WithArgs(sourceID, targetID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE customer_identities source_identity SET is_primary = FALSE`).WithArgs(sourceID, targetID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE customer_identities SET customer_id`).WithArgs(targetID, sourceID, now).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`INSERT INTO customer_tags`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM customer_tags`).WithArgs(sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM customer_consents source_consent`).WithArgs(sourceID, targetID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE customer_consents SET customer_id`).WithArgs(targetID, sourceID, now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO customer_list_memberships`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM customer_list_memberships`).WithArgs(sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE contact_endpoints SET customer_id`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE contact_lists target_membership`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM contact_lists source_membership`).WithArgs(sourceID, targetID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE contact_lists SET customer_id`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE contact_segments SET customer_id`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE custom_events SET customer_id`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE contact_timeline SET customer_id`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE contact_automations SET customer_id`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE automation_trigger_log SET customer_id`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE message_history SET customer_id`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE email_queue SET customer_id`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE contacts source_contact.*CASE.*target_contact.customer_id = \$1.*THEN NULL.*ELSE \$1`).
		WithArgs(targetID, sourceID, now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE customers SET merged_into_id`).WithArgs(targetID, now, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`UPDATE customers SET version = version \+ 1.*RETURNING version`).WithArgs(now, targetID).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(8))
	mock.ExpectExec(`INSERT INTO customer_merge_log`).
		WithArgs(sqlmock.AnyArg(), sourceID, targetID, "user-1", "anonymous login", sqlmock.AnyArg(), now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE customer_idempotency SET customer_id`).
		WithArgs(targetID, sqlmock.AnyArg(), "customer.merge", "merge-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo, err := NewCustomerRepository(workspaceRepo, "secret")
	require.NoError(t, err)
	repo.now = func() time.Time { return now }
	result, err := repo.Merge(context.Background(), domain.CustomerMergeCommand{
		WorkspaceID: "workspace1", IdempotencyKey: "merge-1", PayloadHash: "merge-hash",
		Source: domain.CustomerLocator{CustomerID: sourceID}, Target: domain.CustomerLocator{CustomerID: targetID},
		ActorID: "user-1", Reason: "anonymous login",
	})
	require.NoError(t, err)
	assert.Equal(t, sourceID, result.SourceCustomerID)
	assert.Equal(t, targetID, result.TargetCustomerID)
	assert.Equal(t, int64(8), result.TargetVersion)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCustomerRepositoryMergeRejectsKnownSourceBeforeMutation(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	sourceID := "11111111-1111-4111-8111-111111111111"
	targetID := "22222222-2222-4222-8222-222222222222"
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	expectWorkspaceTransaction(workspaceRepo, db, "workspace1")
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO customer_idempotency`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`WHERE c.id = \$1$`).WithArgs(sourceID).WillReturnRows(sqlmock.NewRows(customerAggregateColumns).
		AddRow(sourceID, "source-no", "known-source", nil, 1, now, now))
	mock.ExpectQuery(`WHERE c.id = \$1$`).WithArgs(targetID).WillReturnRows(sqlmock.NewRows(customerAggregateColumns).
		AddRow(targetID, "target-no", "known-target", nil, 1, now, now))
	mock.ExpectQuery(`WHERE c.id = \$1 FOR UPDATE`).WithArgs(sourceID).WillReturnRows(sqlmock.NewRows(customerAggregateColumns).
		AddRow(sourceID, "source-no", "known-source", nil, 1, now, now))
	mock.ExpectQuery(`WHERE c.id = \$1 FOR UPDATE`).WithArgs(targetID).WillReturnRows(sqlmock.NewRows(customerAggregateColumns).
		AddRow(targetID, "target-no", "known-target", nil, 1, now, now))
	mock.ExpectRollback()

	repo, err := NewCustomerRepository(workspaceRepo, "secret")
	require.NoError(t, err)
	_, err = repo.Merge(context.Background(), domain.CustomerMergeCommand{
		WorkspaceID: "workspace1", IdempotencyKey: "merge-1", PayloadHash: "merge-hash",
		Source: domain.CustomerLocator{CustomerID: sourceID}, Target: domain.CustomerLocator{CustomerID: targetID},
	})
	var rejected *domain.ErrCustomerMergeRejected
	assert.ErrorAs(t, err, &rejected)
	assert.ErrorContains(t, err, "source must be anonymous")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCustomerRepositoryMergeRejectsInvalidResolvedRoles(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	sourceID := "11111111-1111-4111-8111-111111111111"
	targetID := "22222222-2222-4222-8222-222222222222"
	thirdID := "33333333-3333-4333-8333-333333333333"
	tests := []struct {
		name             string
		sourceMergedInto interface{}
		targetExternal   interface{}
		sameResolved     bool
		wantReason       string
	}{
		{name: "same resolved customer", targetExternal: "known", sameResolved: true, wantReason: "same customer"},
		{name: "anonymous target", targetExternal: nil, wantReason: "target must be known"},
		{name: "source merged elsewhere", sourceMergedInto: thirdID, targetExternal: "known", wantReason: "another customer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			require.NoError(t, err)
			defer db.Close()
			ctrl := gomock.NewController(t)
			workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
			expectWorkspaceTransaction(workspaceRepo, db, "workspace1")
			mock.ExpectBegin()
			mock.ExpectExec(`INSERT INTO customer_idempotency`).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectQuery(`WHERE c.id = \$1$`).WithArgs(sourceID).WillReturnRows(sqlmock.NewRows(customerAggregateColumns).
				AddRow(sourceID, "source-no", nil, tt.sourceMergedInto, 1, now, now))
			resolvedTargetID := targetID
			if tt.sameResolved {
				resolvedTargetID = sourceID
			}
			mock.ExpectQuery(`WHERE c.id = \$1$`).WithArgs(targetID).WillReturnRows(sqlmock.NewRows(customerAggregateColumns).
				AddRow(resolvedTargetID, "target-no", tt.targetExternal, nil, 1, now, now))
			if !tt.sameResolved {
				mock.ExpectQuery(`WHERE c.id = \$1 FOR UPDATE`).WithArgs(sourceID).WillReturnRows(sqlmock.NewRows(customerAggregateColumns).
					AddRow(sourceID, "source-no", nil, tt.sourceMergedInto, 1, now, now))
				mock.ExpectQuery(`WHERE c.id = \$1 FOR UPDATE`).WithArgs(targetID).WillReturnRows(sqlmock.NewRows(customerAggregateColumns).
					AddRow(targetID, "target-no", tt.targetExternal, nil, 1, now, now))
			}
			mock.ExpectRollback()

			repo, err := NewCustomerRepository(workspaceRepo, "secret")
			require.NoError(t, err)
			_, err = repo.Merge(context.Background(), domain.CustomerMergeCommand{
				WorkspaceID: "workspace1", IdempotencyKey: "merge-1", PayloadHash: "merge-hash",
				Source: domain.CustomerLocator{CustomerID: sourceID}, Target: domain.CustomerLocator{CustomerID: targetID},
			})
			var rejected *domain.ErrCustomerMergeRejected
			assert.ErrorAs(t, err, &rejected)
			assert.ErrorContains(t, err, tt.wantReason)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCustomerRepositoryMergeReplaysCompletedResult(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	expectWorkspaceTransaction(workspaceRepo, db, "workspace1")
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO customer_idempotency`).
		WithArgs("customer.merge", "merge-1", "merge-hash").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT payload_hash, response FROM customer_idempotency`).
		WithArgs("customer.merge", "merge-1").WillReturnRows(sqlmock.NewRows([]string{"payload_hash", "response"}).
		AddRow("merge-hash", []byte(`{"source_customer_id":"source","target_customer_id":"target","target_customer_no":"target-no","target_version":8}`)))
	mock.ExpectCommit()

	repo, err := NewCustomerRepository(workspaceRepo, "secret")
	require.NoError(t, err)
	result, err := repo.Merge(context.Background(), domain.CustomerMergeCommand{
		WorkspaceID: "workspace1", IdempotencyKey: "merge-1", PayloadHash: "merge-hash",
		Source: domain.CustomerLocator{CustomerID: "11111111-1111-4111-8111-111111111111"},
		Target: domain.CustomerLocator{CustomerID: "22222222-2222-4222-8222-222222222222"},
	})
	require.NoError(t, err)
	assert.True(t, result.Replayed)
	assert.Equal(t, "target", result.TargetCustomerID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCustomerRepositoryUpsertAtomicallyUpdatesProfileIdentityTagsListsAndContactProjection(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	ctrl := gomock.NewController(t)
	workspaceRepo := mocks.NewMockWorkspaceRepository(ctrl)
	expectWorkspaceTransaction(workspaceRepo, db, "workspace1")

	customerID := "11111111-1111-4111-8111-111111111111"
	customerNo := "U0042202608300902030811111111111141118111111111111111"
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO customer_idempotency`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`WHERE c.id = \$1.*FOR UPDATE`).WithArgs(customerID).
		WillReturnRows(sqlmock.NewRows(customerAggregateColumns).AddRow(customerID, customerNo, "crm-old", nil, 3, now, now))
	mock.ExpectQuery(`SELECT id FROM customers WHERE external_user_id = \$1 AND id <> \$2`).
		WithArgs("crm-new", customerID).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT customer_id FROM customer_identities WHERE identity_type = \$1 AND lookup_fingerprint = \$2`).
		WithArgs("email", sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"customer_id"}))
	mock.ExpectQuery(`UPDATE customers SET external_user_id = \$2.*RETURNING version`).
		WithArgs(customerID, "crm-new", now).WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(4))
	mock.ExpectQuery(`SELECT status, language, timezone, attributes, version FROM customer_profiles.*FOR UPDATE`).
		WithArgs(customerID).WillReturnRows(sqlmock.NewRows([]string{"status", "language", "timezone", "attributes", "version"}).
		AddRow("lead", "zh-CN", "Asia/Shanghai", []byte(`{"a":1,"nested":{"keep":1,"drop":2}}`), 2))
	mock.ExpectExec(`INSERT INTO customer_profiles`).
		WithArgs(customerID, "active", "zh-CN", "Asia/Shanghai", JSONEqual(`{"a":1,"nested":{"keep":1,"new":3}}`), 3, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE customer_identities SET is_primary = FALSE`).
		WithArgs(customerID, "email").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO customer_identities`).
		WithArgs(sqlmock.AnyArg(), customerID, "email", EncryptedString("alice@example.com", "secret"), sqlmock.AnyArg(), "a***@example.com", true, true, true, JSONEqual(`{"source":"crm"}`), now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM customer_tags`).WithArgs(customerID).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`INSERT INTO customer_tags`).WithArgs(customerID, "vip", now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM customer_list_memberships`).WithArgs(customerID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO customer_list_memberships`).
		WithArgs(customerID, "list1", "active", now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE contacts SET email = COALESCE`).
		WithArgs(customerID, "crm-new", "Asia/Shanghai", "zh-CN", now, "alice@example.com").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO contacts`).
		WithArgs("alice@example.com", "crm-new", "Asia/Shanghai", "zh-CN", customerID, now, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM contact_lists WHERE customer_id`).WithArgs(customerID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT email FROM contacts WHERE customer_id`).WithArgs(customerID).
		WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("alice@example.com"))
	mock.ExpectExec(`INSERT INTO contact_lists`).WithArgs("alice@example.com", "list1", "active", now, customerID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE customer_idempotency SET customer_id`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	patch := domain.CustomerAttributesPatch{Merge: json.RawMessage(`{"nested":{"new":3}}`), Unset: []string{"nested.drop"}}
	tags := []string{"vip"}
	repo, err := NewCustomerRepository(workspaceRepo, "secret")
	require.NoError(t, err)
	repo.now = func() time.Time { return now }
	result, err := repo.Upsert(context.Background(), domain.CustomerUpsertCommand{
		WorkspaceID: "workspace1", WorkspaceSequence: 42, IdempotencyKey: "idem-rich", PayloadHash: "hash-rich",
		Input: domain.CustomerUpsertInput{
			Locator: &domain.CustomerLocator{CustomerID: customerID}, ExternalUserID: pointerTo("crm-new"),
			Profile:    &domain.CustomerProfilePatch{Status: pointerTo("active"), Attributes: &patch},
			Identities: []domain.CustomerIdentityInput{{Type: domain.CustomerIdentityEmail, Value: "Alice@Example.com", Primary: true, Verified: true, Metadata: map[string]interface{}{"source": "crm"}}},
			Tags:       &tags, ListMemberships: customerListMembershipsPointer([]domain.CustomerListMembershipInput{{ListID: "list1", Status: "active"}}),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "updated", result.Action)
	assert.Equal(t, int64(4), result.Version)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectCustomerContactUpdatesExistingProjectionWithoutEmailInPatch(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE contacts SET email = COALESCE`).
		WithArgs("11111111-1111-4111-8111-111111111111", "crm-42", "Asia/Shanghai", "zh-CN", now, "").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	tx, err := db.Begin()
	require.NoError(t, err)

	err = projectCustomerContact(context.Background(), tx, &domain.Customer{
		ID: "11111111-1111-4111-8111-111111111111", ExternalUserID: pointerTo("crm-42"),
	}, &customerProfileProjection{Language: pointerTo("zh-CN"), Timezone: pointerTo("Asia/Shanghai")}, nil, false, now)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectCustomerContactMovesProjectionToNewPrimaryEmail(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE contacts SET email = COALESCE\(NULLIF\(\$6, ''\), email\)`).
		WithArgs("11111111-1111-4111-8111-111111111111", nil, nil, nil, now, "new@example.com").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	tx, err := db.Begin()
	require.NoError(t, err)

	err = projectCustomerContact(context.Background(), tx, &domain.Customer{
		ID: "11111111-1111-4111-8111-111111111111",
	}, nil, []domain.CustomerIdentityInput{{
		Type: domain.CustomerIdentityEmail, Value: "new@example.com", Primary: true,
	}}, false, now)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProjectCustomerContactReportsPrimaryEmailCollisionAsIdentityConflict(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE contacts SET email = COALESCE\(NULLIF\(\$6, ''\), email\)`).
		WithArgs("11111111-1111-4111-8111-111111111111", nil, nil, nil, now, "owned@example.com").
		WillReturnError(&pq.Error{Code: "23505", Constraint: "contacts_pkey"})
	mock.ExpectRollback()
	tx, err := db.Begin()
	require.NoError(t, err)

	err = projectCustomerContact(context.Background(), tx, &domain.Customer{
		ID: "11111111-1111-4111-8111-111111111111",
	}, nil, []domain.CustomerIdentityInput{{
		Type: domain.CustomerIdentityEmail, Value: "owned@example.com", Primary: true,
	}}, false, now)
	var conflict *domain.ErrCustomerIdentityConflict
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, domain.CustomerIdentityEmail, conflict.IdentityType)
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertCustomerIdentitiesPersistsDisabledAndRejectsLostConcurrentClaim(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	customerID := "11111111-1111-4111-8111-111111111111"

	t.Run("disabled", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer db.Close()
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO customer_identities.*enabled.*DO UPDATE SET.*enabled = EXCLUDED.enabled`).
			WithArgs(sqlmock.AnyArg(), customerID, "email", sqlmock.AnyArg(), sqlmock.AnyArg(), "d***@example.com", false, false, false, JSONEqual(`{}`), now).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		tx, err := db.Begin()
		require.NoError(t, err)
		repo := &CustomerPostgresRepository{secretKey: "secret"}
		enabled := false
		err = repo.upsertCustomerIdentities(context.Background(), tx, "workspace1", customerID, []domain.CustomerIdentityInput{{
			Type: domain.CustomerIdentityEmail, Value: "disabled@example.com", Enabled: &enabled,
		}}, now)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("concurrent owner won", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer db.Close()
		mock.ExpectBegin()
		mock.ExpectExec(`INSERT INTO customer_identities`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()
		tx, err := db.Begin()
		require.NoError(t, err)
		repo := &CustomerPostgresRepository{secretKey: "secret"}
		err = repo.upsertCustomerIdentities(context.Background(), tx, "workspace1", customerID, []domain.CustomerIdentityInput{{
			Type: domain.CustomerIdentityEmail, Value: "claimed@example.com",
		}}, now)
		var conflict *domain.ErrCustomerIdentityConflict
		assert.ErrorAs(t, err, &conflict)
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestProjectCustomerContactSkipsDisabledEmailAndRejectsCrossCustomerReassignment(t *testing.T) {
	now := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	customer := &domain.Customer{ID: "11111111-1111-4111-8111-111111111111"}

	t.Run("disabled email", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		mock.ExpectBegin()
		mock.ExpectCommit()
		tx, err := db.Begin()
		require.NoError(t, err)
		enabled := false
		require.NoError(t, projectCustomerContact(context.Background(), tx, customer, nil, []domain.CustomerIdentityInput{{
			Type: domain.CustomerIdentityEmail, Value: "disabled@example.com", Enabled: &enabled,
		}}, true, now))
		require.NoError(t, tx.Commit())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("disable existing email", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer db.Close()
		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE contacts SET customer_id = NULL.*WHERE customer_id = \$1 AND email = \$2`).
			WithArgs(customer.ID, "disabled@example.com", now).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()
		tx, err := db.Begin()
		require.NoError(t, err)
		enabled := false
		require.NoError(t, projectCustomerContact(context.Background(), tx, customer, nil, []domain.CustomerIdentityInput{{
			Type: domain.CustomerIdentityEmail, Value: "disabled@example.com", Enabled: &enabled,
		}}, false, now))
		require.NoError(t, tx.Commit())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("email belongs to another customer", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
		require.NoError(t, err)
		defer db.Close()
		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE contacts SET email = COALESCE`).WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`INSERT INTO contacts.*WHERE contacts.customer_id IS NULL OR contacts.customer_id = EXCLUDED.customer_id`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()
		tx, err := db.Begin()
		require.NoError(t, err)
		err = projectCustomerContact(context.Background(), tx, customer, nil, []domain.CustomerIdentityInput{{
			Type: domain.CustomerIdentityEmail, Value: "claimed@example.com",
		}}, true, now)
		var conflict *domain.ErrCustomerIdentityConflict
		assert.ErrorAs(t, err, &conflict)
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func expectWorkspaceTransaction(workspaceRepo *mocks.MockWorkspaceRepository, db *sql.DB, workspaceID string) {
	workspaceRepo.EXPECT().WithWorkspaceTransaction(gomock.Any(), workspaceID, gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ string, fn func(*sql.Tx) error) error {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			if err := fn(tx); err != nil {
				_ = tx.Rollback()
				return err
			}
			return tx.Commit()
		},
	)
}

type jsonEqualMatcher string

func JSONEqual(expected string) sqlmock.Argument { return jsonEqualMatcher(expected) }

func (matcher jsonEqualMatcher) Match(value driver.Value) bool {
	actual, ok := value.([]byte)
	if !ok {
		if text, textOK := value.(string); textOK {
			actual = []byte(text)
		} else {
			return false
		}
	}
	var expectedValue, actualValue interface{}
	return json.Unmarshal([]byte(matcher), &expectedValue) == nil &&
		json.Unmarshal(actual, &actualValue) == nil && assert.ObjectsAreEqual(expectedValue, actualValue)
}

type encryptedStringMatcher struct {
	plaintext string
	secretKey string
}

func EncryptedString(plaintext, secretKey string) sqlmock.Argument {
	return encryptedStringMatcher{plaintext: plaintext, secretKey: secretKey}
}

func (matcher encryptedStringMatcher) Match(value driver.Value) bool {
	ciphertext, ok := value.(string)
	if !ok || ciphertext == matcher.plaintext {
		return false
	}
	decrypted, err := domain.DecryptString(ciphertext, matcher.secretKey)
	return err == nil && decrypted == matcher.plaintext
}

func pointerTo(value string) *string { return &value }

func customerListMembershipsPointer(value []domain.CustomerListMembershipInput) *[]domain.CustomerListMembershipInput {
	return &value
}

func expectCustomerAggregateChildren(mock sqlmock.Sqlmock, now time.Time) {
	mock.ExpectQuery(`SELECT status, language, timezone, attributes, version, created_at, updated_at FROM customer_profiles`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "language", "timezone", "attributes", "version", "created_at", "updated_at"}))
	mock.ExpectQuery(`SELECT id, identity_type, display_hint, verified, is_primary, enabled, metadata, created_at, updated_at FROM customer_identities`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "identity_type", "display_hint", "verified", "is_primary", "enabled", "metadata", "created_at", "updated_at"}))
	mock.ExpectQuery(`SELECT tag FROM customer_tags`).WillReturnRows(sqlmock.NewRows([]string{"tag"}))
	mock.ExpectQuery(`SELECT list_id, status, created_at, updated_at FROM customer_list_memberships`).
		WillReturnRows(sqlmock.NewRows([]string{"list_id", "status", "created_at", "updated_at"}))
}
