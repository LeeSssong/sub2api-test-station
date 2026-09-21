package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *groupRepository) ReadGroupToolMappings(ctx context.Context, ids []int64) (map[int64]service.GroupToolMapping, error) {
	result := map[int64]service.GroupToolMapping{}
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := r.sql.QueryContext(ctx, `SELECT group_id, tool_ids, version, effective_at, updated_by FROM group_tool_mappings WHERE group_id = ANY($1)`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m service.GroupToolMapping
		var raw []byte
		if err := rows.Scan(&m.GroupID, &raw, &m.Version, &m.EffectiveAt, &m.UpdatedBy); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &m.ToolIDs); err != nil {
			return nil, err
		}
		result[m.GroupID] = m
	}
	return result, rows.Err()
}
func (r *groupRepository) ReplaceGroupToolMapping(ctx context.Context, id int64, ids []string, actor int64) (service.GroupToolMapping, error) {
	normalized, err := service.NormalizeGroupToolIDs(ids)
	if err != nil {
		return service.GroupToolMapping{}, err
	}
	if actor <= 0 {
		return service.GroupToolMapping{}, fmt.Errorf("mapping actor required")
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return service.GroupToolMapping{}, err
	}
	// One statement atomically versions the mapping and records the exact audit row.
	rows, err := r.sql.QueryContext(ctx, `WITH changed AS (
 INSERT INTO group_tool_mappings(group_id,tool_ids,updated_by) SELECT id,$2::jsonb,$3 FROM groups WHERE id=$1 AND deleted_at IS NULL
 ON CONFLICT(group_id) DO UPDATE SET tool_ids=EXCLUDED.tool_ids,version=group_tool_mappings.version+1,effective_at=NOW(),updated_by=EXCLUDED.updated_by
 RETURNING group_id,tool_ids,version,effective_at,updated_by
 ), audit AS (INSERT INTO group_tool_mapping_audit(group_id,tool_ids,version,effective_at,updated_by) SELECT * FROM changed)
 SELECT group_id,tool_ids,version,effective_at,updated_by FROM changed`, id, string(raw), actor)
	if err != nil {
		return service.GroupToolMapping{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return service.GroupToolMapping{}, err
		}
		return service.GroupToolMapping{}, service.ErrGroupNotFound
	}
	var m service.GroupToolMapping
	var saved []byte
	if err := rows.Scan(&m.GroupID, &saved, &m.Version, &m.EffectiveAt, &m.UpdatedBy); err != nil {
		return m, err
	}
	err = json.Unmarshal(saved, &m.ToolIDs)
	return m, err
}
