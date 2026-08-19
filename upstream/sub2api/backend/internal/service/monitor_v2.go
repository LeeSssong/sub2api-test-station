package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	MonitorV2ContractVersion = "6"

	MonitorV2Window24H MonitorV2Window = "24h"
	MonitorV2Window7D  MonitorV2Window = "7d"
	MonitorV2Window30D MonitorV2Window = "30d"

	MonitorV2StatusOperational = "operational"
	MonitorV2StatusUnavailable = "unavailable"
	// Legacy constants remain for internal source compatibility. The v6
	// snapshot path only returns operational or unavailable.
	MonitorV2StatusDegraded         = "degraded"
	MonitorV2StatusUnconfigured     = "unconfigured"
	MonitorV2StatusInsufficientData = "insufficient_data"

	MonitorV2MetricAvailable        = "available"
	MonitorV2MetricInsufficientData = "insufficient_data"
	MonitorV2MetricNotProvided      = "not_provided"

	monitorV2MaxGroups      = 100
	monitorV2MaxTimeline    = 64
	monitorV2MaxTextLength  = 256
	monitorV2MinimumSamples = 5
)

type MonitorV2Window string

type MonitorV2Scope string

const (
	MonitorV2ScopePublic MonitorV2Scope = "public"
	MonitorV2ScopeAdmin  MonitorV2Scope = "admin"
)

type MonitorV2PerformanceScope struct {
	GroupID int64
	Model   string
}

type MonitorV2PerformanceStats struct {
	SampleCount  int64
	TTFTP50MS    *int
	TTFTP95MS    *int
	LatencyP50MS *int
	LatencyP95MS *int
	TPS          *float64
}

type MonitorV2Repository interface {
	GetPerformanceStats(context.Context, []MonitorV2PerformanceScope, time.Time, time.Time) (map[int64]MonitorV2PerformanceStats, error)
}

type MonitorV2ProbeReader interface {
	ListUserView(context.Context) ([]*UserMonitorView, error)
}

type MonitorV2SettingsReader interface {
	GetAllSettings(context.Context) (*SystemSettings, error)
}

type MonitorV2Metric struct {
	State       string
	Value       *float64
	SampleCount int64
}

type MonitorV2TimelinePoint struct {
	BucketStart time.Time
	Status      string
	LatencyMS   *int
}

type MonitorV2Group struct {
	ID                 int64
	Name               string
	Platform           string
	RateMultiplier     float64
	PeakRateEnabled    bool
	PeakStart          string
	PeakEnd            string
	PeakRateMultiplier float64
	IsFlagship         bool
	Status             string
	TTFT               MonitorV2Metric
	TTFTP95            MonitorV2Metric
	TPS                MonitorV2Metric
	Latency            MonitorV2Metric
	LatencyP95         MonitorV2Metric
	Timeline           []MonitorV2TimelinePoint
}

type MonitorV2Snapshot struct {
	ContractVersion        string
	Window                 MonitorV2Window
	RefreshIntervalSeconds int
	GeneratedAt            time.Time
	Groups                 []MonitorV2Group
}

type MonitorV2Service struct {
	groupRepo GroupRepository
	probes    MonitorV2ProbeReader
	repo      MonitorV2Repository
	settings  MonitorV2SettingsReader
}

func NewMonitorV2Service(
	groupRepo GroupRepository,
	probes MonitorV2ProbeReader,
	repo MonitorV2Repository,
	settings ...MonitorV2SettingsReader,
) *MonitorV2Service {
	var settingsReader MonitorV2SettingsReader
	if len(settings) > 0 {
		settingsReader = settings[0]
	}
	return &MonitorV2Service{
		groupRepo: groupRepo,
		probes:    probes,
		repo:      repo,
		settings:  settingsReader,
	}
}

func (s *MonitorV2Service) Snapshot(
	ctx context.Context,
	window MonitorV2Window,
	now time.Time,
	scopes ...MonitorV2Scope,
) (*MonitorV2Snapshot, error) {
	start, _, err := monitorV2WindowBounds(window, now)
	if err != nil {
		return nil, err
	}
	if s == nil || s.groupRepo == nil {
		return nil, fmt.Errorf("monitor v2 group repository unavailable")
	}

	allGroups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups for monitor v2: %w", err)
	}
	scope := MonitorV2ScopePublic
	if len(scopes) > 0 && scopes[0] == MonitorV2ScopeAdmin {
		scope = MonitorV2ScopeAdmin
	}
	visibleGroups := make([]Group, 0, len(allGroups))
	for i := range allGroups {
		group := allGroups[i]
		if group.Status != StatusActive || (scope != MonitorV2ScopeAdmin && group.IsExclusive) {
			continue
		}
		visibleGroups = append(visibleGroups, group)
	}
	if len(visibleGroups) > monitorV2MaxGroups {
		return nil, fmt.Errorf("too many public groups: %d exceeds %d", len(visibleGroups), monitorV2MaxGroups)
	}

	probes, _ := s.listProbes(ctx)
	probesByGroup := monitorV2ProbesByGroup(visibleGroups, probes)
	if s.probes != nil {
		configuredGroups := visibleGroups[:0]
		for _, group := range visibleGroups {
			if len(probesByGroup[group.ID]) > 0 {
				configuredGroups = append(configuredGroups, group)
			}
		}
		visibleGroups = configuredGroups
	}
	sort.SliceStable(visibleGroups, func(i, j int) bool {
		return monitorV2IsFlagshipGroup(visibleGroups[i].Name) && !monitorV2IsFlagshipGroup(visibleGroups[j].Name)
	})
	performanceScopes := make([]MonitorV2PerformanceScope, 0, len(visibleGroups))
	for _, group := range visibleGroups {
		performanceScopes = append(performanceScopes, MonitorV2PerformanceScope{
			GroupID: group.ID,
			Model:   monitorV2PrimaryModel(probesByGroup[group.ID]),
		})
	}
	performanceStats, _ := s.listPerformanceStats(ctx, performanceScopes, start, now)

	cards := make([]MonitorV2Group, 0, len(visibleGroups))
	for _, group := range visibleGroups {
		cards = append(cards, s.buildGroup(
			group,
			probesByGroup[group.ID],
			performanceStats[group.ID],
			start,
			now,
		))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := monitorV2ValidateSnapshotBounds(cards); err != nil {
		return nil, err
	}
	refreshIntervalSeconds := MonitorPageRefreshIntervalSecondsDefault
	if s.settings != nil {
		if settings, err := s.settings.GetAllSettings(ctx); err == nil && settings != nil {
			refreshIntervalSeconds = NormalizeMonitorPageRefreshIntervalSeconds(
				strconv.Itoa(settings.MonitorPageRefreshIntervalSeconds),
			)
		}
	}

	return &MonitorV2Snapshot{
		ContractVersion:        MonitorV2ContractVersion,
		Window:                 window,
		RefreshIntervalSeconds: refreshIntervalSeconds,
		GeneratedAt:            now.UTC(),
		Groups:                 cards,
	}, nil
}

func monitorV2ValidateSnapshotBounds(groups []MonitorV2Group) error {
	if len(groups) > monitorV2MaxGroups {
		return fmt.Errorf("too many public groups: %d exceeds %d", len(groups), monitorV2MaxGroups)
	}
	for _, group := range groups {
		if len([]rune(group.Name)) > monitorV2MaxTextLength {
			return fmt.Errorf("group name too long for monitor v2")
		}
		if len([]rune(group.Platform)) > monitorV2MaxTextLength {
			return fmt.Errorf("group platform too long for monitor v2")
		}
		if len(group.Timeline) > monitorV2MaxTimeline {
			return fmt.Errorf("too many timeline points for monitor v2 group %d: %d exceeds %d", group.ID, len(group.Timeline), monitorV2MaxTimeline)
		}
	}
	return nil
}

func (s *MonitorV2Service) listProbes(ctx context.Context) ([]*UserMonitorView, error) {
	if s.probes == nil {
		return nil, nil
	}
	return s.probes.ListUserView(ctx)
}

func (s *MonitorV2Service) listPerformanceStats(
	ctx context.Context,
	scopes []MonitorV2PerformanceScope,
	start, end time.Time,
) (map[int64]MonitorV2PerformanceStats, error) {
	if s.repo == nil {
		return map[int64]MonitorV2PerformanceStats{}, nil
	}
	return s.repo.GetPerformanceStats(ctx, scopes, start, end)
}

func (s *MonitorV2Service) buildGroup(
	group Group,
	probes []*UserMonitorView,
	stats MonitorV2PerformanceStats,
	start, end time.Time,
) MonitorV2Group {
	return MonitorV2Group{
		ID:                 group.ID,
		Name:               group.Name,
		Platform:           group.Platform,
		RateMultiplier:     group.RateMultiplier,
		PeakRateEnabled:    group.PeakRateEnabled,
		PeakStart:          group.PeakStart,
		PeakEnd:            group.PeakEnd,
		PeakRateMultiplier: group.PeakRateMultiplier,
		IsFlagship:         monitorV2IsFlagshipGroup(group.Name),
		Status:             monitorV2GroupStatus(probes, end),
		TTFT:               monitorV2PerformanceMetric(stats.TTFTP50MS, stats.SampleCount),
		TTFTP95:            monitorV2PerformanceMetric(stats.TTFTP95MS, stats.SampleCount),
		TPS:                monitorV2PerformanceFloatMetric(stats.TPS, stats.SampleCount),
		Latency:            monitorV2PerformanceMetric(stats.LatencyP50MS, stats.SampleCount),
		LatencyP95:         monitorV2PerformanceMetric(stats.LatencyP95MS, stats.SampleCount),
		Timeline:           monitorV2ProbeTimeline(probes, start, end),
	}
}

func monitorV2ProbeTimeline(probes []*UserMonitorView, start, end time.Time) []MonitorV2TimelinePoint {
	points := make([]MonitorV2TimelinePoint, 0, monitorV2MaxTimeline)
	for _, probe := range probes {
		if probe == nil {
			continue
		}
		for _, item := range probe.Timeline {
			checkedAt := item.CheckedAt.UTC()
			if checkedAt.Before(start.UTC()) || checkedAt.After(end.UTC()) {
				continue
			}
			latency := item.LatencyMs
			if latency == nil {
				latency = item.PingLatencyMs
			}
			point := MonitorV2TimelinePoint{
				BucketStart: checkedAt,
				Status:      monitorV2TimelineStatus(item.Status),
				LatencyMS:   latency,
			}
			points = append(points, point)
		}
	}
	sort.SliceStable(points, func(i, j int) bool { return points[i].BucketStart.Before(points[j].BucketStart) })
	if len(points) > monitorV2MaxTimeline {
		points = points[len(points)-monitorV2MaxTimeline:]
	}
	return points
}

func monitorV2TimelineStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case MonitorStatusOperational, MonitorStatusDegraded, "success", "ok":
		return MonitorV2StatusOperational
	default:
		return MonitorV2StatusUnavailable
	}
}

func monitorV2WindowBounds(window MonitorV2Window, now time.Time) (time.Time, int, error) {
	now = now.UTC()
	switch window {
	case MonitorV2Window24H:
		return now.Add(-24 * time.Hour), 3600, nil
	case MonitorV2Window7D:
		return now.Add(-7 * 24 * time.Hour), 6 * 3600, nil
	case MonitorV2Window30D:
		return now.Add(-30 * 24 * time.Hour), 24 * 3600, nil
	default:
		return time.Time{}, 0, fmt.Errorf("unsupported monitor window %q", window)
	}
}

func monitorV2GroupKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func monitorV2IsFlagshipGroup(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if strings.Contains(normalized, "旗舰") {
		return true
	}
	parts := strings.FieldsFunc(normalized, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for _, part := range parts {
		if part == "pro" {
			return true
		}
	}
	return false
}

func monitorV2PrimaryModel(probes []*UserMonitorView) string {
	type candidate struct {
		model     string
		checkedAt time.Time
	}
	candidates := make([]candidate, 0, len(probes))
	for _, probe := range probes {
		if probe == nil {
			continue
		}
		model := strings.TrimSpace(probe.PrimaryModel)
		if model == "" {
			continue
		}
		candidates = append(candidates, candidate{model: model, checkedAt: probe.PrimaryCheckedAt.UTC()})
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].checkedAt.Equal(candidates[j].checkedAt) {
			return candidates[i].model < candidates[j].model
		}
		return candidates[i].checkedAt.After(candidates[j].checkedAt)
	})
	return candidates[0].model
}

func monitorV2ProbesByGroup(groups []Group, views []*UserMonitorView) map[int64][]*UserMonitorView {
	visibleIDs, groupNameToID := monitorV2VisibleGroupLookup(groups)
	out := make(map[int64][]*UserMonitorView)
	for _, view := range views {
		if view == nil {
			continue
		}
		groupID := monitorV2ProbeGroupID(view, visibleIDs, groupNameToID)
		if groupID == 0 {
			continue
		}
		out[groupID] = append(out[groupID], view)
	}
	return out
}

func monitorV2VisibleGroupLookup(groups []Group) (map[int64]struct{}, map[string]int64) {
	visibleIDs := make(map[int64]struct{}, len(groups))
	groupNameToID := make(map[string]int64, len(groups))
	for i := range groups {
		group := groups[i]
		visibleIDs[group.ID] = struct{}{}
		if key := monitorV2GroupKey(group.Name); key != "" {
			groupNameToID[key] = group.ID
		}
	}
	return visibleIDs, groupNameToID
}

func monitorV2ProbeGroupID(
	probe *UserMonitorView,
	visibleIDs map[int64]struct{},
	groupNameToID map[string]int64,
) int64 {
	if probe == nil {
		return 0
	}
	if probe.GroupID != nil {
		if _, visible := visibleIDs[*probe.GroupID]; visible {
			return *probe.GroupID
		}
		return 0
	}
	return groupNameToID[monitorV2GroupKey(probe.GroupName)]
}

func monitorV2GroupStatus(probes []*UserMonitorView, now time.Time) string {
	if len(probes) == 0 {
		return MonitorV2StatusUnavailable
	}
	type observation struct {
		status    string
		checkedAt time.Time
	}
	observations := make([]observation, 0, len(probes))
	for _, probe := range probes {
		if probe == nil {
			continue
		}
		if monitorV2ProbeObservationFresh(probe.IntervalSeconds, probe.PrimaryCheckedAt, now) {
			observations = append(observations, observation{
				status:    monitorV2NormalizeProbeStatus(probe.PrimaryStatus),
				checkedAt: probe.PrimaryCheckedAt,
			})
		}
		for _, point := range probe.Timeline {
			if !monitorV2ProbeObservationFresh(probe.IntervalSeconds, point.CheckedAt, now) {
				continue
			}
			observations = append(observations, observation{
				status:    monitorV2NormalizeProbeStatus(point.Status),
				checkedAt: point.CheckedAt,
			})
		}
	}

	latestIndex := -1
	for i := range observations {
		if observations[i].status == "" {
			continue
		}
		if latestIndex == -1 || observations[i].checkedAt.After(observations[latestIndex].checkedAt) ||
			(observations[i].checkedAt.Equal(observations[latestIndex].checkedAt) &&
				monitorV2ProbeStatusPriority(observations[i].status) > monitorV2ProbeStatusPriority(observations[latestIndex].status)) {
			latestIndex = i
		}
	}
	if latestIndex == -1 {
		return MonitorV2StatusUnavailable
	}

	latest := observations[latestIndex]
	switch latest.status {
	case MonitorStatusOperational, MonitorStatusDegraded:
		return MonitorV2StatusOperational
	case MonitorStatusFailed, MonitorStatusError:
		return MonitorV2StatusUnavailable
	default:
		return MonitorV2StatusUnavailable
	}
}

func monitorV2NormalizeProbeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case MonitorStatusOperational:
		return MonitorStatusOperational
	case MonitorStatusDegraded:
		return MonitorStatusDegraded
	case MonitorStatusFailed:
		return MonitorStatusFailed
	case MonitorStatusError:
		return MonitorStatusError
	default:
		return ""
	}
}

func monitorV2ProbeStatusPriority(status string) int {
	switch status {
	case MonitorStatusOperational:
		return 3
	case MonitorStatusDegraded:
		return 2
	case MonitorStatusFailed, MonitorStatusError:
		return 1
	default:
		return 0
	}
}

func monitorV2ProbeObservationFresh(intervalSeconds int, checkedAt, now time.Time) bool {
	if intervalSeconds <= 0 || checkedAt.IsZero() {
		return false
	}
	return !checkedAt.Before(now.UTC().Add(-2 * time.Duration(intervalSeconds) * time.Second))
}

func monitorV2PerformanceMetric(value *int, sampleCount int64) MonitorV2Metric {
	out := MonitorV2Metric{
		State:       MonitorV2MetricInsufficientData,
		SampleCount: sampleCount,
	}
	if value == nil || sampleCount < monitorV2MinimumSamples {
		return out
	}
	n := float64(*value)
	out.State = MonitorV2MetricAvailable
	out.Value = &n
	return out
}

func monitorV2PerformanceFloatMetric(value *float64, sampleCount int64) MonitorV2Metric {
	out := MonitorV2Metric{
		State:       MonitorV2MetricInsufficientData,
		SampleCount: sampleCount,
	}
	if value == nil || sampleCount < monitorV2MinimumSamples {
		return out
	}
	out.State = MonitorV2MetricAvailable
	out.Value = value
	return out
}
