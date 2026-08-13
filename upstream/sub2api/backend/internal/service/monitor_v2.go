package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	MonitorV2ContractVersion = "5"

	MonitorV2Window24H MonitorV2Window = "24h"
	MonitorV2Window7D  MonitorV2Window = "7d"
	MonitorV2Window30D MonitorV2Window = "30d"

	MonitorV2StatusOperational      = "operational"
	MonitorV2StatusDegraded         = "degraded"
	MonitorV2StatusUnavailable      = "unavailable"
	MonitorV2StatusUnconfigured     = "unconfigured"
	MonitorV2StatusInsufficientData = "insufficient_data"

	MonitorV2MetricAvailable        = "available"
	MonitorV2MetricInsufficientData = "insufficient_data"
	MonitorV2MetricNotProvided      = "not_provided"

	monitorV2MaxGroups      = 100
	monitorV2MaxTimeline    = 64
	monitorV2MaxTextLength  = 256
	monitorV2MetricWorkers  = 4
	monitorV2MinimumSamples = 5
)

type MonitorV2Window string

type MonitorV2Scope string

const (
	MonitorV2ScopePublic MonitorV2Scope = "public"
	MonitorV2ScopeAdmin  MonitorV2Scope = "admin"
)

type MonitorV2CacheStats struct {
	EvidenceAvailable bool
	RequestCount      int64
	HitCount          int64
}

type MonitorV2Repository interface {
	GetCacheStats(
		ctx context.Context,
		groupIDs []int64,
		start, end time.Time,
	) (map[int64]MonitorV2CacheStats, error)
}

type MonitorV2ChannelReader interface {
	ListAvailable(context.Context) ([]AvailableChannel, error)
}

type MonitorV2ProbeReader interface {
	ListUserView(context.Context) ([]*UserMonitorView, error)
}

type MonitorV2OpsReader interface {
	GetDashboardOverview(context.Context, *OpsDashboardFilter) (*OpsDashboardOverview, error)
	GetThroughputTrend(context.Context, *OpsDashboardFilter, int) (*OpsThroughputTrendResponse, error)
	GetErrorTrend(context.Context, *OpsDashboardFilter, int) (*OpsErrorTrendResponse, error)
	GetOpenAITokenStats(context.Context, *OpsOpenAITokenStatsFilter) (*OpsOpenAITokenStatsResponse, error)
}

type MonitorV2SettingsReader interface {
	GetAllSettings(context.Context) (*SystemSettings, error)
}

type MonitorV2Metric struct {
	State       string
	Value       *float64
	SampleCount int64
}

type MonitorV2Availability struct {
	MonitorV2Metric
	SuccessCount  int64
	EligibleCount int64
}

type MonitorV2TimelinePoint struct {
	BucketStart   time.Time
	State         string
	Value         *float64
	SuccessCount  int64
	EligibleCount int64
	LatencyMS     *int
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
	Status             string
	Availability       MonitorV2Availability
	TTFT               MonitorV2Metric
	TTFTP95            MonitorV2Metric
	TPS                MonitorV2Metric
	Latency            MonitorV2Metric
	LatencyP95         MonitorV2Metric
	CacheHit           MonitorV2Metric
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
	ops       MonitorV2OpsReader
	repo      MonitorV2Repository
	settings  MonitorV2SettingsReader
}

func NewMonitorV2Service(
	groupRepo GroupRepository,
	_ MonitorV2ChannelReader,
	probes MonitorV2ProbeReader,
	ops MonitorV2OpsReader,
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
		ops:       ops,
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
	start, bucketSeconds, err := monitorV2WindowBounds(window, now)
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
	groupIDs := make([]int64, 0, len(visibleGroups))
	for _, group := range visibleGroups {
		groupIDs = append(groupIDs, group.ID)
	}
	cacheStats, _ := s.listCacheStats(ctx, groupIDs, start, now)

	cards := make([]MonitorV2Group, len(visibleGroups))
	sem := make(chan struct{}, monitorV2MetricWorkers)
	var wg sync.WaitGroup
	for i := range visibleGroups {
		i := i
		group := visibleGroups[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			cards[i] = s.buildGroup(
				ctx,
				group,
				probesByGroup[group.ID],
				cacheStats[group.ID],
				window,
				start,
				now,
				bucketSeconds,
			)
		}()
	}
	wg.Wait()
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

func (s *MonitorV2Service) listCacheStats(
	ctx context.Context,
	groupIDs []int64,
	start, end time.Time,
) (map[int64]MonitorV2CacheStats, error) {
	if s.repo == nil {
		return map[int64]MonitorV2CacheStats{}, nil
	}
	return s.repo.GetCacheStats(ctx, groupIDs, start, end)
}

func (s *MonitorV2Service) buildGroup(
	ctx context.Context,
	group Group,
	probes []*UserMonitorView,
	cache MonitorV2CacheStats,
	window MonitorV2Window,
	start, end time.Time,
	bucketSeconds int,
) MonitorV2Group {
	card := MonitorV2Group{
		ID:                 group.ID,
		Name:               group.Name,
		Platform:           group.Platform,
		RateMultiplier:     group.RateMultiplier,
		PeakRateEnabled:    group.PeakRateEnabled,
		PeakStart:          group.PeakStart,
		PeakEnd:            group.PeakEnd,
		PeakRateMultiplier: group.PeakRateMultiplier,
		Status:             monitorV2GroupStatus(probes, end),
		Availability:       monitorV2UnavailableAvailability(),
		TTFT:               monitorV2UnavailableMetric(MonitorV2MetricInsufficientData),
		TTFTP95:            monitorV2UnavailableMetric(MonitorV2MetricInsufficientData),
		TPS:                monitorV2UnavailableMetric(MonitorV2MetricNotProvided),
		Latency:            monitorV2UnavailableMetric(MonitorV2MetricInsufficientData),
		LatencyP95:         monitorV2UnavailableMetric(MonitorV2MetricInsufficientData),
		CacheHit:           monitorV2CacheMetric(cache),
		Timeline:           monitorV2ProbeTimeline(probes, start, end),
	}
	if s.ops == nil {
		return card
	}

	groupID := group.ID
	filter := &OpsDashboardFilter{
		StartTime: start,
		EndTime:   end,
		Platform:  group.Platform,
		GroupID:   &groupID,
		QueryMode: OpsQueryModeRaw,
	}
	if overview, err := s.ops.GetDashboardOverview(ctx, filter); err == nil && overview != nil {
		card.Availability = monitorV2AvailabilityFromOverview(overview)
		card.TTFT = monitorV2PercentileMetric(overview.TTFT.P50, overview.TTFT.SampleCount)
		card.TTFTP95 = monitorV2PercentileMetric(overview.TTFT.P95, overview.TTFT.SampleCount)
		card.Latency = monitorV2PercentileMetric(overview.Duration.P50, overview.Duration.SampleCount)
		card.LatencyP95 = monitorV2PercentileMetric(overview.Duration.P95, overview.Duration.SampleCount)
	}

	if strings.EqualFold(group.Platform, PlatformOpenAI) {
		tokenStats, err := s.ops.GetOpenAITokenStats(ctx, &OpsOpenAITokenStatsFilter{
			TimeRange: string(window),
			StartTime: start,
			EndTime:   end,
			Platform:  group.Platform,
			GroupID:   &groupID,
			TopN:      100,
		})
		if err == nil {
			card.TPS = monitorV2TPSMetric(tokenStats)
		}
	}
	return card
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
				BucketStart: checkedAt, State: MonitorV2MetricAvailable,
				EligibleCount: 1, LatencyMS: latency,
			}
			switch strings.ToLower(strings.TrimSpace(item.Status)) {
			case "operational", "degraded", "success", "ok":
				value := float64(100)
				point.Value = &value
				point.SuccessCount = 1
			case "unavailable":
				// A probe that cannot reach the channel is still a completed
				// probe for the channel monitor. Keep it green/successful, but
				// leave latency empty so the UI can render a shorter bar.
				value := float64(100)
				point.Value = &value
				point.SuccessCount = 1
			case "failed", "error":
				value := float64(0)
				point.Value = &value
			default:
				point.State = MonitorV2MetricInsufficientData
				point.EligibleCount = 0
				point.LatencyMS = nil
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
		return MonitorV2StatusUnconfigured
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
		return MonitorV2StatusInsufficientData
	}

	latest := observations[latestIndex]
	switch latest.status {
	case MonitorStatusOperational:
		return MonitorV2StatusOperational
	case MonitorStatusDegraded:
		return MonitorV2StatusDegraded
	case MonitorStatusFailed, MonitorStatusError:
		for _, candidate := range observations {
			if candidate.checkedAt.Before(latest.checkedAt) && monitorV2ProbeStatusSuccessful(candidate.status) {
				return MonitorV2StatusDegraded
			}
		}
		return MonitorV2StatusUnavailable
	default:
		return MonitorV2StatusInsufficientData
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

func monitorV2ProbeStatusSuccessful(status string) bool {
	return status == MonitorStatusOperational || status == MonitorStatusDegraded
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

func monitorV2UnavailableMetric(state string) MonitorV2Metric {
	return MonitorV2Metric{State: state}
}

func monitorV2UnavailableAvailability() MonitorV2Availability {
	return MonitorV2Availability{
		MonitorV2Metric: MonitorV2Metric{State: MonitorV2MetricInsufficientData},
	}
}

func monitorV2AvailabilityFromOverview(overview *OpsDashboardOverview) MonitorV2Availability {
	out := MonitorV2Availability{
		SuccessCount:  overview.SuccessCount,
		EligibleCount: overview.RequestCountSLA,
		MonitorV2Metric: MonitorV2Metric{
			State:       MonitorV2MetricInsufficientData,
			SampleCount: overview.RequestCountSLA,
		},
	}
	if overview.RequestCountSLA <= 0 {
		return out
	}
	value := float64(overview.SuccessCount) / float64(overview.RequestCountSLA) * 100
	out.State = MonitorV2MetricAvailable
	out.Value = &value
	return out
}

func monitorV2PercentileMetric(value *int, sampleCount int64) MonitorV2Metric {
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

func monitorV2CacheMetric(stats MonitorV2CacheStats) MonitorV2Metric {
	if !stats.EvidenceAvailable {
		return MonitorV2Metric{State: MonitorV2MetricNotProvided}
	}
	out := MonitorV2Metric{
		State:       MonitorV2MetricInsufficientData,
		SampleCount: stats.RequestCount,
	}
	if stats.RequestCount < monitorV2MinimumSamples {
		return out
	}
	value := float64(stats.HitCount) / float64(stats.RequestCount) * 100
	out.State = MonitorV2MetricAvailable
	out.Value = &value
	return out
}

func monitorV2TPSMetric(stats *OpsOpenAITokenStatsResponse) MonitorV2Metric {
	out := MonitorV2Metric{State: MonitorV2MetricInsufficientData}
	if stats == nil {
		return out
	}
	var (
		samples  int64
		weighted float64
	)
	for _, item := range stats.Items {
		if item == nil || item.AvgTokensPerSec == nil || item.TPSSampleCount <= 0 {
			continue
		}
		samples += item.TPSSampleCount
		weighted += *item.AvgTokensPerSec * float64(item.TPSSampleCount)
	}
	out.SampleCount = samples
	if samples < monitorV2MinimumSamples {
		return out
	}
	value := weighted / float64(samples)
	out.State = MonitorV2MetricAvailable
	out.Value = &value
	return out
}

func monitorV2Timeline(
	throughput *OpsThroughputTrendResponse,
	errorsTrend *OpsErrorTrendResponse,
) []MonitorV2TimelinePoint {
	type counts struct {
		requests int64
		errors   int64
	}
	byBucket := make(map[time.Time]counts)
	if throughput != nil {
		for _, point := range throughput.Points {
			if point == nil {
				continue
			}
			key := point.BucketStart.UTC()
			current := byBucket[key]
			current.requests += point.RequestCount
			byBucket[key] = current
		}
	}
	if errorsTrend != nil {
		for _, point := range errorsTrend.Points {
			if point == nil {
				continue
			}
			key := point.BucketStart.UTC()
			current := byBucket[key]
			current.errors += point.ErrorCountSLA
			byBucket[key] = current
		}
	}
	keys := make([]time.Time, 0, len(byBucket))
	for key := range byBucket {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].Before(keys[j]) })

	out := make([]MonitorV2TimelinePoint, 0, len(keys))
	for _, key := range keys {
		count := byBucket[key]
		eligible := count.requests
		success := eligible - count.errors
		if success < 0 {
			success = 0
		}
		point := MonitorV2TimelinePoint{
			BucketStart:   key,
			State:         MonitorV2MetricInsufficientData,
			SuccessCount:  success,
			EligibleCount: eligible,
		}
		if eligible > 0 {
			value := float64(success) / float64(eligible) * 100
			point.State = MonitorV2MetricAvailable
			point.Value = &value
		}
		out = append(out, point)
	}
	return out
}

func monitorV2TrimBoundaryBucket(
	points []MonitorV2TimelinePoint,
	start, end time.Time,
	bucketSeconds int,
) []MonitorV2TimelinePoint {
	bucket := time.Duration(bucketSeconds) * time.Second
	window := end.Sub(start)
	if bucket <= 0 || window <= 0 {
		return points
	}
	expected := int((window + bucket - 1) / bucket)
	if len(points) != expected+1 {
		return points
	}
	return append([]MonitorV2TimelinePoint(nil), points[len(points)-expected:]...)
}
