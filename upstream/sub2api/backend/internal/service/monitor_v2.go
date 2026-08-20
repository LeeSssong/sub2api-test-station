package service

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	MonitorV2ContractVersion = "7"

	MonitorV2Window24H MonitorV2Window = "24h"
	MonitorV2Window7D  MonitorV2Window = "7d"
	MonitorV2Window30D MonitorV2Window = "30d"

	MonitorV2StatusOperational = "operational"
	MonitorV2StatusUnavailable = "unavailable"

	MonitorV2MetricAvailable        = "available"
	MonitorV2MetricInsufficientData = "insufficient_data"

	monitorV2MaxGroups     = 100
	monitorV2MaxTextLength = 256
)

type MonitorV2Window string

type MonitorV2NativeProbeReader interface {
	ProjectMonitorV2Groups(context.Context, []int64, time.Time, time.Time, time.Duration) (map[int64]MonitorV2NativeGroupProjection, error)
}

type MonitorV2AvailableGroupReader interface {
	GetAvailableGroups(context.Context, int64) ([]Group, error)
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
	Status             string
	Availability       MonitorV2Metric
	TTFT               MonitorV2Metric
	AverageLatency     MonitorV2Metric
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
	available MonitorV2AvailableGroupReader
	native    MonitorV2NativeProbeReader
	settings  MonitorV2SettingsReader
}

func monitorV2VisibleGroups(allGroups, availableGroups []Group) ([]Group, []int64) {
	availableIDs := make(map[int64]struct{}, len(availableGroups))
	for _, group := range availableGroups {
		availableIDs[group.ID] = struct{}{}
	}
	visibleGroups := make([]Group, 0, len(allGroups))
	groupIDs := make([]int64, 0, len(allGroups))
	for _, group := range allGroups {
		if group.Status != StatusActive {
			continue
		}
		if group.IsExclusive {
			if _, ok := availableIDs[group.ID]; !ok {
				continue
			}
		}
		visibleGroups = append(visibleGroups, group)
		groupIDs = append(groupIDs, group.ID)
	}
	return visibleGroups, groupIDs
}

func NewMonitorV2Service(groupRepo GroupRepository, available MonitorV2AvailableGroupReader, native MonitorV2NativeProbeReader, settings MonitorV2SettingsReader) *MonitorV2Service {
	return &MonitorV2Service{groupRepo: groupRepo, available: available, native: native, settings: settings}
}

func (s *MonitorV2Service) Snapshot(ctx context.Context, userID int64, window MonitorV2Window, now time.Time) (*MonitorV2Snapshot, error) {
	start, bucketCount, bucketSize, err := monitorV2WindowBounds(window, now)
	if err != nil {
		return nil, err
	}
	if s == nil || s.groupRepo == nil {
		return nil, fmt.Errorf("monitor v2 group repository unavailable")
	}
	if userID <= 0 {
		return nil, fmt.Errorf("monitor v2 authenticated user unavailable")
	}
	if s.available == nil {
		return nil, fmt.Errorf("monitor v2 available group reader unavailable")
	}
	if s.native == nil {
		return nil, fmt.Errorf("monitor v2 native probe reader unavailable")
	}
	allGroups, err := s.groupRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active groups for monitor v2: %w", err)
	}
	availableGroups, err := s.available.GetAvailableGroups(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load available groups for monitor v2: %w", err)
	}
	visibleGroups, groupIDs := monitorV2VisibleGroups(allGroups, availableGroups)
	if len(visibleGroups) > monitorV2MaxGroups {
		return nil, fmt.Errorf("too many public groups: %d exceeds %d", len(visibleGroups), monitorV2MaxGroups)
	}
	projections, err := s.native.ProjectMonitorV2Groups(ctx, groupIDs, start, now.UTC(), bucketSize)
	if err != nil {
		return nil, fmt.Errorf("load native monitor v2 projection: %w", err)
	}
	cards := make([]MonitorV2Group, 0, len(visibleGroups))
	for _, group := range visibleGroups {
		projection, ok := projections[group.ID]
		if !ok {
			projection = monitorV2EmptyProjection(start, bucketCount, bucketSize)
		}
		cards = append(cards, monitorV2GroupFromProjection(group, projection, start, bucketCount, bucketSize))
	}
	if err := monitorV2ValidateSnapshotBounds(cards); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	refreshIntervalSeconds := MonitorPageRefreshIntervalSecondsDefault
	if s.settings != nil {
		if settings, settingsErr := s.settings.GetAllSettings(ctx); settingsErr == nil && settings != nil {
			refreshIntervalSeconds = NormalizeMonitorPageRefreshIntervalSeconds(strconv.Itoa(settings.MonitorPageRefreshIntervalSeconds))
		}
	}
	return &MonitorV2Snapshot{ContractVersion: MonitorV2ContractVersion, Window: window, RefreshIntervalSeconds: refreshIntervalSeconds, GeneratedAt: now.UTC(), Groups: cards}, nil
}

func monitorV2WindowBounds(window MonitorV2Window, now time.Time) (time.Time, int, time.Duration, error) {
	now = now.UTC()
	switch window {
	case MonitorV2Window24H:
		return now.Add(-24 * time.Hour).Truncate(time.Microsecond), 24, time.Hour, nil
	case MonitorV2Window7D:
		return now.Add(-7 * 24 * time.Hour).Truncate(time.Microsecond), 28, 6 * time.Hour, nil
	case MonitorV2Window30D:
		return now.Add(-30 * 24 * time.Hour).Truncate(time.Microsecond), 30, 24 * time.Hour, nil
	default:
		return time.Time{}, 0, 0, fmt.Errorf("unsupported monitor window %q", window)
	}
}

func monitorV2EmptyProjection(start time.Time, bucketCount int, bucketSize time.Duration) MonitorV2NativeGroupProjection {
	timeline := make([]MonitorV2NativeTimelinePoint, 0, bucketCount)
	for index := 0; index < bucketCount; index++ {
		timeline = append(timeline, MonitorV2NativeTimelinePoint{BucketStart: start.Add(time.Duration(index) * bucketSize), Status: MonitorV2StatusUnavailable})
	}
	return MonitorV2NativeGroupProjection{Status: MonitorV2StatusUnavailable, TotalBucketCount: bucketCount, Timeline: timeline}
}

func monitorV2GroupFromProjection(group Group, projection MonitorV2NativeGroupProjection, start time.Time, bucketCount int, bucketSize time.Duration) MonitorV2Group {
	return MonitorV2Group{
		ID: group.ID, Name: group.Name, Platform: group.Platform, RateMultiplier: group.RateMultiplier,
		PeakRateEnabled: group.PeakRateEnabled, PeakStart: group.PeakStart, PeakEnd: group.PeakEnd, PeakRateMultiplier: group.PeakRateMultiplier,
		Status:         monitorV2NormalizeStatus(projection.Status),
		Availability:   monitorV2AvailabilityMetric(projection.OperationalBucketCount, projection.TotalBucketCount),
		TTFT:           monitorV2Metric(projection.TTFTP50MS, projection.TTFTSampleCount),
		AverageLatency: monitorV2Metric(projection.AverageLatencyMS, projection.LatencySampleCount),
		Timeline:       monitorV2Timeline(projection.Timeline, start, bucketCount, bucketSize),
	}
}

func monitorV2AvailabilityMetric(operational, total int) MonitorV2Metric {
	if total < 0 {
		total = 0
	}
	if operational < 0 {
		operational = 0
	}
	if operational > total {
		operational = total
	}
	value := 0.0
	if total > 0 {
		value = math.Round(float64(operational) * 100 / float64(total))
	}
	return MonitorV2Metric{State: MonitorV2MetricAvailable, Value: &value, SampleCount: int64(total)}
}

func monitorV2Metric(value *float64, sampleCount int) MonitorV2Metric {
	metric := MonitorV2Metric{State: MonitorV2MetricInsufficientData, SampleCount: int64(monitorV2MaxInt(sampleCount, 0))}
	if value != nil && sampleCount > 0 {
		metric.State = MonitorV2MetricAvailable
		metric.Value = value
	}
	return metric
}

func monitorV2Timeline(points []MonitorV2NativeTimelinePoint, start time.Time, bucketCount int, bucketSize time.Duration) []MonitorV2TimelinePoint {
	byBucket := make(map[time.Time]MonitorV2NativeTimelinePoint, len(points))
	for _, point := range points {
		point.BucketStart = point.BucketStart.UTC().Truncate(time.Microsecond)
		byBucket[point.BucketStart] = point
	}
	start = start.UTC().Truncate(time.Microsecond)
	result := make([]MonitorV2TimelinePoint, 0, bucketCount)
	for index := 0; index < bucketCount; index++ {
		bucketStart := start.Add(time.Duration(index) * bucketSize).UTC()
		point, ok := byBucket[bucketStart]
		if !ok {
			result = append(result, MonitorV2TimelinePoint{BucketStart: bucketStart, Status: MonitorV2StatusUnavailable})
			continue
		}
		result = append(result, MonitorV2TimelinePoint{BucketStart: bucketStart, Status: monitorV2NormalizeStatus(point.Status), LatencyMS: monitorV2RoundedInt(point.LatencyMS)})
	}
	return result
}

func monitorV2RoundedInt(value *float64) *int {
	if value == nil {
		return nil
	}
	rounded := int(math.Round(*value))
	return &rounded
}

func monitorV2NormalizeStatus(status string) string {
	if strings.EqualFold(strings.TrimSpace(status), MonitorV2StatusOperational) {
		return MonitorV2StatusOperational
	}
	return MonitorV2StatusUnavailable
}

func monitorV2MaxInt(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
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
	}
	return nil
}
