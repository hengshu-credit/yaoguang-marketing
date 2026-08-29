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
	mock.ExpectExec(`INSERT INTO customer_consents`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM customer_consents`).WithArgs(sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO customer_list_memberships`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM customer_list_memberships`).WithArgs(sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE contact_endpoints SET customer_id`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE contacts SET customer_id`).WithArgs(targetID, sourceID).WillReturnResult(sqlmock.NewResult(0, 1))
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
		WithArgs(sqlmock.AnyArg(), customerID, "email", EncryptedString("alice@example.com", "secret"), sqlmock.AnyArg(), "a***@example.com", true, true, JSONEqual(`{"source":"crm"}`), now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM customer_tags`).WithArgs(customerID).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`INSERT INTO customer_tags`).WithArgs(customerID, "vip", now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM customer_list_memberships`).WithArgs(customerID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO customer_list_memberships`).
		WithArgs(customerID, "list1", "active", now).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO contacts`).
		WithArgs("alice@example.com", "crm-new", "Asia/Shanghai", "zh-CN", customerID, now, now).
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
			Tags:       &tags, ListMemberships: []domain.CustomerListMembershipInput{{ListID: "list1", Status: "active"}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "updated", result.Action)
	assert.Equal(t, int64(4), result.Version)
	require.NoError(t, mock.ExpectationsWereMet())
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

func expectCustomerAggregateChildren(mock sqlmock.Sqlmock, now time.Time) {
	mock.ExpectQuery(`SELECT status, language, timezone, attributes, version, created_at, updated_at FROM customer_profiles`).
		WillReturnRows(sqlmock.NewRows([]string{"status", "language", "timezone", "attributes", "version", "created_at", "updated_at"}))
	mock.ExpectQuery(`SELECT id, identity_type, display_hint, verified, is_primary, enabled, metadata, created_at, updated_at FROM customer_identities`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "identity_type", "display_hint", "verified", "is_primary", "enabled", "metadata", "created_at", "updated_at"}))
	mock.ExpectQuery(`SELECT tag FROM customer_tags`).WillReturnRows(sqlmock.NewRows([]string{"tag"}))
	mock.ExpectQuery(`SELECT list_id, status, created_at, updated_at FROM customer_list_memberships`).
		WillReturnRows(sqlmock.NewRows([]string{"list_id", "status", "created_at", "updated_at"}))
}
