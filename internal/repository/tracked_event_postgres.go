package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"sort"
	"strings"
	"time"
)

type TrackedEventPostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
	customers     *CustomerPostgresRepository
}

func NewTrackedEventRepository(w domain.WorkspaceRepository, customers *CustomerPostgresRepository) *TrackedEventPostgresRepository {
	return &TrackedEventPostgresRepository{workspaceRepo: w, customers: customers}
}
func NewTrackedEventRepositoryWithDB(db *sql.DB, secret string) *TrackedEventPostgresRepository {
	return &TrackedEventPostgresRepository{db: db, customers: &CustomerPostgresRepository{secretKey: secret, now: time.Now}}
}
func (r *TrackedEventPostgresRepository) getDB(ctx context.Context, w string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	return r.workspaceRepo.GetConnection(ctx, w)
}
func readTrackedReceipt(ctx context.Context, q customerQueryer, source, reportID string) (*domain.TrackedEventResult, string, error) {
	var payload []byte
	var hash string
	if err := q.QueryRowContext(ctx, `SELECT response,payload_hash FROM event_report_receipts WHERE source=$1 AND report_id=$2`, source, reportID).Scan(&payload, &hash); err != nil {
		return nil, "", err
	}
	var result domain.TrackedEventResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, "", err
	}
	result.Duplicate = true
	return &result, hash, nil
}
func (r *TrackedEventPostgresRepository) GetReceipt(ctx context.Context, w, source, reportID string) (*domain.TrackedEventResult, string, error) {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return nil, "", err
	}
	return readTrackedReceipt(ctx, db, source, reportID)
}
func (r *TrackedEventPostgresRepository) Ingest(ctx context.Context, c domain.TrackedEventCommand) (*domain.TrackedEventResult, error) {
	db, err := r.getDB(ctx, c.WorkspaceID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var erased bool
	session, _ := c.Event.CustomerAttributes["browser_session_id"].(string)
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM business_erased_reports WHERE report_hash=md5($1||':'||$2)) OR ($3<>'' AND EXISTS(SELECT 1 FROM business_erased_sessions WHERE session_hash=md5($1||':'||$3)))`, c.Source, c.Event.ReportID, session).Scan(&erased); err != nil {
		return nil, err
	}
	if erased {
		return nil, domain.ErrTrackedEventErased
	}
	eventID := uuid.New()
	c.Event.AttributePaths = map[string][]string{}
	c.Event.SensitiveAttributes = nil
	for key, definition := range c.Attributes {
		if definition.Settings["sensitive"] == true {
			c.Event.SensitiveAttributes = append(c.Event.SensitiveAttributes, key)
		}
	}
	sort.Strings(c.Event.SensitiveAttributes)
	if c.Mapping != nil {
		var rules []domain.AttributeMapping
		rawRules, _ := json.Marshal(c.Mapping.Settings["attribute_mappings"])
		if err = json.Unmarshal(rawRules, &rules); err != nil {
			return nil, domain.NewValidationError("invalid attribute mappings")
		}
		for _, rule := range rules {
			c.Event.AttributePaths[rule.Attribute] = append(c.Event.AttributePaths[rule.Attribute], rule.Path)
		}
	}
	normalized, _ := json.Marshal(c.Event)
	res, err := tx.ExecContext(ctx, `INSERT INTO event_report_receipts(source,report_id,event_id,payload_hash,reported_at,processing_status,normalized_event) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(source,report_id) DO NOTHING`, c.Source, c.Event.ReportID, eventID, c.PayloadHash, c.Event.ReportedAt, c.Event.ProcessingStatus, normalized)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		result, hash, err := readTrackedReceipt(ctx, tx, c.Source, c.Event.ReportID)
		if err != nil {
			return nil, err
		}
		if hash != c.PayloadHash {
			return nil, domain.ErrEventPayloadConflict
		}
		return result, tx.Commit()
	}
	applyTrackedEventCutoff(c.Event, time.Now().UTC())
	if objectType, _ := c.Definition.Settings["subject_type"].(string); objectType != "" && objectType != "customer" {
		c.ObjectDefinition, err = scanBusinessDefinition(tx.QueryRowContext(ctx, `SELECT `+businessDefinitionColumns+` FROM business_definitions WHERE kind='object_type' AND key=$1 AND status='active'`, objectType))
		if err != nil {
			return nil, err
		}
	}
	objectSnapshot, _ := json.Marshal(c.ObjectDefinition)
	if _, err = tx.ExecContext(ctx, `UPDATE event_report_receipts SET object_definition_snapshot=$3 WHERE source=$1 AND report_id=$2`, c.Source, c.Event.ReportID, objectSnapshot); err != nil {
		return nil, err
	}
	definitionSnapshot, _ := json.Marshal(c.Definition)
	mappingSnapshot, _ := json.Marshal(c.Mapping)
	attributeSnapshot, _ := json.Marshal(c.Attributes)
	if _, err = tx.ExecContext(ctx, `UPDATE event_report_receipts SET definition_snapshot=$3,mapping_snapshot=$4,attribute_definitions=$5 WHERE source=$1 AND report_id=$2`, c.Source, c.Event.ReportID, definitionSnapshot, mappingSnapshot, attributeSnapshot); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `SELECT set_config('notifuse.business_event_at',$1,true),set_config('notifuse.business_event_id',$2,true)`, c.Event.TriggeredAt.Format(time.RFC3339Nano), eventID.String()); err != nil {
		return nil, err
	}
	customerID := ""
	if !(strings.HasPrefix(c.Source, "browser:") && c.Event.CustomerAttributes["browser_session_id"] != nil && c.Event.CustomerAttributes["customer_id"] == nil) {
		customerID, err = r.resolveTrackedCustomer(ctx, tx, c)
	}
	if err != nil {
		return nil, err
	}
	if customerID == "" && !trackedEventIsHistorical(c.Event) {
		c.Event.ProcessingStatus = "pending_identity"
	}
	subject, err := resolveTrackedSubject(ctx, tx, c, customerID, eventID)
	if err != nil {
		return nil, err
	}
	normalized, _ = json.Marshal(c.Event)
	if customerID != "" {
		if err = markBusinessProjectionDirty(ctx, tx, "customer", customerID); err != nil {
			return nil, err
		}
		if subject.Type != "customer" {
			if err = markBusinessProjectionDirty(ctx, tx, subject.Type, subject.ID); err != nil {
				return nil, err
			}
		}
	}
	envelope := domain.EventEnvelope{EventID: eventID, Type: c.Event.EventCode, Subject: subject, Source: c.Source, OccurredAt: c.Event.TriggeredAt, Data: normalized}
	if _, err = AppendRealtimeEventTx(ctx, tx, c.WorkspaceID, envelope, c.Event.ReportedAt); err != nil {
		return nil, err
	}
	result := &domain.TrackedEventResult{ReportID: c.Event.ReportID, EventID: eventID.String(), CustomerID: customerID, ReportedAt: c.Event.ReportedAt, ProcessingStatus: c.Event.ProcessingStatus, Event: c.Event}
	response, _ := json.Marshal(result)
	if _, err = tx.ExecContext(ctx, `UPDATE event_report_receipts SET customer_id=NULLIF($3,'')::uuid,processing_status=$4,normalized_event=$5,response=$6 WHERE source=$1 AND report_id=$2`, c.Source, c.Event.ReportID, customerID, c.Event.ProcessingStatus, normalized, response); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
func (r *TrackedEventPostgresRepository) List(ctx context.Context, w, customerID string, limit int) ([]domain.TrackedEventResult, error) {
	db, err := r.getDB(ctx, w)
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `SELECT response FROM event_report_receipts WHERE ($1='' OR customer_id=NULLIF($1,'')::uuid) ORDER BY reported_at DESC LIMIT $2`, customerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TrackedEventResult{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var value domain.TrackedEventResult
		if err = json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}
func (r *TrackedEventPostgresRepository) resolveTrackedCustomer(ctx context.Context, tx *sql.Tx, c domain.TrackedEventCommand) (string, error) {
	// Sort lock keys so reports sharing identities serialize creation consistently.
	keys := []string{}
	locators := []domain.CustomerLocator{}
	bind := []domain.CustomerIdentityInput{}
	var externalID *string
	attrs := map[string]any{}
	for k, v := range c.Event.CustomerAttributes {
		attrs[k] = v
	}
	actions := map[string]string{}
	updates := map[string]any{}
	policies := map[string]string{}
	var mappings []domain.AttributeMapping
	if c.Mapping != nil {
		raw, _ := json.Marshal(c.Mapping.Settings["attribute_mappings"])
		if string(raw) != "null" {
			if err := json.Unmarshal(raw, &mappings); err != nil {
				return "", domain.NewValidationError("invalid attribute_mappings")
			}
		}
	}
	rawEvent := map[string]any{"data": c.Event.Data, "customer_attributes": c.Event.CustomerAttributes}
	for _, m := range mappings {
		v, exists := domain.BusinessJSONPath(rawEvent, m.Path)
		if !exists || v == nil && m.NullPolicy != "clear" {
			continue
		}
		switch m.Action {
		case "lookup", "bind":
			attrs[m.Attribute] = v
			actions[m.Attribute] = m.Action
		case "update":
			updates[m.Attribute] = v
			policies[m.Attribute] = m.UpdatePolicy
		case "record":
		default:
			return "", domain.NewValidationError("unknown attribute action")
		}
	}
	for key, value := range attrs {
		def, defined := c.Attributes[key]
		if text, ok := value.(string); ok && defined {
			switch def.Settings["normalization"] {
			case "lowercase":
				value = strings.ToLower(strings.TrimSpace(text))
			case "trim":
				value = strings.TrimSpace(text)
			case "phone":
				normalized, err := domain.NormalizeCustomerIdentity(domain.CustomerIdentityInput{Type: domain.CustomerIdentityPhone, Value: text})
				if err != nil {
					return "", domain.NewValidationError(err.Error())
				}
				value = normalized.Value
			}
		}
		if defined && value != nil {
			if err := domain.ValidateBusinessValue(def.Schema, value, "customer_attributes."+key); err != nil {
				return "", domain.NewValidationError(err.Error())
			}
		}
		identity := key == "external_user_id" || key == "customer_id" || key == "email" || key == "phone" || defined && def.Settings["identity"] == true
		if !identity {
			continue
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return "", domain.NewValidationError("identity attributes must be nonempty strings")
		}
		text = strings.TrimSpace(text)
		locator := domain.CustomerLocator{}
		switch key {
		case "customer_id":
			locator.CustomerID = text
		case "external_user_id":
			locator.ExternalUserID = text
			if actions[key] != "lookup" {
				copy := text
				externalID = &copy
			}
		default:
			typ := domain.CustomerIdentityType(key)
			if key != "email" && key != "phone" {
				typ = domain.CustomerIdentityCustom
				text = key + ":" + text
			}
			input := domain.CustomerIdentityInput{Type: typ, Value: text}
			normalized, err := domain.NormalizeCustomerIdentity(input)
			if err != nil {
				return "", domain.NewValidationError(err.Error())
			}
			locator.Identity = &domain.CustomerIdentityLocator{Type: typ, Value: normalized.Value}
			if actions[key] != "lookup" {
				bind = append(bind, input)
			}
		}
		if err := locator.Validate(); err != nil {
			return "", domain.NewValidationError(err.Error())
		}
		locators = append(locators, locator)
		bytes, _ := json.Marshal(locator)
		keys = append(keys, string(bytes))
	}
	if len(locators) == 0 {
		return "", domain.NewValidationError("no configured customer identity provided")
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); err != nil {
			return "", err
		}
	}
	customerID := ""
	for _, locator := range locators {
		customer, err := r.customers.findCustomer(ctx, tx, c.WorkspaceID, locator)
		var missing *domain.ErrCustomerNotFound
		if errors.As(err, &missing) {
			continue
		}
		if err != nil {
			return "", err
		}
		for depth := 0; customer.MergedIntoID != nil; depth++ {
			if depth > 32 {
				return "", fmt.Errorf("customer merge cycle")
			}
			customer, err = r.customers.findCustomer(ctx, tx, c.WorkspaceID, domain.CustomerLocator{CustomerID: *customer.MergedIntoID})
			if err != nil {
				return "", err
			}
		}
		if customerID != "" && customerID != customer.ID {
			return "", domain.NewValidationError("customer identity attributes resolve to different customers")
		}
		customerID = customer.ID
	}
	// Historical association may locate an existing Customer but cannot create an
	// authority, bind an identity/alias, or write a current profile.
	if trackedEventIsHistorical(c.Event) {
		return customerID, nil
	}
	if customerID == "" {
		mode, _ := c.Definition.Settings["unmatched_customer"].(string)
		switch mode {
		case "pending":
			return "", nil
		case "create":
			id := uuid.New()
			no, err := domain.GenerateCustomerNumber(c.WorkspaceSequence, c.Event.ReportedAt, id)
			if err != nil {
				return "", err
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO customers(id,customer_no,external_user_id) VALUES($1,$2,$3)`, id, no, externalID); err != nil {
				return "", err
			}
			customerID = id.String()
		default:
			return "", domain.NewValidationError("customer identity not found")
		}
	}
	if externalID != nil {
		result, err := tx.ExecContext(ctx, `INSERT INTO customer_external_id_aliases(external_user_id,customer_id) VALUES($1,$2) ON CONFLICT(external_user_id) DO UPDATE SET customer_id=EXCLUDED.customer_id WHERE customer_external_id_aliases.customer_id=EXCLUDED.customer_id`, *externalID, customerID)
		if err != nil {
			return "", err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return "", err
		}
		if n != 1 {
			return "", domain.NewValidationError("external user ID alias belongs to another customer")
		}
		var owner string
		err = tx.QueryRowContext(ctx, `SELECT id FROM customers WHERE external_user_id=$1`, *externalID).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) {
			if _, err = tx.ExecContext(ctx, `UPDATE customers SET external_user_id=COALESCE(external_user_id,$2),updated_at=NOW() WHERE id=$1`, customerID, *externalID); err != nil {
				return "", err
			}
		} else if err != nil {
			return "", err
		}
	}
	var lockedID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM customers WHERE id=$1 FOR UPDATE`, customerID).Scan(&lockedID); err != nil {
		return "", err
	}
	if err := r.customers.upsertCustomerIdentities(ctx, tx, c.WorkspaceID, customerID, bind, c.Event.ReportedAt); err != nil {
		return "", err
	}
	for key, value := range updates {
		def, ok := c.Attributes[key]
		if !ok || def.Settings["writable"] != true {
			return "", domain.NewValidationError("attribute is not writable: " + key)
		}
		if def.Settings["identity"] == true || key == "email" || key == "phone" || key == "external_user_id" {
			return "", domain.NewValidationError("identity attributes require bind action")
		}
		if value != nil {
			if err := domain.ValidateBusinessValue(def.Schema, value, "attributes."+key); err != nil {
				return "", domain.NewValidationError(err.Error())
			}
		}
		if policies[key] == "set_union" {
			incoming, ok := value.([]any)
			if !ok {
				return "", domain.NewValidationError("set_union requires an array")
			}
			var prior []byte
			err := tx.QueryRowContext(ctx, `SELECT value FROM customer_attribute_values WHERE customer_id=$1 AND attribute_key=$2 FOR UPDATE`, customerID, key).Scan(&prior)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return "", err
			}
			var old []any
			_ = json.Unmarshal(prior, &old)
			seen := map[string]bool{}
			combined := []any{}
			for _, item := range append(old, incoming...) {
				b, _ := json.Marshal(item)
				if !seen[string(b)] {
					combined = append(combined, item)
					seen[string(b)] = true
				}
			}
			value = combined
		} else if policies[key] != "" && policies[key] != "overwrite" {
			return "", domain.NewValidationError("invalid attribute update policy")
		}
		encoded, _ := json.Marshal(value)
		if _, err := tx.ExecContext(ctx, `INSERT INTO customer_attribute_values(customer_id,attribute_key,value,definition_revision) VALUES($1,$2,$3,$4) ON CONFLICT(customer_id,attribute_key) DO UPDATE SET value=EXCLUDED.value,definition_revision=EXCLUDED.definition_revision,updated_at=NOW()`, customerID, key, encoded, def.Revision); err != nil {
			return "", err
		}
		patch, _ := json.Marshal(map[string]any{key: value})
		if _, err := tx.ExecContext(ctx, `INSERT INTO customer_profiles(customer_id,attributes) VALUES($1,$2) ON CONFLICT(customer_id) DO UPDATE SET attributes=customer_profiles.attributes||EXCLUDED.attributes,version=customer_profiles.version+1,updated_at=NOW()`, customerID, patch); err != nil {
			return "", err
		}
	}
	return customerID, nil
}
