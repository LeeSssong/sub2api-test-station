package httpserver

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"example.invalid/relay-ops-service/internal/candidates"
	"example.invalid/relay-ops-service/internal/sub2api"
)

type NativePricingSource struct {
	Reader sub2api.Reader
	Clock  func() time.Time
}

func (s NativePricingSource) PublicPricing(ctx context.Context) ([]PublicGroup, error) {
	groups, err := s.Reader.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list public pricing groups: %w", err)
	}
	channels, err := s.Reader.ListChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list public pricing channels: %w", err)
	}
	visible := make(map[int64]string)
	for _, group := range groups {
		if group.CustomerVisible() {
			visible[group.ID] = group.Name
		}
	}
	now := time.Now().UTC()
	if s.Clock != nil {
		now = s.Clock().UTC()
	}
	updated := now.Format("2006-01-02 15:04 UTC")
	byGroup := make(map[int64]*PublicGroup)
	for _, channel := range channels {
		if channel.Status != "active" {
			continue
		}
		for _, groupID := range channel.GroupIDs {
			name, ok := visible[groupID]
			if !ok {
				continue
			}
			group := byGroup[groupID]
			if group == nil {
				group = &PublicGroup{Name: name, UpdatedAt: updated}
				byGroup[groupID] = group
			}
			for _, price := range channel.ModelPricing {
				for _, modelID := range price.Models {
					group.Models = append(group.Models, PublicModel{ModelID: modelID, Input: priceValue(price.InputPrice), Output: priceValue(price.OutputPrice), CacheRead: priceValue(price.CacheReadPrice), CacheWrite: priceValue(price.CacheWritePrice)})
				}
			}
		}
	}
	result := make([]PublicGroup, 0, len(byGroup))
	for _, group := range byGroup {
		result = append(result, *group)
	}
	return result, nil
}

func priceValue(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

type OpsRepository interface {
	ListCandidates(context.Context) ([]candidates.Candidate, error)
	ListPublicGroupNames(context.Context) ([]string, error)
}

type DatabaseOpsSource struct{ Repository OpsRepository }

func (s DatabaseOpsSource) Snapshot(ctx context.Context) (OpsView, error) {
	groups, err := s.Repository.ListPublicGroupNames(ctx)
	if err != nil {
		return OpsView{}, err
	}
	items, err := s.Repository.ListCandidates(ctx)
	if err != nil {
		return OpsView{}, err
	}
	candidateViews := make([]CandidateView, 0, len(items))
	for _, item := range items {
		status := "已停用"
		if item.Enabled {
			status = "等待采集"
		}
		candidateViews = append(candidateViews, CandidateView{ID: int64(item.ID), Name: item.Name, Status: status, LastCheck: "-", PriceChange: "-"})
	}
	return OpsView{PublicGroups: groups, NativeMonitorURL: "/monitor", Candidates: candidateViews}, nil
}
