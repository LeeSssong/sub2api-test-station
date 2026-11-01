package service

import (
	"context"
	"fmt"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"sort"
	"time"
)

type GroupToolMapping struct {
	GroupID     int64     `json:"group_id"`
	ToolIDs     []string  `json:"tool_ids"`
	Version     int64     `json:"version"`
	EffectiveAt time.Time `json:"effective_at"`
	UpdatedBy   int64     `json:"updated_by"`
}
type GroupToolMappingRepository interface {
	ReadGroupToolMappings(context.Context, []int64) (map[int64]GroupToolMapping, error)
	ReplaceGroupToolMapping(context.Context, int64, []string, int64) (GroupToolMapping, error)
}

// GetGroupToolMappings lets the native user group response reuse the same
// associations as the dashboard, after its caller has authorized the group IDs.
func (s *APIKeyService) GetGroupToolMappings(ctx context.Context, groupIDs []int64) (map[int64]GroupToolMapping, error) {
	repo, ok := s.groupRepo.(GroupToolMappingRepository)
	if !ok || len(groupIDs) == 0 {
		return nil, nil
	}
	mappings, err := repo.ReadGroupToolMappings(ctx, groupIDs)
	if err != nil {
		return nil, fmt.Errorf("get group tool mappings: %w", err)
	}
	return mappings, nil
}

func NormalizeGroupToolIDs(ids []string) ([]string, error) {
	allowed := map[string]bool{"codex": true, "claude": true, "grok": true, "deepseek": true, "gemini": true}
	seen := map[string]bool{}
	result := []string{}
	for _, id := range ids {
		if !allowed[id] {
			return nil, infraerrors.BadRequest("INVALID_TOOL_ID", "unsupported tool id")
		}
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result, nil
}

// Narrow capability keeps existing AdminService test doubles source-compatible.
func (s *adminServiceImpl) GetGroupToolMapping(ctx context.Context, id int64) (GroupToolMapping, error) {
	if _, err := s.groupRepo.GetByID(ctx, id); err != nil {
		return GroupToolMapping{}, err
	}
	repo, ok := s.groupRepo.(GroupToolMappingRepository)
	if !ok {
		return GroupToolMapping{}, infraerrors.InternalServer("TOOL_MAPPING_UNAVAILABLE", "tool mapping unavailable")
	}
	mappings, err := repo.ReadGroupToolMappings(ctx, []int64{id})
	if err != nil {
		return GroupToolMapping{}, err
	}
	if m, ok := mappings[id]; ok {
		return m, nil
	}
	return GroupToolMapping{GroupID: id, ToolIDs: []string{}}, nil
}
func (s *adminServiceImpl) SetGroupToolMapping(ctx context.Context, id int64, ids []string, actor int64) (GroupToolMapping, error) {
	normalized, err := NormalizeGroupToolIDs(ids)
	if err != nil {
		return GroupToolMapping{}, err
	}
	if _, err := s.groupRepo.GetByID(ctx, id); err != nil {
		return GroupToolMapping{}, err
	}
	repo, ok := s.groupRepo.(GroupToolMappingRepository)
	if !ok {
		return GroupToolMapping{}, infraerrors.InternalServer("TOOL_MAPPING_UNAVAILABLE", "tool mapping unavailable")
	}
	return repo.ReplaceGroupToolMapping(ctx, id, normalized, actor)
}
