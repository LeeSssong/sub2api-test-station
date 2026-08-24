package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"
)

const (
	MonitorV4ContractVersion = "1"
	MonitorV4BucketSize      = 5 * time.Minute
	monitorV4MaxGroups       = 100
)

type MonitorV4Window string

const (
	MonitorV4Window24H MonitorV4Window = "24h"
	MonitorV4Window7D  MonitorV4Window = "7d"
	MonitorV4Window30D MonitorV4Window = "30d"
)

type MonitorV4ProjectionReader interface {
	ProjectMonitorV4Groups(context.Context, []int64, time.Time, time.Time, time.Duration) (map[int64]MonitorV4GroupProjection, error)
}

type MonitorV4Metric struct {
	Value       float64
	SampleCount int
}

type MonitorV4Group struct {
	ID                  int64
	Name                string
	Platform            string
	RateMultiplier      float64
	Availability        float64
	AvailabilityBuckets int
	TotalBuckets        int
	TTFTP95MS           float64
	LatencyP95MS        float64
	SampleCount         int
	SourceUpdatedAt     *time.Time
	CurrentOperational  bool
	MetricFallback      bool
}

type MonitorV4Snapshot struct {
	ContractVersion        string
	Window                 MonitorV4Window
	RefreshIntervalSeconds int
	GeneratedAt            time.Time
	Groups                 []MonitorV4Group
}

type MonitorV4Service struct {
	groupRepo  GroupRepository
	available  MonitorV2AvailableGroupReader
	configured MonitorV2ConfiguredGroupReader
	native     MonitorV4ProjectionReader
	settings   MonitorV2SettingsReader
}

func NewMonitorV4Service(groupRepo GroupRepository, available MonitorV2AvailableGroupReader, native MonitorV4ProjectionReader, settings MonitorV2SettingsReader, configured MonitorV2ConfiguredGroupReader) *MonitorV4Service {
	return &MonitorV4Service{groupRepo: groupRepo, available: available, native: native, settings: settings, configured: configured}
}

func (s *MonitorV4Service) Snapshot(ctx context.Context, userID int64, window MonitorV4Window, now time.Time) (*MonitorV4Snapshot, error) {
	start, err := monitorV4WindowStart(window, now)
	if err != nil {
		return nil, err
	}
	if s == nil || s.groupRepo == nil || s.available == nil || s.configured == nil || s.native == nil {
		return nil, fmt.Errorf("monitor v4 dependencies unavailable")
	}
	if userID <= 0 {
		return nil, fmt.Errorf("monitor v4 authenticated user unavailable")
	}
	allGroups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups for monitor v4: %w", err)
	}
	config, err := s.configured.GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load channel monitor config for monitor v4: %w", err)
	}
	if config == nil || !config.Enabled {
		return s.snapshotWithGroups(ctx, window, now, start, nil, nil)
	}
	availableGroups, err := s.available.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load available groups for monitor v4: %w", err)
	}
	visibleGroups, groupIDs := monitorV2VisibleGroups(allGroups, availableGroups, config.GroupIDs, len(config.GroupIDs) == 0)
	if len(visibleGroups) > monitorV4MaxGroups {
		return nil, fmt.Errorf("too many public groups: %d exceeds %d", len(visibleGroups), monitorV4MaxGroups)
	}
	projections, err := s.native.ProjectMonitorV4Groups(ctx, groupIDs, start, now.UTC(), MonitorV4BucketSize)
	if err != nil {
		return nil, fmt.Errorf("load hybrid monitor v4 projection: %w", err)
	}
	return s.snapshotWithGroups(ctx, window, now, start, visibleGroups, projections)
}

func (s *MonitorV4Service) snapshotWithGroups(ctx context.Context, window MonitorV4Window, now, start time.Time, visibleGroups []Group, projections map[int64]MonitorV4GroupProjection) (*MonitorV4Snapshot, error) {
	now = now.UTC()
	cards := make([]MonitorV4Group, 0, len(visibleGroups))
	for _, group := range visibleGroups {
		projection := projections[group.ID]
		availability := 0.0
		if projection.TotalBucketCount > 0 {
			availability = math.Round(float64(projection.AvailabilityBucketCount)*1000/float64(projection.TotalBucketCount)) / 10
		}
		cards = append(cards, MonitorV4Group{
			ID: group.ID, Name: group.Name, Platform: group.Platform, RateMultiplier: group.RateMultiplier,
			Availability: availability, AvailabilityBuckets: projection.AvailabilityBucketCount, TotalBuckets: projection.TotalBucketCount,
			TTFTP95MS: projection.TTFTP95MS, LatencyP95MS: projection.LatencyP95MS, SampleCount: projection.SampleCount,
			SourceUpdatedAt: projection.SourceUpdatedAt, CurrentOperational: projection.CurrentOperational, MetricFallback: projection.MetricFallback,
		})
	}
	refreshIntervalSeconds := MonitorPageRefreshIntervalSecondsDefault
	if s.settings != nil {
		if settings, settingsErr := s.settings.GetAllSettings(ctx); settingsErr == nil && settings != nil {
			refreshIntervalSeconds = NormalizeMonitorPageRefreshIntervalSeconds(strconv.Itoa(settings.MonitorPageRefreshIntervalSeconds))
		}
	}
	return &MonitorV4Snapshot{ContractVersion: MonitorV4ContractVersion, Window: window, RefreshIntervalSeconds: refreshIntervalSeconds, GeneratedAt: now, Groups: cards}, nil
}

func monitorV4WindowStart(window MonitorV4Window, now time.Time) (time.Time, error) {
	now = now.UTC()
	switch window {
	case "", MonitorV4Window7D:
		return now.Add(-7 * 24 * time.Hour), nil
	case MonitorV4Window24H:
		return now.Add(-24 * time.Hour), nil
	case MonitorV4Window30D:
		return now.Add(-30 * 24 * time.Hour), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported monitor window %q", window)
	}
}
