package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type usageCostEvidenceRepository struct {
	db *sql.DB
}

func NewUsageCostEvidenceRepository(db *sql.DB) service.UsageCostEvidenceRepository {
	return &usageCostEvidenceRepository{db: db}
}

func (r *usageCostEvidenceRepository) CreateOnce(ctx context.Context, evidence *service.UsageCostEvidence) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("usage cost evidence repository unavailable")
	}
	if evidence == nil {
		return false, nil
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO usage_upstream_cost_evidence (
			usage_log_id, source, upstream_request_id, sub_actual_cost,
			newapi_quota, newapi_quota_per_unit, normalized_cost_cny,
			profit_cny, evidence_status, reason_code, recorded_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''), $11)
		ON CONFLICT (usage_log_id) DO NOTHING
		RETURNING id
	`, evidence.UsageLogID, evidence.Source, evidence.UpstreamRequestID, evidence.SubActualCost,
		evidence.NewAPIQuota, evidence.NewAPIQuotaPerUnit, evidence.NormalizedCostCNY,
		evidence.ProfitCNY, evidence.Status, evidence.ReasonCode, evidence.RecordedAt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
