package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	MonitorV4ContractVersion = "2"
	MonitorV4BucketSize      = 5 * time.Minute
	monitorV4MaxGroups       = 100
)

type MonitorV4Window string

const (
	MonitorV4Window1H  MonitorV4Window = "1h"
	MonitorV4Window24H MonitorV4Window = "24h"
	MonitorV4Window7D  MonitorV4Window = "7d"
)

type MonitorV4ProjectionReader interface {
	ProjectMonitorV4Groups(context.Context, []int64, time.Time, time.Time, time.Duration) (map[int64]MonitorV4GroupProjection, error)
}

type MonitorV4StoredWindow struct {
	Window          MonitorV4Window
	SnapshotID      string
	WindowStart     time.Time
	WindowEnd       time.Time
	GeneratedAt     time.Time
	ContractVersion string
	Groups          map[int64]MonitorV4GroupProjection
}

type MonitorV4SnapshotStore interface {
	LoadLatestMonitorV4Snapshot(context.Context, MonitorV4Window) (MonitorV4StoredWindow, error)
	ReplaceMonitorV4Snapshots(context.Context, string, []MonitorV4StoredWindow) error
}

type MonitorV4SnapshotRefresher interface {
	RefreshMonitorV4Snapshots(context.Context, time.Time) error
}

func ValidateMonitorV4Projection(projection MonitorV4GroupProjection) error {
	if projection.RequestCount < 0 || projection.SuccessCount < 0 || projection.RealRequestCount < 0 || projection.RealSuccessCount < 0 || projection.ProbeFallbackBucketCount < 0 || projection.ProbeFallbackRequestCount < 0 || projection.MissingProbeTerminalCount < 0 || projection.TTFTSampleCount < 0 || projection.LatencySampleCount < 0 || projection.InputTokens < 0 || projection.CacheReadTokens < 0 || projection.CacheCreationTokens < 0 || projection.CacheHitDenominator < 0 {
		return fmt.Errorf("monitor v4 projection contains negative counts")
	}
	if projection.CacheHitDenominator != projection.InputTokens+projection.CacheReadTokens+projection.CacheCreationTokens {
		return fmt.Errorf("monitor v4 cache denominator invariant violated")
	}
	if projection.SuccessCount > projection.RequestCount || projection.RealSuccessCount > projection.RealRequestCount || projection.ProbeFallbackRequestCount != projection.ProbeFallbackBucketCount || projection.RealRequestCount+projection.ProbeFallbackRequestCount != projection.RequestCount || projection.RealSuccessCount > projection.SuccessCount || projection.SuccessCount > projection.RealSuccessCount+projection.ProbeFallbackRequestCount {
		return fmt.Errorf("monitor v4 projection count invariants violated")
	}
	return nil
}

type MonitorV4Metric struct {
	Value       float64
	SampleCount int
}

type MonitorV4Group struct {
	ID                        int64
	Name                      string
	Platform                  string
	RateMultiplier            float64
	SuccessRate               *float64
	RequestCount              int
	SuccessCount              int
	RealRequestCount          int
	RealSuccessCount          int
	ProbeFallbackBucketCount  int
	ProbeFallbackRequestCount int
	TTFTP95MS                 *float64
	TTFTSampleCount           int
	LatencyP95MS              *float64
	LatencySampleCount        int
	CacheHitRate              *float64
	CacheReadTokens           int64
	CacheCreationTokens       int64
	CacheHitDenominator       int64
	SourceUpdatedAt           *time.Time
	CurrentOperational        bool
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
	store      MonitorV4SnapshotStore
}

func (s *MonitorV4Service) SetSnapshotStore(store MonitorV4SnapshotStore) {
	if s != nil {
		s.store = store
	}
}

func NewMonitorV4Service(groupRepo GroupRepository, available MonitorV2AvailableGroupReader, native MonitorV4ProjectionReader, settings MonitorV2SettingsReader, configured MonitorV2ConfiguredGroupReader) *MonitorV4Service {
	svc := &MonitorV4Service{groupRepo: groupRepo, available: available, native: native, settings: settings, configured: configured}
	if nativeService, ok := native.(*AccountMonitorService); ok {
		svc.store = nativeService
	}
	return svc
}

func (s *MonitorV4Service) Snapshot(ctx context.Context, userID int64, window MonitorV4Window, now time.Time) (*MonitorV4Snapshot, error) {
	_, err := monitorV4WindowStart(window, now)
	if err != nil {
		return nil, err
	}
	if s == nil || s.groupRepo == nil || s.available == nil || s.configured == nil {
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
	configuredGroupIDs := []int64(nil)
	if config != nil {
		configuredGroupIDs = config.GroupIDs
	}
	availableGroups, err := s.available.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load available groups for monitor v4: %w", err)
	}
	visibleGroups, _ := monitorV2VisibleGroups(allGroups, availableGroups, configuredGroupIDs, len(configuredGroupIDs) == 0)
	if len(visibleGroups) > monitorV4MaxGroups {
		return nil, fmt.Errorf("too many public groups: %d exceeds %d", len(visibleGroups), monitorV4MaxGroups)
	}
	if s.store == nil {
		return nil, fmt.Errorf("monitor v4 snapshot store unavailable")
	}
	stored, err := s.store.LoadLatestMonitorV4Snapshot(ctx, window)
	if err != nil {
		return nil, fmt.Errorf("load persisted monitor v4 snapshot: %w", err)
	}
	if stored.Window != window || stored.ContractVersion != MonitorV4ContractVersion || stored.WindowStart.IsZero() || stored.WindowEnd.IsZero() || !stored.WindowStart.Before(stored.WindowEnd) || stored.GeneratedAt.IsZero() || stored.SnapshotID == "" {
		return nil, fmt.Errorf("invalid persisted monitor v4 snapshot metadata")
	}
	for groupID, projection := range stored.Groups {
		if groupID <= 0 {
			return nil, fmt.Errorf("invalid persisted monitor v4 snapshot counts for group %d", groupID)
		}
		if err := ValidateMonitorV4Projection(projection); err != nil {
			return nil, fmt.Errorf("invalid persisted monitor v4 snapshot counts for group %d: %w", groupID, err)
		}
	}
	return s.snapshotWithGroups(ctx, window, stored.GeneratedAt, stored.WindowStart, visibleGroups, stored.Groups)
}

func (s *MonitorV4Service) RefreshMonitorV4Snapshots(ctx context.Context, asOf time.Time) error {
	if s == nil || s.groupRepo == nil || s.configured == nil || s.native == nil || s.store == nil {
		return fmt.Errorf("monitor v4 snapshot refresh dependencies unavailable")
	}
	end := asOf.UTC().Truncate(time.Minute)
	if end.IsZero() {
		return fmt.Errorf("monitor v4 snapshot refresh time unavailable")
	}
	allGroups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list active groups for monitor v4 snapshot refresh: %w", err)
	}
	config, err := s.configured.GetConfig(ctx)
	if err != nil {
		return fmt.Errorf("load channel monitor config for monitor v4 snapshot refresh: %w", err)
	}
	configuredIDs := map[int64]struct{}{}
	if config != nil {
		for _, id := range config.GroupIDs {
			if id > 0 {
				configuredIDs[id] = struct{}{}
			}
		}
	}
	groupIDs := make([]int64, 0, len(allGroups))
	for _, group := range allGroups {
		if group.Status != StatusActive || (len(configuredIDs) > 0 && func() bool { _, ok := configuredIDs[group.ID]; return !ok }()) {
			continue
		}
		groupIDs = append(groupIDs, group.ID)
	}
	snapshots := make([]MonitorV4StoredWindow, 0, 3)
	for _, window := range []MonitorV4Window{MonitorV4Window1H, MonitorV4Window24H, MonitorV4Window7D} {
		start, err := monitorV4WindowStart(window, end)
		if err != nil {
			return err
		}
		projections, err := s.native.ProjectMonitorV4Groups(ctx, groupIDs, start, end, MonitorV4BucketSize)
		if err != nil {
			return fmt.Errorf("project monitor v4 snapshot %s: %w", window, err)
		}
		for groupID, projection := range projections {
			if groupID <= 0 {
				return fmt.Errorf("project monitor v4 snapshot %s: invalid group id %d", window, groupID)
			}
			if err := ValidateMonitorV4Projection(projection); err != nil {
				return fmt.Errorf("project monitor v4 snapshot %s group %d: %w", window, groupID, err)
			}
		}
		snapshots = append(snapshots, MonitorV4StoredWindow{Window: window, SnapshotID: "pending", WindowStart: start, WindowEnd: end, GeneratedAt: end, ContractVersion: MonitorV4ContractVersion, Groups: projections})
	}
	snapshotID := uuid.NewString()
	for i := range snapshots {
		snapshots[i].SnapshotID = snapshotID
	}
	if err := s.store.ReplaceMonitorV4Snapshots(ctx, snapshotID, snapshots); err != nil {
		return fmt.Errorf("persist monitor v4 snapshots: %w", err)
	}
	return nil
}

func (s *MonitorV4Service) snapshotWithGroups(ctx context.Context, window MonitorV4Window, now, start time.Time, visibleGroups []Group, projections map[int64]MonitorV4GroupProjection) (*MonitorV4Snapshot, error) {
	now = now.UTC()
	cards := make([]MonitorV4Group, 0, len(visibleGroups))
	for _, group := range visibleGroups {
		projection := projections[group.ID]
		cards = append(cards, MonitorV4Group{
			ID: group.ID, Name: group.Name, Platform: group.Platform, RateMultiplier: group.RateMultiplier,
			SuccessRate: projection.SuccessRate, RequestCount: projection.RequestCount, SuccessCount: projection.SuccessCount,
			RealRequestCount: projection.RealRequestCount, RealSuccessCount: projection.RealSuccessCount,
			ProbeFallbackBucketCount: projection.ProbeFallbackBucketCount, ProbeFallbackRequestCount: projection.ProbeFallbackRequestCount,
			TTFTP95MS: projection.TTFTP95MS, TTFTSampleCount: projection.TTFTSampleCount,
			LatencyP95MS: projection.LatencyP95MS, LatencySampleCount: projection.LatencySampleCount,
			CacheHitRate: projection.CacheHitRate, CacheReadTokens: projection.CacheReadTokens, CacheCreationTokens: projection.CacheCreationTokens, CacheHitDenominator: projection.CacheHitDenominator,
			SourceUpdatedAt: projection.SourceUpdatedAt, CurrentOperational: projection.CurrentOperational,
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
	case MonitorV4Window1H:
		return now.Add(-time.Hour), nil
	case "", MonitorV4Window7D:
		return now.Add(-7 * 24 * time.Hour), nil
	case MonitorV4Window24H:
		return now.Add(-24 * time.Hour), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported monitor window %q", window)
	}
}
