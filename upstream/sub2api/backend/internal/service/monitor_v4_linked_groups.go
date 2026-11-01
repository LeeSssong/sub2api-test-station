package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type monitorV4LinkedGroups interface {
	LinkedMonitorGroups(context.Context, int64) ([]Group, error)
}

// LinkedMonitorGroups includes disabled keys; no credential is exposed to callers.
func (s *APIKeyService) LinkedMonitorGroups(ctx context.Context, userID int64) ([]Group, error) {
	seen := map[int64]bool{}
	groups := []Group{}
	for page := 1; ; page++ {
		keys, paging, err := s.apiKeyRepo.ListByUserID(ctx, userID, pagination.PaginationParams{Page: page, PageSize: 1000}, APIKeyListFilters{})
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			if key.Group != nil && !seen[key.Group.ID] {
				seen[key.Group.ID] = true
				groups = append(groups, *key.Group)
			}
		}
		if len(keys) < 1000 || paging == nil || page >= paging.Pages {
			break
		}
	}
	return groups, nil
}

func (s *MonitorV4Service) withLinkedMonitorGroups(ctx context.Context, userID int64, groups []Group) ([]Group, error) {
	reader, ok := s.available.(monitorV4LinkedGroups)
	if !ok {
		return groups, nil
	}
	linked, err := reader.LinkedMonitorGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	for _, g := range groups {
		seen[g.ID] = true
	}
	for _, g := range linked {
		if !seen[g.ID] {
			groups = append(groups, g)
			seen[g.ID] = true
		}
	}
	return groups, nil
}
