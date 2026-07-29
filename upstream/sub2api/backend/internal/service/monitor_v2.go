package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	MonitorV2ContractVersion = "2"

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
	monitorV2MaxModels      = 200
	monitorV2MaxTimeline    = 64
	monitorV2MaxTextLength  = 256
	monitorV2MetricWorkers  = 4
	monitorV2MinimumSamples = 5
)

type MonitorV2Window string

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
}

type MonitorV2Model struct {
	Name   string
	Status string
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
	Models             []MonitorV2Model
}

type MonitorV2Snapshot struct {
	ContractVersion string
	Window          MonitorV2Window
	GeneratedAt     time.Time
	Groups          []MonitorV2Group
}

type MonitorV2Service struct {
	groupRepo GroupRepository
	channels  MonitorV2ChannelReader
	probes    MonitorV2ProbeReader
	ops       MonitorV2OpsReader
	repo      MonitorV2Repository
}

func NewMonitorV2Service(
	groupRepo GroupRepository,
	channels MonitorV2ChannelReader,
	probes MonitorV2ProbeReader,
	ops MonitorV2OpsReader,
	repo MonitorV2Repository,
) *MonitorV2Service {
	return &MonitorV2Service{
		groupRepo: groupRepo,
		channels:  channels,
		probes:    probes,
		ops:       ops,
		repo:      repo,
	}
}

func (s *MonitorV2Service) Snapshot(
	ctx context.Context,
	window MonitorV2Window,
	now time.Time,
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
	publicGroups := make([]Group, 0, len(allGroups))
	groupIDs := make([]int64, 0, len(allGroups))
	for i := range allGroups {
		group := allGroups[i]
		if group.Status != StatusActive || group.IsExclusive {
			continue
		}
		publicGroups = append(publicGroups, group)
		groupIDs = append(groupIDs, group.ID)
	}
	if len(publicGroups) > monitorV2MaxGroups {
		return nil, fmt.Errorf("too many public groups: %d exceeds %d", len(publicGroups), monitorV2MaxGroups)
	}

	channels, _ := s.listChannels(ctx)
	probes, _ := s.listProbes(ctx)
	cacheStats, _ := s.listCacheStats(ctx, groupIDs, start, now)
	modelsByGroup := monitorV2ModelsByGroup(publicGroups, channels, probes)
	probesByGroup := monitorV2ProbesByGroup(probes)

	cards := make([]MonitorV2Group, len(publicGroups))
	sem := make(chan struct{}, monitorV2MetricWorkers)
	var wg sync.WaitGroup
	for i := range publicGroups {
		i := i
		group := publicGroups[i]
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
				probesByGroup[monitorV2GroupKey(group.Name)],
				modelsByGroup[group.ID],
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

	return &MonitorV2Snapshot{
		ContractVersion: MonitorV2ContractVersion,
		Window:          window,
		GeneratedAt:     now.UTC(),
		Groups:          cards,
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
		if len(group.Models) > monitorV2MaxModels {
			return fmt.Errorf("too many models for monitor v2 group %d: %d exceeds %d", group.ID, len(group.Models), monitorV2MaxModels)
		}
		if len(group.Timeline) > monitorV2MaxTimeline {
			return fmt.Errorf("too many timeline points for monitor v2 group %d: %d exceeds %d", group.ID, len(group.Timeline), monitorV2MaxTimeline)
		}
		for _, model := range group.Models {
			if len([]rune(model.Name)) > monitorV2MaxTextLength {
				return fmt.Errorf("model name too long for monitor v2 group %d", group.ID)
			}
		}
	}
	return nil
}

func (s *MonitorV2Service) listChannels(ctx context.Context) ([]AvailableChannel, error) {
	if s.channels == nil {
		return nil, nil
	}
	return s.channels.ListAvailable(ctx)
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
	models []MonitorV2Model,
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
		Status:             monitorV2GroupStatus(probes),
		Availability:       monitorV2UnavailableAvailability(),
		TTFT:               monitorV2UnavailableMetric(MonitorV2MetricInsufficientData),
		TTFTP95:            monitorV2UnavailableMetric(MonitorV2MetricInsufficientData),
		TPS:                monitorV2UnavailableMetric(MonitorV2MetricNotProvided),
		Latency:            monitorV2UnavailableMetric(MonitorV2MetricInsufficientData),
		LatencyP95:         monitorV2UnavailableMetric(MonitorV2MetricInsufficientData),
		CacheHit:           monitorV2CacheMetric(cache),
		Timeline:           []MonitorV2TimelinePoint{},
		Models:             monitorV2ApplyProbeStatuses(models, probes),
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
		QueryMode: OpsQueryModeAuto,
	}
	if overview, err := s.ops.GetDashboardOverview(ctx, filter); err == nil && overview != nil {
		card.Availability = monitorV2AvailabilityFromOverview(overview)
		card.TTFT = monitorV2PercentileMetric(overview.TTFT.P50, overview.TTFT.SampleCount)
		card.TTFTP95 = monitorV2PercentileMetric(overview.TTFT.P95, overview.TTFT.SampleCount)
		card.Latency = monitorV2PercentileMetric(overview.Duration.P50, overview.SuccessCount)
		card.LatencyP95 = monitorV2PercentileMetric(overview.Duration.P95, overview.SuccessCount)
	}

	throughput, throughputErr := s.ops.GetThroughputTrend(ctx, filter, bucketSeconds)
	errorsTrend, errorsErr := s.ops.GetErrorTrend(ctx, filter, bucketSeconds)
	if throughputErr == nil && errorsErr == nil {
		card.Timeline = monitorV2Timeline(throughput, errorsTrend)
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

func monitorV2ProbesByGroup(views []*UserMonitorView) map[string][]*UserMonitorView {
	out := make(map[string][]*UserMonitorView)
	for _, view := range views {
		if view == nil {
			continue
		}
		key := monitorV2GroupKey(view.GroupName)
		if key == "" {
			continue
		}
		out[key] = append(out[key], view)
	}
	return out
}

func monitorV2ModelsByGroup(
	publicGroups []Group,
	channels []AvailableChannel,
	probes []*UserMonitorView,
) map[int64][]MonitorV2Model {
	modelSets := make(map[int64]map[string]string)
	for i := range channels {
		channel := channels[i]
		if channel.Status != StatusActive {
			continue
		}
		for _, group := range channel.Groups {
			if modelSets[group.ID] == nil {
				modelSets[group.ID] = make(map[string]string)
			}
			for _, model := range channel.SupportedModels {
				name := strings.TrimSpace(model.Name)
				if name != "" {
					modelSets[group.ID][strings.ToLower(name)] = name
				}
			}
		}
	}
	groupNameToID := make(map[string]int64)
	for i := range publicGroups {
		groupNameToID[monitorV2GroupKey(publicGroups[i].Name)] = publicGroups[i].ID
	}
	for i := range channels {
		for _, group := range channels[i].Groups {
			if _, exists := groupNameToID[monitorV2GroupKey(group.Name)]; !exists {
				groupNameToID[monitorV2GroupKey(group.Name)] = group.ID
			}
		}
	}
	for _, probe := range probes {
		if probe == nil {
			continue
		}
		groupID := groupNameToID[monitorV2GroupKey(probe.GroupName)]
		if groupID == 0 {
			continue
		}
		if modelSets[groupID] == nil {
			modelSets[groupID] = make(map[string]string)
		}
		if name := strings.TrimSpace(probe.PrimaryModel); name != "" {
			modelSets[groupID][strings.ToLower(name)] = name
		}
		for _, extra := range probe.ExtraModels {
			if name := strings.TrimSpace(extra.Model); name != "" {
				modelSets[groupID][strings.ToLower(name)] = name
			}
		}
	}
	out := make(map[int64][]MonitorV2Model, len(modelSets))
	for groupID, names := range modelSets {
		models := make([]MonitorV2Model, 0, len(names))
		for _, name := range names {
			models = append(models, MonitorV2Model{Name: name})
		}
		sort.Slice(models, func(i, j int) bool {
			return strings.ToLower(models[i].Name) < strings.ToLower(models[j].Name)
		})
		out[groupID] = models
	}
	return out
}

func monitorV2ApplyProbeStatuses(
	models []MonitorV2Model,
	probes []*UserMonitorView,
) []MonitorV2Model {
	statuses := make(map[string][]string)
	for _, probe := range probes {
		if probe == nil {
			continue
		}
		if model := strings.TrimSpace(probe.PrimaryModel); model != "" {
			statuses[strings.ToLower(model)] = append(statuses[strings.ToLower(model)], probe.PrimaryStatus)
		}
		for _, extra := range probe.ExtraModels {
			model := strings.TrimSpace(extra.Model)
			if model != "" {
				statuses[strings.ToLower(model)] = append(statuses[strings.ToLower(model)], extra.Status)
			}
		}
	}
	out := append([]MonitorV2Model(nil), models...)
	for i := range out {
		out[i].Status = monitorV2ProbeStatuses(statuses[strings.ToLower(out[i].Name)])
	}
	return out
}

func monitorV2GroupStatus(probes []*UserMonitorView) string {
	if len(probes) == 0 {
		return MonitorV2StatusUnconfigured
	}
	statuses := make([]string, 0, len(probes))
	for _, probe := range probes {
		if probe != nil {
			statuses = append(statuses, probe.PrimaryStatus)
		}
	}
	return monitorV2ProbeStatuses(statuses)
}

func monitorV2ProbeStatuses(statuses []string) string {
	if len(statuses) == 0 {
		return MonitorV2StatusInsufficientData
	}
	operational := 0
	failed := 0
	degraded := 0
	for _, status := range statuses {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case MonitorStatusOperational:
			operational++
		case MonitorStatusDegraded:
			degraded++
		case MonitorStatusFailed, MonitorStatusError:
			failed++
		}
	}
	switch {
	case operational == len(statuses):
		return MonitorV2StatusOperational
	case failed == len(statuses):
		return MonitorV2StatusUnavailable
	case operational+degraded+failed == 0:
		return MonitorV2StatusInsufficientData
	default:
		return MonitorV2StatusDegraded
	}
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
		success int64
		errors  int64
	}
	byBucket := make(map[time.Time]counts)
	if throughput != nil {
		for _, point := range throughput.Points {
			if point == nil {
				continue
			}
			key := point.BucketStart.UTC()
			current := byBucket[key]
			current.success += point.RequestCount
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
		eligible := count.success + count.errors
		point := MonitorV2TimelinePoint{
			BucketStart:   key,
			State:         MonitorV2MetricInsufficientData,
			SuccessCount:  count.success,
			EligibleCount: eligible,
		}
		if eligible > 0 {
			value := float64(count.success) / float64(eligible) * 100
			point.State = MonitorV2MetricAvailable
			point.Value = &value
		}
		out = append(out, point)
	}
	return out
}
