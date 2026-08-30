package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"github.com/lib/pq"
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

func (r *CampaignPostgresRepository) EnsureBroadcastCampaign(ctx context.Context, workspaceID, broadcastID, name string,
	audienceID string, audienceVersion int, channel string, variants []domain.CampaignVariant) (*domain.CampaignVersion, error) {
	now := time.Now().UTC()
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	campaignID := ""
	versionNumber := 1
	err = tx.QueryRowContext(ctx, `SELECT id, draft_version + 1 FROM campaigns
		WHERE source_type = 'broadcast' AND source_id = $1 FOR UPDATE`, broadcastID).Scan(&campaignID, &versionNumber)
	if errors.Is(err, sql.ErrNoRows) {
		campaignID = uuid.NewString()
		if _, err := tx.ExecContext(ctx, `INSERT INTO campaigns (
			id, name, status, draft_version, source_type, source_id, created_at, updated_at
		) VALUES ($1, $2, 'draft', 1, 'broadcast', $3, $4, $4)`, campaignID, name, broadcastID, now); err != nil {
			return nil, err
		}
		versionNumber = 1
	} else if err != nil {
		return nil, err
	}
	version := &domain.CampaignVersion{CampaignID: campaignID, Version: versionNumber,
		AudienceID: audienceID, AudienceVersion: audienceVersion, Channel: channel, Variants: variants, CreatedAt: now}
	if err := version.Validate(); err != nil {
		return nil, err
	}
	variantJSON, err := json.Marshal(variants)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO campaign_versions (
		campaign_id, version, audience_id, audience_version, channel, variants, config, created_at
	) VALUES ($1, $2, $3, $4, $5, $6, jsonb_build_object('broadcast_id', $7::text), $8)`,
		campaignID, versionNumber, audienceID, audienceVersion, channel, variantJSON, broadcastID, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE campaigns SET name = $2, draft_version = $3,
		updated_at = $4 WHERE id = $1`, campaignID, name, versionNumber, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return version, nil
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

func (r *CampaignPostgresRepository) GetCampaign(ctx context.Context, workspaceID, campaignID string) (*domain.Campaign, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	item := &domain.Campaign{}
	var activeVersion sql.NullInt64
	err = db.QueryRowContext(ctx, `SELECT id, name, status, draft_version, active_version, created_at, updated_at
		FROM campaigns WHERE id = NULLIF($1, '')::uuid`, campaignID).
		Scan(&item.ID, &item.Name, &item.Status, &item.DraftVersion, &activeVersion, &item.CreatedAt, &item.UpdatedAt)
	item.ActiveVersion = int(activeVersion.Int64)
	return item, err
}

func (r *CampaignPostgresRepository) ListCampaigns(ctx context.Context, workspaceID string, limit, offset int) ([]domain.Campaign, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaigns`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT id, name, status, draft_version, active_version, created_at, updated_at
		FROM campaigns ORDER BY updated_at DESC, id DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.Campaign, 0)
	for rows.Next() {
		var item domain.Campaign
		var activeVersion sql.NullInt64
		if err := rows.Scan(&item.ID, &item.Name, &item.Status, &item.DraftVersion, &activeVersion, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		item.ActiveVersion = int(activeVersion.Int64)
		items = append(items, item)
	}
	return items, total, rows.Err()
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

func (r *CampaignPostgresRepository) GetCampaignRun(ctx context.Context, workspaceID, runID string) (*domain.CampaignRun, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	item := &domain.CampaignRun{}
	var lastCustomerID sql.NullString
	err = db.QueryRowContext(ctx, `SELECT id, campaign_id, campaign_version, status, run_seed,
		snapshot_last_customer_id, snapshot_count, next_ordinal, created_at FROM campaign_runs WHERE id = NULLIF($1, '')::uuid`, runID).
		Scan(&item.ID, &item.CampaignID, &item.CampaignVersion, &item.Status, &item.RunSeed,
			&lastCustomerID, &item.SnapshotCount, &item.NextOrdinal, &item.CreatedAt)
	item.SnapshotLastCustomerID = lastCustomerID.String
	return item, err
}

func (r *CampaignPostgresRepository) ListAudienceMembers(ctx context.Context, workspaceID, audienceID string, version int, after string, limit int) ([]domain.CampaignAudienceMember, string, error) {
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, "", err
	}
	rows, err := db.QueryContext(ctx, `WITH source_build AS (
		SELECT id FROM audience_builds WHERE audience_id = NULLIF($1, '')::uuid
			AND audience_version = $2 AND status = 'completed'
		ORDER BY completed_at DESC, id DESC LIMIT 1
	) SELECT membership.customer_id, membership.build_id FROM audience_memberships membership
		JOIN source_build ON source_build.id = membership.build_id
		WHERE 1 = 1
			AND membership.customer_id > COALESCE(NULLIF($3, '')::uuid, '00000000-0000-0000-0000-000000000000'::uuid)
		ORDER BY membership.customer_id LIMIT $4`, audienceID, version, after, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	members := []domain.CampaignAudienceMember{}
	for rows.Next() {
		var member domain.CampaignAudienceMember
		if err := rows.Scan(&member.CustomerID, &member.BuildID); err != nil {
			return nil, "", err
		}
		members = append(members, member)
	}
	next := ""
	if len(members) > 0 {
		next = members[len(members)-1].CustomerID
	}
	return members, next, rows.Err()
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
	ordinals := make([]int64, len(snapshots))
	customerIDs := make([]string, len(snapshots))
	variants := make([]string, len(snapshots))
	buildIDs := make([]string, len(snapshots))
	createdAt := make([]time.Time, len(snapshots))
	for index, snapshot := range snapshots {
		ordinals[index], customerIDs[index], variants[index], buildIDs[index], createdAt[index] =
			snapshot.Ordinal, snapshot.CustomerID, snapshot.Variant, snapshot.SourceBuildID, snapshot.CreatedAt
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO campaign_recipient_snapshots (
		run_id, ordinal, customer_id, variant, source_build_id, created_at
	) SELECT NULLIF($1, '')::uuid, item.ordinal, item.customer_id, item.variant,
		NULLIF(item.build_id, '')::uuid, item.created_at FROM unnest(
		$2::bigint[], $3::uuid[], $4::text[], $5::text[], $6::timestamptz[]
	) AS item(ordinal, customer_id, variant, build_id, created_at)
	ON CONFLICT DO NOTHING`, runID, pq.Array(ordinals), pq.Array(customerIDs), pq.Array(variants), pq.Array(buildIDs), pq.Array(createdAt))
	if err != nil {
		return 0, err
	}
	inserted, _ := result.RowsAffected()
	if _, err := tx.ExecContext(ctx, `UPDATE campaign_runs SET snapshot_count = snapshot_count + $2,
		next_ordinal = GREATEST(next_ordinal, $3), snapshot_last_customer_id = NULLIF($4, '')::uuid,
		updated_at = CURRENT_TIMESTAMP WHERE id = NULLIF($1, '')::uuid`,
		runID, inserted, snapshots[len(snapshots)-1].Ordinal+1, snapshots[len(snapshots)-1].CustomerID); err != nil {
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

func (r *CampaignPostgresRepository) ListCampaignSnapshots(ctx context.Context, workspaceID, runID string, afterOrdinal int64, limit int) ([]domain.CampaignRecipientSnapshot, int64, error) {
	if limit <= 0 || limit > 5_000 {
		limit = 500
	}
	db, err := r.getDB(ctx, workspaceID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT run_id, ordinal, customer_id, variant, source_build_id, created_at
		FROM campaign_recipient_snapshots WHERE run_id = NULLIF($1, '')::uuid AND ordinal > $2
		ORDER BY ordinal LIMIT $3`, runID, afterOrdinal, limit+1)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]domain.CampaignRecipientSnapshot, 0, limit+1)
	for rows.Next() {
		var item domain.CampaignRecipientSnapshot
		var sourceBuildID sql.NullString
		if err := rows.Scan(&item.RunID, &item.Ordinal, &item.CustomerID, &item.Variant, &sourceBuildID, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		item.SourceBuildID = sourceBuildID.String
		items = append(items, item)
	}
	next := int64(0)
	if len(items) > limit {
		next = items[limit-1].Ordinal
		items = items[:limit]
	}
	return items, next, rows.Err()
}
