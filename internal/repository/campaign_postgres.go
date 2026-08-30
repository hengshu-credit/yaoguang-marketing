package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
)

type CampaignPostgresRepository struct {
	workspaceRepo domain.WorkspaceRepository
	db            *sql.DB
}

func NewCampaignRepository(workspaceRepo domain.WorkspaceRepository) *CampaignPostgresRepository {
	return &CampaignPostgresRepository{workspaceRepo: workspaceRepo}
}
func NewCampaignRepositoryWithDB(db *sql.DB) *CampaignPostgresRepository {
	return &CampaignPostgresRepository{db: db}
}
func (r *CampaignPostgresRepository) getDB(ctx context.Context, workspaceID string) (*sql.DB, error) {
	if r.db != nil {
		return r.db, nil
	}
	if r.workspaceRepo == nil {
		return nil, errors.New("workspace repository is required")
	}
	return r.workspaceRepo.GetConnection(ctx, workspaceID)
}

func (r *CampaignPostgresRepository) CreateCampaign(ctx context.Context, workspaceID string, campaign domain.Campaign, version domain.CampaignVersion) error {
	if err := version.Validate(); err != nil {
		return err
	}
	variants, _ := json.Marshal(version.Variants)
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaigns (id, name, status, draft_version, created_at, updated_at)
		VALUES (NULLIF($1, '')::uuid, $2, 'draft', $3, $4, $4)`, campaign.ID, campaign.Name, version.Version, campaign.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_versions (
		campaign_id, version, audience_id, audience_version, channel, variants, created_at
	) VALUES (NULLIF($1, '')::uuid, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7)`,
		version.CampaignID, version.Version, version.AudienceID, version.AudienceVersion, version.Channel, variants, version.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *CampaignPostgresRepository) GetCampaignVersion(ctx context.Context, workspaceID, campaignID string, version int) (*domain.CampaignVersion, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	item := &domain.CampaignVersion{}
	var variants []byte
	err = db.QueryRowContext(ctx, `SELECT campaign_id, version, audience_id, audience_version, channel, variants, activated_at, created_at
		FROM campaign_versions WHERE campaign_id = NULLIF($1, '')::uuid AND version = $2`, campaignID, version).
		Scan(&item.CampaignID, &item.Version, &item.AudienceID, &item.AudienceVersion, &item.Channel, &variants, &item.ActivatedAt, &item.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(variants, &item.Variants); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *CampaignPostgresRepository) CreateCampaignRun(ctx context.Context, workspaceID string, run domain.CampaignRun) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var activatedAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT activated_at FROM campaign_versions
		WHERE campaign_id = NULLIF($1, '')::uuid AND version = $2 FOR UPDATE`, run.CampaignID, run.CampaignVersion).Scan(&activatedAt); err != nil {
		return err
	}
	if !activatedAt.Valid {
		if _, err := tx.ExecContext(ctx, `UPDATE campaign_versions SET activated_at = $3
			WHERE campaign_id = NULLIF($1, '')::uuid AND version = $2`, run.CampaignID, run.CampaignVersion, run.CreatedAt); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE campaigns SET active_version = $2, status = 'scheduled', updated_at = $3
		WHERE id = NULLIF($1, '')::uuid AND (active_version IS NULL OR active_version = $2)`, run.CampaignID, run.CampaignVersion, run.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_runs (
		id, campaign_id, campaign_version, status, run_seed, snapshot_count, next_ordinal, created_at, updated_at
	) VALUES (NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, $3, 'snapshotting', $4, 0, 1, $5, $5)`,
		run.ID, run.CampaignID, run.CampaignVersion, run.RunSeed, run.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *CampaignPostgresRepository) ListAudienceMemberIDs(ctx context.Context, workspaceID, audienceID string, version int, after string, limit int) ([]string, string, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, "", err
	}
	rows, err := db.QueryContext(ctx, `SELECT membership.customer_id FROM audience_memberships membership
		JOIN audience_builds build ON build.id = membership.build_id
		JOIN audiences audience ON audience.active_build_id = build.id
		WHERE audience.id = NULLIF($1, '')::uuid AND build.audience_version = $2
			AND membership.customer_id > COALESCE(NULLIF($3, '')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
		ORDER BY membership.customer_id LIMIT $4`, audienceID, version, after, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, "", err
		}
		ids = append(ids, id)
	}
	next := ""
	if len(ids) > 0 {
		next = ids[len(ids)-1]
	}
	return ids, next, rows.Err()
}

func (r *CampaignPostgresRepository) AppendCampaignSnapshots(ctx context.Context, workspaceID, runID string, snapshots []domain.CampaignRecipientSnapshot) (int64, error) {
	if len(snapshots) == 0 {
		return 0, nil
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var inserted int64
	for _, snapshot := range snapshots {
		result, err := tx.ExecContext(ctx, `INSERT INTO campaign_recipient_snapshots (run_id, ordinal, customer_id, variant, source_build_id, created_at)
			VALUES (NULLIF($1, '')::uuid, $2, NULLIF($3, '')::uuid, $4, NULLIF($5, '')::uuid, $6)
			ON CONFLICT DO NOTHING`, runID, snapshot.Ordinal, snapshot.CustomerID, snapshot.Variant, snapshot.SourceBuildID, snapshot.CreatedAt)
		if err != nil {
			return 0, err
		}
		count, _ := result.RowsAffected()
		inserted += count
	}
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_runs SET snapshot_count = snapshot_count + $2,
		next_ordinal = GREATEST(next_ordinal, $3), updated_at = CURRENT_TIMESTAMP WHERE id = NULLIF($1, '')::uuid`,
		runID, inserted, snapshots[len(snapshots)-1].Ordinal+1); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

func (r *CampaignPostgresRepository) CompleteCampaignSnapshot(ctx context.Context, workspaceID, runID string, count int64) error {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `UPDATE campaign_runs SET status = 'dispatching', snapshot_count = $2,
		started_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = NULLIF($1, '')::uuid AND status = 'snapshotting'`, runID, count)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return fmt.Errorf("campaign snapshot completion rejected")
	}
	return nil
}
