package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/config"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

// V51Migration regenerates every public customer number with the numeric-first
// three-character workspace identifier introduced in version 51.
type V51Migration struct{}

func (m *V51Migration) GetMajorVersion() float64                                       { return 51.0 }
func (m *V51Migration) HasSystemUpdate() bool                                          { return false }
func (m *V51Migration) HasWorkspaceUpdate() bool                                       { return true }
func (m *V51Migration) ShouldRestartServer() bool                                      { return false }
func (m *V51Migration) UpdateSystem(context.Context, *config.Config, DBExecutor) error { return nil }

const (
	v51SelectFirstCustomerPageSQL = `SELECT id, created_at FROM customers ORDER BY created_at, id LIMIT $1`
	v51SelectNextCustomerPageSQL  = `SELECT id, created_at FROM customers
		WHERE (created_at, id) > ($1, $2) ORDER BY created_at, id LIMIT $3`
	v51CreateMappingTableSQL = `CREATE TEMP TABLE yaoguang_v51_customer_numbers (
		customer_id UUID PRIMARY KEY,
		customer_no VARCHAR(53) NOT NULL UNIQUE
	) ON COMMIT DROP`
	v51AssignTemporaryNumbersSQL = `UPDATE customers SET customer_no = '~' || REPLACE(id::text, '-', '')`
	v51ApplyCustomerNumbersSQL   = `UPDATE customers customer SET customer_no = mapping.customer_no
		FROM yaoguang_v51_customer_numbers mapping WHERE customer.id = mapping.customer_id`
	v51RefreshIdempotencyResponsesSQL = `UPDATE customer_idempotency idempotency SET
		response = CASE
			WHEN idempotency.operation = 'customer.upsert' THEN
				jsonb_set(idempotency.response, '{customer_no}', to_jsonb(mapping.customer_no), false)
			WHEN idempotency.operation = 'customer.merge' THEN
				jsonb_set(idempotency.response, '{target_customer_no}', to_jsonb(mapping.customer_no), false)
			ELSE idempotency.response
		END,
		updated_at = CURRENT_TIMESTAMP
		FROM yaoguang_v51_customer_numbers mapping
		WHERE idempotency.customer_id = mapping.customer_id
			AND idempotency.response IS NOT NULL
			AND ((idempotency.operation = 'customer.upsert' AND idempotency.response ? 'customer_no')
				OR (idempotency.operation = 'customer.merge' AND idempotency.response ? 'target_customer_no'))`
	v51CustomerNumberPrefixLength = 20
	v51CustomerPageSize           = 1000
)

type v51CustomerSeed struct {
	customerID uuid.UUID
	createdAt  time.Time
}

type v51CustomerNumberMapping struct {
	customerID uuid.UUID
	customerNo string
}

func (m *V51Migration) UpdateWorkspace(ctx context.Context, _ *config.Config, workspace *domain.Workspace, db DBExecutor) error {
	if workspace == nil || workspace.Sequence < domain.MinWorkspaceSequence || workspace.Sequence > domain.MaxWorkspaceSequence {
		return fmt.Errorf("v51: workspace sequence must be between %d and %d", domain.MinWorkspaceSequence, domain.MaxWorkspaceSequence)
	}
	if db == nil {
		return fmt.Errorf("v51: workspace database is required")
	}

	if _, err := db.ExecContext(ctx, v51CreateMappingTableSQL); err != nil {
		return fmt.Errorf("v51: create customer number mapping for workspace %s: %w", workspace.ID, err)
	}
	customerCount, err := stageV51CustomerNumbers(ctx, workspace.Sequence, db, v51CustomerPageSize)
	if err != nil {
		return fmt.Errorf("v51: stage customer numbers for workspace %s: %w", workspace.ID, err)
	}
	if customerCount == 0 {
		return nil
	}
	if err := execV51CustomerNumberUpdate(ctx, db, v51AssignTemporaryNumbersSQL, customerCount); err != nil {
		return fmt.Errorf("v51: assign temporary customer numbers for workspace %s: %w", workspace.ID, err)
	}
	if err := execV51CustomerNumberUpdate(ctx, db, v51ApplyCustomerNumbersSQL, customerCount); err != nil {
		return fmt.Errorf("v51: apply customer numbers for workspace %s: %w", workspace.ID, err)
	}
	if _, err := db.ExecContext(ctx, v51RefreshIdempotencyResponsesSQL); err != nil {
		return fmt.Errorf("v51: refresh customer idempotency responses for workspace %s: %w", workspace.ID, err)
	}
	return nil
}

func stageV51CustomerNumbers(ctx context.Context, workspaceSequence uint16, db DBExecutor, pageSize int) (int, error) {
	if pageSize <= 0 {
		return 0, fmt.Errorf("customer page size must be positive")
	}
	var cursor *v51CustomerSeed
	total := 0
	currentPrefix := ""
	usedNumbers := make(map[string]struct{})
	for {
		seeds, err := readV51CustomerPage(ctx, db, cursor, pageSize)
		if err != nil {
			return 0, err
		}
		if len(seeds) == 0 {
			return total, nil
		}

		mappings := make([]v51CustomerNumberMapping, 0, len(seeds))
		for _, seed := range seeds {
			primary, err := domain.GenerateCustomerNumber(workspaceSequence, seed.createdAt, seed.customerID)
			if err != nil {
				return 0, err
			}
			prefix := primary[:v51CustomerNumberPrefixLength]
			if prefix != currentPrefix {
				currentPrefix = prefix
				clear(usedNumbers)
			}

			customerNo := ""
			for suffixOffset := uint64(0); suffixOffset <= uint64(len(usedNumbers)); suffixOffset++ {
				candidate, err := domain.GenerateCustomerNumberWithSuffixOffset(workspaceSequence, seed.createdAt, seed.customerID, suffixOffset)
				if err != nil {
					return 0, err
				}
				if _, exists := usedNumbers[candidate]; !exists {
					customerNo = candidate
					break
				}
			}
			if customerNo == "" {
				return 0, fmt.Errorf("customer number suffix space exhausted at %s", currentPrefix)
			}
			usedNumbers[customerNo] = struct{}{}
			mappings = append(mappings, v51CustomerNumberMapping{customerID: seed.customerID, customerNo: customerNo})
		}
		if err := insertV51CustomerNumberMappings(ctx, db, mappings); err != nil {
			return 0, err
		}
		total += len(mappings)
		last := seeds[len(seeds)-1]
		cursor = &last
		if len(seeds) < pageSize {
			return total, nil
		}
	}
}

func readV51CustomerPage(ctx context.Context, db DBExecutor, cursor *v51CustomerSeed, pageSize int) ([]v51CustomerSeed, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if cursor == nil {
		rows, err = db.QueryContext(ctx, v51SelectFirstCustomerPageSQL, pageSize)
	} else {
		rows, err = db.QueryContext(ctx, v51SelectNextCustomerPageSQL, cursor.createdAt, cursor.customerID.String(), pageSize)
	}
	if err != nil {
		return nil, err
	}

	seeds := make([]v51CustomerSeed, 0, pageSize)
	for rows.Next() {
		var seed v51CustomerSeed
		if err := rows.Scan(&seed.customerID, &seed.createdAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		seeds = append(seeds, seed)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return seeds, nil
}

func insertV51CustomerNumberMappings(ctx context.Context, db DBExecutor, mappings []v51CustomerNumberMapping) error {
	var query strings.Builder
	query.WriteString("INSERT INTO yaoguang_v51_customer_numbers (customer_id, customer_no) VALUES ")
	args := make([]interface{}, 0, len(mappings)*2)
	for index, mapping := range mappings {
		if index > 0 {
			query.WriteString(", ")
		}
		argument := index*2 + 1
		fmt.Fprintf(&query, "($%d, $%d)", argument, argument+1)
		args = append(args, mapping.customerID.String(), mapping.customerNo)
	}
	result, err := db.ExecContext(ctx, query.String(), args...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != int64(len(mappings)) {
		return fmt.Errorf("staged %d of %d customer numbers", affected, len(mappings))
	}
	return nil
}

func execV51CustomerNumberUpdate(ctx context.Context, db DBExecutor, query string, expected int) error {
	result, err := db.ExecContext(ctx, query)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != int64(expected) {
		return fmt.Errorf("updated %d of %d customers", affected, expected)
	}
	return nil
}

func init() { Register(&V51Migration{}) }
