package repository

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Update only this judgment. Generation, cost, response and other metadata stay intact.
func (r *pelicanGroupTestRepository) CompleteDrawingJudgment(ctx context.Context, id int64, j *service.DrawingJudgment) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE pelican_group_test_results SET pelican_config=jsonb_set(pelican_config,'{intelligence_result,drawing_judgment}',$2::jsonb) WHERE id=$1 AND pelican_config->'intelligence_result'->'drawing_judgment'->>'status'='judging'`, id, string(data))
	return err
}

// Claim is durable and never re-opened: crash recovery must not spend twice.
func (r *pelicanGroupTestRepository) ClaimDrawingJudgment(ctx context.Context, id int64) (bool, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE pelican_group_test_results SET pelican_config=jsonb_set(pelican_config,'{intelligence_result,drawing_judgment,status}','"judging"'::jsonb) WHERE id=$1 AND pelican_config->'intelligence_result'->'drawing_judgment'->>'status'='pending' AND (pelican_config->'intelligence_result'->'drawing_judgment'->>'deadline')::timestamptz > NOW()`, id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}
