package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/hengshu-credit/yaoguang-marketing/internal/domain"
	"time"
)

func (r *CampaignPostgresRepository) CreateCurrentCampaign(ctx context.Context, ws string, campaign domain.Campaign, config domain.CampaignConfiguration) error {
	if err := config.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(config)
	if err != nil {
		return err
	}
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO campaigns(id,name,status,current_configuration,created_at,updated_at) VALUES($1,$2,'draft',$3,$4,$4)`, campaign.ID, campaign.Name, body, campaign.CreatedAt)
	return err
}
func (r *CampaignPostgresRepository) SaveCurrentCampaignConfiguration(ctx context.Context, ws, id, name string, config domain.CampaignConfiguration) error {
	if err := config.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(config)
	if err != nil {
		return err
	}
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `UPDATE campaigns SET name=$2,current_configuration=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, id, name, body)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}
func (r *CampaignPostgresRepository) GetCurrentCampaignConfiguration(ctx context.Context, ws, id string) (*domain.CampaignConfiguration, error) {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return nil, err
	}
	var body []byte
	err = db.QueryRowContext(ctx, `SELECT COALESCE(c.current_configuration,to_jsonb(v)) FROM campaigns c LEFT JOIN campaign_versions v ON v.campaign_id=c.id AND v.version=c.draft_version WHERE c.id=$1`, id).Scan(&body)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, errors.New("campaign has no current configuration")
	}
	var config domain.CampaignConfiguration
	err = json.Unmarshal(body, &config)
	return &config, err
}
func (r *CampaignPostgresRepository) EnsureBroadcastCurrentCampaign(ctx context.Context, ws string, broadcast *domain.Broadcast, config domain.CampaignConfiguration) (string, error) {
	if err := config.Validate(); err != nil {
		return "", err
	}
	body, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	err = db.QueryRowContext(ctx, `INSERT INTO campaigns(id,name,status,source_type,source_id,current_configuration) VALUES($1,$2,'draft','broadcast',$3,$4)
 ON CONFLICT(source_type,source_id) WHERE source_type IS NOT NULL AND source_id IS NOT NULL DO UPDATE SET name=EXCLUDED.name,current_configuration=EXCLUDED.current_configuration,updated_at=CURRENT_TIMESTAMP RETURNING id`, id, broadcast.Name, broadcast.ID, body).Scan(&id)
	return id, err
}
func (r *CampaignPostgresRepository) CreateCampaignRunWithSnapshot(ctx context.Context, ws string, run domain.CampaignRun, snapshot *domain.ExecutionSnapshot) error {
	db, err := r.getDB(ctx, ws)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Freeze dependency definitions in the same transaction as the run binding.
	dependencies := map[string]any{}
	var definitions json.RawMessage
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(d)),'[]'::jsonb) FROM business_definitions d WHERE status='active'`).Scan(&definitions)
	if err != nil {
		return err
	}
	dependencies["business_definitions"] = definitions
	if run.AudienceID != "" {
		var definition json.RawMessage
		err = tx.QueryRowContext(ctx, `SELECT definition FROM audience_versions WHERE audience_id=$1 AND version=$2`, run.AudienceID, run.AudienceVersion).Scan(&definition)
		if err != nil {
			return err
		}
		dependencies["audience"] = definition
	}
	var configuration domain.CampaignConfiguration
	if err = json.Unmarshal(run.ExecutionConfiguration, &configuration); err != nil {
		return err
	}
	if configuration.ListID != "" {
		var list json.RawMessage
		err = tx.QueryRowContext(ctx, `SELECT to_jsonb(l) FROM lists l WHERE id=$1`, configuration.ListID).Scan(&list)
		if err != nil {
			return err
		}
		dependencies["list"] = list
	}
	snapshot.Dependencies, err = domain.SanitizeExecutionJSON(dependencies)
	if err != nil {
		return err
	}
	if err = saveExecutionSnapshot(ctx, tx, snapshot); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO campaign_runs(id,campaign_id,campaign_version,audience_id,audience_version,audience_build_id,status,run_seed,snapshot_count,next_ordinal,execution_configuration,created_at,updated_at)
 VALUES($1,$2,NULL,NULLIF($3,'')::uuid,NULLIF($4,0),NULLIF($5,'')::uuid,'snapshotting',$6,0,1,$7,$8,$8)`, run.ID, run.CampaignID, run.AudienceID, run.AudienceVersion, run.AudienceBuildID, run.RunSeed, []byte(run.ExecutionConfiguration), run.CreatedAt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE campaigns SET status='scheduled',updated_at=$2 WHERE id=$1`, run.CampaignID, time.Now().UTC())
	if err != nil {
		return err
	}
	return tx.Commit()
}
