package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	pkgcrypto "github.com/hengshu-credit/yaoguang-marketing/pkg/crypto"
	"github.com/lib/pq"
)

var customerColumns = `c.id, c.customer_no, c.external_user_id, c.merged_into_id,
	c.version, c.created_at, c.updated_at`

// CustomerPostgresRepository persists the Customer authority aggregate inside
// the database dedicated to each Workspace.
type CustomerPostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	secretKey     string
	now           func() time.Time
}

var _ domain.CustomerRepository = (*CustomerPostgresRepository)(nil)

func NewCustomerRepository(workspaceRepo domain.WorkspaceRepository, secretKey string) (*CustomerPostgresRepository, error) {
	if workspaceRepo == nil {
		return nil, errors.New("workspace repository is required")
	}
	if strings.TrimSpace(secretKey) == "" {
		return nil, errors.New("customer identity encryption secret is required")
	}
	return &CustomerPostgresRepository{workspaceRepo: workspaceRepo, secretKey: secretKey, now: time.Now}, nil
}

func (r *CustomerPostgresRepository) Get(ctx context.Context, workspaceID string, locator domain.CustomerLocator) (*domain.Customer, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, errors.New("workspace ID is required")
	}
	if err := locator.Validate(); err != nil {
		return nil, fmt.Errorf("invalid customer locator: %w", err)
	}
	db, err := r.workspaceRepo.GetConnection(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("get workspace connection: %w", err)
	}

	customer, err := r.findCustomer(ctx, db, workspaceID, locator)
	if err != nil {
		return nil, err
	}
	if customer.MergedIntoID != nil {
		sourceID := customer.ID
		customer, err = r.findCustomer(ctx, db, workspaceID, domain.CustomerLocator{CustomerID: *customer.MergedIntoID})
		if err != nil {
			return nil, fmt.Errorf("resolve merged customer %s: %w", sourceID, err)
		}
		customer.ResolvedFromCustomerID = &sourceID
	}
	if err := r.loadCustomerChildren(ctx, db, customer); err != nil {
		return nil, err
	}
	return customer, nil
}

type customerQueryer interface {
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func (r *CustomerPostgresRepository) findCustomer(ctx context.Context, db customerQueryer, workspaceID string, locator domain.CustomerLocator) (*domain.Customer, error) {
	return r.findCustomerWithLock(ctx, db, workspaceID, locator, false)
}

func (r *CustomerPostgresRepository) findCustomerWithLock(ctx context.Context, db customerQueryer, workspaceID string, locator domain.CustomerLocator, lock bool) (*domain.Customer, error) {
	query := "SELECT " + customerColumns + " FROM customers c"
	var args []interface{}
	switch {
	case locator.CustomerID != "":
		query += " WHERE c.id = $1"
		args = []interface{}{locator.CustomerID}
	case locator.CustomerNo != "":
		query += " WHERE c.customer_no = $1"
		args = []interface{}{locator.CustomerNo}
	case locator.ExternalUserID != "":
		query += " WHERE c.external_user_id = $1"
		args = []interface{}{locator.ExternalUserID}
	case locator.Identity != nil:
		normalized, err := domain.NormalizeCustomerIdentity(domain.CustomerIdentityInput{Type: locator.Identity.Type, Value: locator.Identity.Value})
		if err != nil {
			return nil, fmt.Errorf("normalize customer identity locator: %w", err)
		}
		fingerprint, err := domain.CustomerIdentityFingerprintForWorkspace(r.secretKey, workspaceID, normalized)
		if err != nil {
			return nil, err
		}
		query += ` JOIN customer_identities ci ON ci.customer_id = c.id
			WHERE ci.identity_type = $1 AND ci.lookup_fingerprint = $2 AND ci.enabled`
		args = []interface{}{normalized.Type, fingerprint}
	default:
		return nil, errors.New("customer locator is required")
	}
	if lock {
		query += " FOR UPDATE"
	}

	row := db.QueryRowContext(ctx, query, args...)
	var customer domain.Customer
	var externalID, mergedInto sql.NullString
	if err := row.Scan(
		&customer.ID, &customer.CustomerNo, &externalID, &mergedInto,
		&customer.Version, &customer.CreatedAt, &customer.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &domain.ErrCustomerNotFound{}
		}
		return nil, fmt.Errorf("query customer: %w", err)
	}
	if externalID.Valid {
		customer.ExternalUserID = &externalID.String
	}
	if mergedInto.Valid {
		customer.MergedIntoID = &mergedInto.String
	}
	return &customer, nil
}

func (r *CustomerPostgresRepository) Upsert(ctx context.Context, command domain.CustomerUpsertCommand) (*domain.CustomerMutationResult, error) {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.PayloadHash = strings.TrimSpace(command.PayloadHash)
	if command.WorkspaceID == "" {
		return nil, errors.New("workspace ID is required")
	}
	if command.WorkspaceSequence < domain.MinWorkspaceSequence || command.WorkspaceSequence > domain.MaxWorkspaceSequence {
		return nil, fmt.Errorf("workspace sequence must be between %d and %d", domain.MinWorkspaceSequence, domain.MaxWorkspaceSequence)
	}
	if command.IdempotencyKey == "" || command.PayloadHash == "" {
		return nil, errors.New("idempotency key and payload hash are required")
	}
	if err := command.Input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid customer input: %w", err)
	}

	var result *domain.CustomerMutationResult
	err := r.workspaceRepo.WithWorkspaceTransaction(ctx, command.WorkspaceID, func(tx *sql.Tx) error {
		claimed, replayResponse, err := claimCustomerIdempotency(ctx, tx, "customer.upsert", command.IdempotencyKey, command.PayloadHash)
		if err != nil {
			return err
		}
		if !claimed {
			var replayed domain.CustomerMutationResult
			if err := json.Unmarshal(replayResponse, &replayed); err != nil {
				return fmt.Errorf("decode customer idempotency response: %w", err)
			}
			replayed.Replayed = true
			result = &replayed
			return nil
		}

		customer, err := r.resolveCustomerForUpsert(ctx, tx, command.WorkspaceID, command.Input)
		if err != nil {
			return err
		}
		now := r.now().UTC()
		action := "updated"
		if customer == nil {
			action = "created"
			customerID := uuid.New()
			customerNo, err := domain.GenerateCustomerNumber(command.WorkspaceSequence, now, customerID)
			if err != nil {
				return err
			}
			customer = &domain.Customer{ID: customerID.String(), CustomerNo: customerNo, ExternalUserID: command.Input.ExternalUserID, CreatedAt: now, UpdatedAt: now}
			if err := tx.QueryRowContext(ctx, `INSERT INTO customers (
				id, customer_no, external_user_id, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5) RETURNING version`,
				customer.ID, customer.CustomerNo, customer.ExternalUserID, now, now,
			).Scan(&customer.Version); err != nil {
				return mapCustomerMutationError(err, "")
			}
		} else {
			if command.Input.ExternalUserID != nil {
				customer.ExternalUserID = command.Input.ExternalUserID
				if err := tx.QueryRowContext(ctx, `UPDATE customers SET external_user_id = $2,
					version = version + 1, updated_at = $3 WHERE id = $1 RETURNING version`,
					customer.ID, *command.Input.ExternalUserID, now,
				).Scan(&customer.Version); err != nil {
					return mapCustomerMutationError(err, "")
				}
			} else if err := tx.QueryRowContext(ctx, `UPDATE customers SET version = version + 1,
				updated_at = $2 WHERE id = $1 RETURNING version`, customer.ID, now).Scan(&customer.Version); err != nil {
				return mapCustomerMutationError(err, "")
			}
		}

		profile, err := upsertCustomerProfile(ctx, tx, customer.ID, command.Input.Profile, now)
		if err != nil {
			return err
		}
		if err := r.upsertCustomerIdentities(ctx, tx, command.WorkspaceID, customer.ID, command.Input.Identities, now); err != nil {
			return err
		}
		if err := replaceCustomerTags(ctx, tx, customer.ID, command.Input.Tags, now); err != nil {
			return err
		}
		if err := replaceCustomerListMemberships(ctx, tx, customer.ID, command.Input.ListMemberships, now); err != nil {
			return err
		}
		if err := projectCustomerContact(ctx, tx, customer, profile, command.Input.Identities, now); err != nil {
			return err
		}

		result = &domain.CustomerMutationResult{
			CustomerID: customer.ID, CustomerNo: customer.CustomerNo, ExternalUserID: customer.ExternalUserID,
			Action: action, Version: customer.Version,
		}
		response, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("encode customer idempotency response: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE customer_idempotency SET customer_id = $1, response = $2, updated_at = CURRENT_TIMESTAMP WHERE operation = $3 AND idempotency_key = $4`,
			customer.ID, response, "customer.upsert", command.IdempotencyKey); err != nil {
			return fmt.Errorf("store customer idempotency response: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func claimCustomerIdempotency(ctx context.Context, tx *sql.Tx, operation, key, payloadHash string) (bool, []byte, error) {
	result, err := tx.ExecContext(ctx, `INSERT INTO customer_idempotency (operation, idempotency_key, payload_hash)
		VALUES ($1, $2, $3) ON CONFLICT (operation, idempotency_key) DO NOTHING`, operation, key, payloadHash)
	if err != nil {
		return false, nil, fmt.Errorf("claim customer idempotency key: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, nil, fmt.Errorf("read customer idempotency claim: %w", err)
	}
	if affected == 1 {
		return true, nil, nil
	}

	var storedHash string
	var response []byte
	if err := tx.QueryRowContext(ctx, `SELECT payload_hash, response FROM customer_idempotency
		WHERE operation = $1 AND idempotency_key = $2 FOR UPDATE`, operation, key).Scan(&storedHash, &response); err != nil {
		return false, nil, fmt.Errorf("read customer idempotency replay: %w", err)
	}
	if storedHash != payloadHash {
		return false, nil, &domain.ErrCustomerIdempotencyConflict{}
	}
	if len(response) == 0 {
		return false, nil, errors.New("customer idempotency response is incomplete")
	}
	return false, response, nil
}

func (r *CustomerPostgresRepository) Merge(ctx context.Context, command domain.CustomerMergeCommand) (*domain.CustomerMergeResult, error) {
	command.WorkspaceID = strings.TrimSpace(command.WorkspaceID)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	command.PayloadHash = strings.TrimSpace(command.PayloadHash)
	command.ActorID = strings.TrimSpace(command.ActorID)
	command.Reason = strings.TrimSpace(command.Reason)
	if command.WorkspaceID == "" || command.IdempotencyKey == "" || command.PayloadHash == "" {
		return nil, errors.New("workspace ID, idempotency key, and payload hash are required")
	}
	if err := command.Source.Validate(); err != nil {
		return nil, fmt.Errorf("invalid source customer locator: %w", err)
	}
	if err := command.Target.Validate(); err != nil {
		return nil, fmt.Errorf("invalid target customer locator: %w", err)
	}

	var result *domain.CustomerMergeResult
	err := r.workspaceRepo.WithWorkspaceTransaction(ctx, command.WorkspaceID, func(tx *sql.Tx) error {
		claimed, replayResponse, err := claimCustomerIdempotency(ctx, tx, "customer.merge", command.IdempotencyKey, command.PayloadHash)
		if err != nil {
			return err
		}
		if !claimed {
			var replayed domain.CustomerMergeResult
			if err := json.Unmarshal(replayResponse, &replayed); err != nil {
				return fmt.Errorf("decode customer merge idempotency response: %w", err)
			}
			replayed.Replayed = true
			result = &replayed
			return nil
		}

		sourceCandidate, err := r.findCustomer(ctx, tx, command.WorkspaceID, command.Source)
		if err != nil {
			return fmt.Errorf("resolve merge source: %w", err)
		}
		targetCandidate, err := r.findCustomer(ctx, tx, command.WorkspaceID, command.Target)
		if err != nil {
			return fmt.Errorf("resolve merge target: %w", err)
		}
		if sourceCandidate.ID == targetCandidate.ID {
			return &domain.ErrCustomerMergeRejected{Reason: "source and target resolve to the same customer"}
		}

		lockIDs := []string{sourceCandidate.ID, targetCandidate.ID}
		sort.Strings(lockIDs)
		locked := make(map[string]*domain.Customer, 2)
		for _, customerID := range lockIDs {
			customer, err := r.findCustomerWithLock(ctx, tx, command.WorkspaceID, domain.CustomerLocator{CustomerID: customerID}, true)
			if err != nil {
				return fmt.Errorf("lock merge customer %s: %w", customerID, err)
			}
			locked[customerID] = customer
		}
		source := locked[sourceCandidate.ID]
		target := locked[targetCandidate.ID]
		if source.MergedIntoID != nil {
			if *source.MergedIntoID != target.ID {
				return &domain.ErrCustomerMergeRejected{Reason: "source was already merged into another customer"}
			}
			result = &domain.CustomerMergeResult{
				SourceCustomerID: source.ID, TargetCustomerID: target.ID,
				TargetCustomerNo: target.CustomerNo, TargetVersion: target.Version,
			}
			return storeCustomerIdempotencyResponse(ctx, tx, "customer.merge", command.IdempotencyKey, target.ID, result)
		}
		if target.MergedIntoID != nil {
			return &domain.ErrCustomerMergeRejected{Reason: "target customer has already been merged"}
		}
		if source.ExternalUserID != nil {
			return &domain.ErrCustomerMergeRejected{Reason: "source must be anonymous and have no external user ID"}
		}
		if target.ExternalUserID == nil {
			return &domain.ErrCustomerMergeRejected{Reason: "target must be known and have an external user ID"}
		}
		if err := r.loadCustomerChildren(ctx, tx, source); err != nil {
			return fmt.Errorf("snapshot merge source: %w", err)
		}
		snapshot, err := json.Marshal(source)
		if err != nil {
			return fmt.Errorf("encode merge source snapshot: %w", err)
		}
		now := r.now().UTC()
		if err := moveCustomerAggregate(ctx, tx, source.ID, target.ID, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE customers SET merged_into_id = $1, merged_at = $2,
			version = version + 1, updated_at = $2 WHERE id = $3`, target.ID, now, source.ID); err != nil {
			return fmt.Errorf("mark merge source redirected: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `UPDATE customers SET version = version + 1, updated_at = $1
			WHERE id = $2 RETURNING version`, now, target.ID).Scan(&target.Version); err != nil {
			return fmt.Errorf("increment merge target version: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO customer_merge_log (
			id, source_customer_id, target_customer_id, actor_id, reason, source_snapshot, created_at
		) VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7)`,
			uuid.New(), source.ID, target.ID, command.ActorID, command.Reason, snapshot, now); err != nil {
			return fmt.Errorf("write customer merge audit: %w", err)
		}
		result = &domain.CustomerMergeResult{
			SourceCustomerID: source.ID, TargetCustomerID: target.ID,
			TargetCustomerNo: target.CustomerNo, TargetVersion: target.Version,
		}
		return storeCustomerIdempotencyResponse(ctx, tx, "customer.merge", command.IdempotencyKey, target.ID, result)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func moveCustomerAggregate(ctx context.Context, tx *sql.Tx, sourceID, targetID string, now time.Time) error {
	statements := []struct {
		query string
		args  []interface{}
	}{
		{query: `INSERT INTO customer_profiles (customer_id, status, language, timezone, attributes, version, created_at, updated_at)
			SELECT $1, status, language, timezone, attributes, version, created_at, $3
			FROM customer_profiles WHERE customer_id = $2
			ON CONFLICT (customer_id) DO UPDATE SET
				status = COALESCE(customer_profiles.status, EXCLUDED.status),
				language = COALESCE(customer_profiles.language, EXCLUDED.language),
				timezone = COALESCE(customer_profiles.timezone, EXCLUDED.timezone),
				attributes = EXCLUDED.attributes || customer_profiles.attributes,
				version = customer_profiles.version + 1, updated_at = EXCLUDED.updated_at`, args: []interface{}{targetID, sourceID, now}},
		{query: `DELETE FROM customer_profiles WHERE customer_id = $1`, args: []interface{}{sourceID}},
		{query: `DELETE FROM customer_identities source_identity USING customer_identities target_identity
			WHERE source_identity.customer_id = $1 AND target_identity.customer_id = $2
			AND source_identity.identity_type = target_identity.identity_type
			AND source_identity.lookup_fingerprint = target_identity.lookup_fingerprint`, args: []interface{}{sourceID, targetID}},
		{query: `UPDATE customer_identities source_identity SET is_primary = FALSE
			FROM customer_identities target_identity
			WHERE source_identity.customer_id = $1 AND target_identity.customer_id = $2
			AND source_identity.identity_type = target_identity.identity_type
			AND source_identity.is_primary AND target_identity.is_primary AND target_identity.enabled`, args: []interface{}{sourceID, targetID}},
		{query: `UPDATE customer_identities SET customer_id = $1, updated_at = $3 WHERE customer_id = $2`, args: []interface{}{targetID, sourceID, now}},
		{query: `INSERT INTO customer_tags (customer_id, tag, created_at)
			SELECT $1, tag, created_at FROM customer_tags WHERE customer_id = $2
			ON CONFLICT (customer_id, tag) DO NOTHING`, args: []interface{}{targetID, sourceID}},
		{query: `DELETE FROM customer_tags WHERE customer_id = $1`, args: []interface{}{sourceID}},
		{query: `INSERT INTO customer_consents (id, customer_id, purpose, channel, status, source, valid_from, revoked_at, metadata, created_at, updated_at)
			SELECT id, $1, purpose, channel, status, source, valid_from, revoked_at, metadata, created_at, updated_at
			FROM customer_consents WHERE customer_id = $2
			ON CONFLICT (customer_id, purpose, channel) DO NOTHING`, args: []interface{}{targetID, sourceID}},
		{query: `DELETE FROM customer_consents WHERE customer_id = $1`, args: []interface{}{sourceID}},
		{query: `INSERT INTO customer_list_memberships (customer_id, list_id, status, created_at, updated_at)
			SELECT $1, list_id, status, created_at, updated_at FROM customer_list_memberships WHERE customer_id = $2
			ON CONFLICT (customer_id, list_id) DO NOTHING`, args: []interface{}{targetID, sourceID}},
		{query: `DELETE FROM customer_list_memberships WHERE customer_id = $1`, args: []interface{}{sourceID}},
		{query: `UPDATE contact_endpoints SET customer_id = $1 WHERE customer_id = $2`, args: []interface{}{targetID, sourceID}},
		{query: `UPDATE contacts SET customer_id = $1 WHERE customer_id = $2`, args: []interface{}{targetID, sourceID}},
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement.query, statement.args...); err != nil {
			return fmt.Errorf("move customer aggregate: %w", err)
		}
	}
	return nil
}

func storeCustomerIdempotencyResponse(ctx context.Context, tx *sql.Tx, operation, key, customerID string, response interface{}) error {
	encoded, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode customer idempotency response: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE customer_idempotency SET customer_id = $1, response = $2,
		updated_at = CURRENT_TIMESTAMP WHERE operation = $3 AND idempotency_key = $4`,
		customerID, encoded, operation, key); err != nil {
		return fmt.Errorf("store customer idempotency response: %w", err)
	}
	return nil
}

func (r *CustomerPostgresRepository) resolveCustomerForUpsert(ctx context.Context, tx *sql.Tx, workspaceID string, input domain.CustomerUpsertInput) (*domain.Customer, error) {
	var customer *domain.Customer
	var err error
	if input.Locator != nil {
		customer, err = r.findCustomerWithLock(ctx, tx, workspaceID, *input.Locator, true)
		if err != nil {
			return nil, err
		}
		if customer.MergedIntoID != nil {
			customer, err = r.findCustomerWithLock(ctx, tx, workspaceID, domain.CustomerLocator{CustomerID: *customer.MergedIntoID}, true)
			if err != nil {
				return nil, err
			}
		}
	}

	if input.ExternalUserID != nil {
		if customer == nil {
			candidate, findErr := r.findCustomerWithLock(ctx, tx, workspaceID, domain.CustomerLocator{ExternalUserID: *input.ExternalUserID}, true)
			switch {
			case findErr == nil:
				customer = candidate
			case isCustomerNotFound(findErr):
			case findErr != nil:
				return nil, findErr
			}
		} else {
			var ownerID string
			err := tx.QueryRowContext(ctx, `SELECT id FROM customers WHERE external_user_id = $1 AND id <> $2 FOR UPDATE`, *input.ExternalUserID, customer.ID).Scan(&ownerID)
			switch {
			case err == nil:
				return nil, &domain.ErrCustomerExternalIDConflict{}
			case errors.Is(err, sql.ErrNoRows):
			case err != nil:
				return nil, fmt.Errorf("check external user ID ownership: %w", err)
			}
		}
	}

	for _, identityInput := range input.Identities {
		normalized, err := domain.NormalizeCustomerIdentity(identityInput)
		if err != nil {
			return nil, err
		}
		fingerprint, err := domain.CustomerIdentityFingerprintForWorkspace(r.secretKey, workspaceID, normalized)
		if err != nil {
			return nil, err
		}
		if customer == nil {
			candidate, findErr := r.findCustomerWithLock(ctx, tx, workspaceID, domain.CustomerLocator{Identity: &domain.CustomerIdentityLocator{Type: normalized.Type, Value: normalized.Value}}, true)
			switch {
			case findErr == nil:
				customer = candidate
			case isCustomerNotFound(findErr):
			case findErr != nil:
				return nil, findErr
			}
			continue
		}
		var ownerID string
		err = tx.QueryRowContext(ctx, `SELECT customer_id FROM customer_identities WHERE identity_type = $1 AND lookup_fingerprint = $2 FOR UPDATE`, normalized.Type, fingerprint).Scan(&ownerID)
		switch {
		case err == nil && ownerID != customer.ID:
			return nil, &domain.ErrCustomerIdentityConflict{IdentityType: normalized.Type}
		case err == nil:
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return nil, fmt.Errorf("check %s identity ownership: %w", normalized.Type, err)
		}
	}
	// Without an explicit locator, external_user_id and identities are all
	// discovery keys for the same aggregate. If the external ID was not found
	// but an identity resolved a Customer that already has a different external
	// ID, silently replacing it would conflate two business users. Callers that
	// intentionally rename an external ID must identify the Customer explicitly.
	if input.Locator == nil && input.ExternalUserID != nil && customer != nil &&
		customer.ExternalUserID != nil && *customer.ExternalUserID != *input.ExternalUserID {
		return nil, &domain.ErrCustomerExternalIDConflict{}
	}
	return customer, nil
}

func isCustomerNotFound(err error) bool {
	var notFound *domain.ErrCustomerNotFound
	return errors.As(err, &notFound)
}

type customerProfileProjection struct {
	Language *string
	Timezone *string
}

func upsertCustomerProfile(ctx context.Context, tx *sql.Tx, customerID string, patch *domain.CustomerProfilePatch, now time.Time) (*customerProfileProjection, error) {
	if patch == nil {
		return nil, nil
	}
	var status, language, timezone sql.NullString
	var attributesJSON []byte
	var version int64
	err := tx.QueryRowContext(ctx, `SELECT status, language, timezone, attributes, version FROM customer_profiles WHERE customer_id = $1 FOR UPDATE`, customerID).
		Scan(&status, &language, &timezone, &attributesJSON, &version)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("lock customer profile: %w", err)
	}
	attributes := map[string]interface{}{}
	if err == nil && len(attributesJSON) > 0 {
		if err := json.Unmarshal(attributesJSON, &attributes); err != nil {
			return nil, fmt.Errorf("decode current customer attributes: %w", err)
		}
	}
	attributes, err = domain.ApplyCustomerAttributesPatch(attributes, patch.Attributes)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(attributes)
	if err != nil {
		return nil, fmt.Errorf("encode customer attributes: %w", err)
	}
	statusValue := nullableStringOrExisting(patch.Status, status)
	languageValue := nullableStringOrExisting(patch.Language, language)
	timezoneValue := nullableStringOrExisting(patch.Timezone, timezone)
	nextVersion := version + 1
	if _, err := tx.ExecContext(ctx, `INSERT INTO customer_profiles (
		customer_id, status, language, timezone, attributes, version, created_at, updated_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
	ON CONFLICT (customer_id) DO UPDATE SET status = EXCLUDED.status, language = EXCLUDED.language,
		timezone = EXCLUDED.timezone, attributes = EXCLUDED.attributes, version = EXCLUDED.version, updated_at = EXCLUDED.updated_at`,
		customerID, statusValue, languageValue, timezoneValue, encoded, nextVersion, now); err != nil {
		return nil, fmt.Errorf("upsert customer profile: %w", err)
	}
	return &customerProfileProjection{Language: stringPointer(languageValue), Timezone: stringPointer(timezoneValue)}, nil
}

func nullableStringOrExisting(patch *string, current sql.NullString) interface{} {
	if patch != nil {
		return *patch
	}
	if current.Valid {
		return current.String
	}
	return nil
}

func stringPointer(value interface{}) *string {
	text, ok := value.(string)
	if !ok {
		return nil
	}
	return &text
}

func (r *CustomerPostgresRepository) upsertCustomerIdentities(ctx context.Context, tx *sql.Tx, workspaceID, customerID string, identities []domain.CustomerIdentityInput, now time.Time) error {
	for _, input := range identities {
		normalized, err := domain.NormalizeCustomerIdentity(input)
		if err != nil {
			return err
		}
		fingerprint, err := domain.CustomerIdentityFingerprintForWorkspace(r.secretKey, workspaceID, normalized)
		if err != nil {
			return err
		}
		ciphertext, err := pkgcrypto.EncryptString(normalized.Value, r.secretKey)
		if err != nil {
			return fmt.Errorf("encrypt customer identity: %w", err)
		}
		metadata, err := json.Marshal(input.Metadata)
		if err != nil {
			return fmt.Errorf("encode customer identity metadata: %w", err)
		}
		if input.Metadata == nil {
			metadata = []byte("{}")
		}
		if input.Primary {
			if _, err := tx.ExecContext(ctx, `UPDATE customer_identities SET is_primary = FALSE, updated_at = CURRENT_TIMESTAMP WHERE customer_id = $1 AND identity_type = $2 AND is_primary`, customerID, normalized.Type); err != nil {
				return fmt.Errorf("demote primary customer identity: %w", err)
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO customer_identities (
			id, customer_id, identity_type, value_ciphertext, lookup_fingerprint, display_hint,
			verified, is_primary, metadata, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
		ON CONFLICT (identity_type, lookup_fingerprint) DO UPDATE SET
			verified = customer_identities.verified OR EXCLUDED.verified,
			is_primary = EXCLUDED.is_primary, metadata = customer_identities.metadata || EXCLUDED.metadata,
			enabled = TRUE, updated_at = EXCLUDED.updated_at
		WHERE customer_identities.customer_id = EXCLUDED.customer_id`,
			uuid.New(), customerID, normalized.Type, ciphertext, fingerprint, normalized.DisplayHint,
			input.Verified, input.Primary, metadata, now)
		if err != nil {
			return mapCustomerMutationError(err, normalized.Type)
		}
	}
	return nil
}

func replaceCustomerTags(ctx context.Context, tx *sql.Tx, customerID string, tags *[]string, now time.Time) error {
	if tags == nil {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM customer_tags WHERE customer_id = $1`, customerID); err != nil {
		return fmt.Errorf("replace customer tags: %w", err)
	}
	for _, tag := range *tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO customer_tags (customer_id, tag, created_at) VALUES ($1, $2, $3)`, customerID, tag, now); err != nil {
			return fmt.Errorf("insert customer tag: %w", err)
		}
	}
	return nil
}

func replaceCustomerListMemberships(ctx context.Context, tx *sql.Tx, customerID string, memberships []domain.CustomerListMembershipInput, now time.Time) error {
	if memberships == nil {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM customer_list_memberships WHERE customer_id = $1`, customerID); err != nil {
		return fmt.Errorf("replace customer list memberships: %w", err)
	}
	for _, membership := range memberships {
		if _, err := tx.ExecContext(ctx, `INSERT INTO customer_list_memberships (customer_id, list_id, status, created_at, updated_at) VALUES ($1, $2, $3, $4, $4)`,
			customerID, membership.ListID, membership.Status, now); err != nil {
			return fmt.Errorf("insert customer list membership: %w", err)
		}
	}
	return nil
}

func projectCustomerContact(ctx context.Context, tx *sql.Tx, customer *domain.Customer, profile *customerProfileProjection, identities []domain.CustomerIdentityInput, now time.Time) error {
	var email string
	for _, identity := range identities {
		if identity.Type == domain.CustomerIdentityEmail && (email == "" || identity.Primary) {
			email = identity.Value
			if identity.Primary {
				break
			}
		}
	}
	if email == "" {
		return nil
	}
	var language, timezone interface{}
	if profile != nil {
		language = profile.Language
		timezone = profile.Timezone
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO contacts (
		email, external_id, timezone, language, customer_id, created_at, updated_at
	) VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (email) DO UPDATE SET external_id = COALESCE(EXCLUDED.external_id, contacts.external_id),
		timezone = COALESCE(EXCLUDED.timezone, contacts.timezone), language = COALESCE(EXCLUDED.language, contacts.language),
		customer_id = EXCLUDED.customer_id, updated_at = EXCLUDED.updated_at`,
		email, customer.ExternalUserID, timezone, language, customer.ID, now, now)
	if err != nil {
		return fmt.Errorf("project customer contact: %w", err)
	}
	return nil
}

func mapCustomerMutationError(err error, identityType domain.CustomerIdentityType) error {
	var postgresError *pq.Error
	if !errors.As(err, &postgresError) || postgresError.Code != "23505" {
		return err
	}
	switch postgresError.Constraint {
	case "uq_customers_external_user_id":
		return &domain.ErrCustomerExternalIDConflict{}
	case "uq_customer_identities_lookup":
		return &domain.ErrCustomerIdentityConflict{IdentityType: identityType}
	case "uq_customers_customer_no":
		return &domain.ErrCustomerNumberConflict{}
	default:
		return &domain.ErrCustomerConflict{Constraint: postgresError.Constraint}
	}
}

func (r *CustomerPostgresRepository) loadCustomerChildren(ctx context.Context, db customerQueryer, customer *domain.Customer) error {
	if err := loadCustomerProfile(ctx, db, customer); err != nil {
		return err
	}
	if err := loadCustomerIdentities(ctx, db, customer); err != nil {
		return err
	}
	if err := loadCustomerTags(ctx, db, customer); err != nil {
		return err
	}
	if err := loadCustomerListMemberships(ctx, db, customer); err != nil {
		return err
	}
	return nil
}

func loadCustomerProfile(ctx context.Context, db customerQueryer, customer *domain.Customer) error {
	row := db.QueryRowContext(ctx, `SELECT status, language, timezone, attributes, version, created_at, updated_at FROM customer_profiles WHERE customer_id = $1`, customer.ID)
	var status, language, timezone sql.NullString
	var attributes []byte
	profile := &domain.CustomerProfile{}
	if err := row.Scan(&status, &language, &timezone, &attributes, &profile.Version, &profile.CreatedAt, &profile.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("query customer profile: %w", err)
	}
	if status.Valid {
		profile.Status = &status.String
	}
	if language.Valid {
		profile.Language = &language.String
	}
	if timezone.Valid {
		profile.Timezone = &timezone.String
	}
	if len(attributes) > 0 {
		if err := json.Unmarshal(attributes, &profile.Attributes); err != nil {
			return fmt.Errorf("decode customer profile attributes: %w", err)
		}
	}
	if profile.Attributes == nil {
		profile.Attributes = map[string]interface{}{}
	}
	customer.Profile = profile
	return nil
}

func loadCustomerIdentities(ctx context.Context, db customerQueryer, customer *domain.Customer) error {
	rows, err := db.QueryContext(ctx, `SELECT id, identity_type, display_hint, verified, is_primary, enabled, metadata, created_at, updated_at FROM customer_identities WHERE customer_id = $1 ORDER BY identity_type, is_primary DESC, id`, customer.ID)
	if err != nil {
		return fmt.Errorf("query customer identities: %w", err)
	}
	defer rows.Close()
	customer.Identities = []domain.CustomerIdentity{}
	for rows.Next() {
		var identity domain.CustomerIdentity
		var identityType string
		var metadata []byte
		if err := rows.Scan(&identity.ID, &identityType, &identity.DisplayHint, &identity.Verified, &identity.Primary, &identity.Enabled, &metadata, &identity.CreatedAt, &identity.UpdatedAt); err != nil {
			return fmt.Errorf("scan customer identity: %w", err)
		}
		identity.Type = domain.CustomerIdentityType(identityType)
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &identity.Metadata); err != nil {
				return fmt.Errorf("decode customer identity metadata: %w", err)
			}
		}
		customer.Identities = append(customer.Identities, identity)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate customer identities: %w", err)
	}
	return nil
}

func loadCustomerTags(ctx context.Context, db customerQueryer, customer *domain.Customer) error {
	rows, err := db.QueryContext(ctx, `SELECT tag FROM customer_tags WHERE customer_id = $1 ORDER BY tag`, customer.ID)
	if err != nil {
		return fmt.Errorf("query customer tags: %w", err)
	}
	defer rows.Close()
	customer.Tags = []string{}
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return fmt.Errorf("scan customer tag: %w", err)
		}
		customer.Tags = append(customer.Tags, tag)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate customer tags: %w", err)
	}
	return nil
}

func loadCustomerListMemberships(ctx context.Context, db customerQueryer, customer *domain.Customer) error {
	rows, err := db.QueryContext(ctx, `SELECT list_id, status, created_at, updated_at FROM customer_list_memberships WHERE customer_id = $1 ORDER BY list_id`, customer.ID)
	if err != nil {
		return fmt.Errorf("query customer list memberships: %w", err)
	}
	defer rows.Close()
	customer.ListMemberships = []domain.CustomerListMembership{}
	for rows.Next() {
		var membership domain.CustomerListMembership
		if err := rows.Scan(&membership.ListID, &membership.Status, &membership.CreatedAt, &membership.UpdatedAt); err != nil {
			return fmt.Errorf("scan customer list membership: %w", err)
		}
		customer.ListMemberships = append(customer.ListMemberships, membership)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate customer list memberships: %w", err)
	}
	return nil
}
