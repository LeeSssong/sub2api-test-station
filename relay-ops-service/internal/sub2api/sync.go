package sub2api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
)

const NativeMetricSchemaV1 = "sub2api-native-v1"

type PublicGroupRecord struct {
	GroupID           int64
	Name              string
	Platform          string
	Enabled           bool
	CustomerVisible   bool
	UserMultiplierBPS int64
	ChannelIDs        []int64
	MonitorIDs        []int64
	HealthGate        string
	SourceRevision    string
	LastSeenAt        time.Time
}

type MetricRef struct {
	SourceKind    string
	ExternalID    string
	WindowStart   time.Time
	WindowEnd     time.Time
	PayloadHash   string
	SchemaVersion string
}

type SyncSink interface {
	UpsertPublicGroup(context.Context, PublicGroupRecord) error
	AppendMetricRef(context.Context, MetricRef) error
}

type MonitorObserver interface {
	ObserveMonitor(context.Context, ChannelMonitor, MonitorHistory) error
}

type Synchronizer struct {
	Reader   Reader
	Sink     SyncSink
	Observer MonitorObserver
	Now      func() time.Time
}

func (s Synchronizer) Sync(ctx context.Context) error {
	channels, err := s.Reader.ListChannels(ctx)
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}
	groups, err := s.Reader.ListGroups(ctx)
	if err != nil {
		return fmt.Errorf("list groups: %w", err)
	}
	monitors, err := s.Reader.ListChannelMonitors(ctx)
	if err != nil {
		return fmt.Errorf("list channel monitors: %w", err)
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	for _, group := range groups {
		if !group.CustomerVisible() {
			continue
		}
		channelIDs := activeChannelIDs(group.ID, channels)
		if len(channelIDs) == 0 {
			continue
		}
		groupMonitors := matchingMonitors(group.Name, monitors)
		monitorIDs := make([]int64, 0, len(groupMonitors))
		for _, monitor := range groupMonitors {
			monitorIDs = append(monitorIDs, monitor.ID)
		}
		record := PublicGroupRecord{
			GroupID: group.ID, Name: group.Name, Platform: group.Platform,
			Enabled: true, CustomerVisible: true,
			UserMultiplierBPS: int64(math.Round(group.RateMultiplier * 10_000)),
			ChannelIDs:        channelIDs, MonitorIDs: monitorIDs, HealthGate: "pending",
			SourceRevision: payloadHash(struct {
				Group    Group   `json:"group"`
				Channels []int64 `json:"channels"`
				Monitors []int64 `json:"monitors"`
			}{group, channelIDs, monitorIDs}),
			LastSeenAt: now,
		}
		if err := s.Sink.UpsertPublicGroup(ctx, record); err != nil {
			return fmt.Errorf("upsert public group %d: %w", group.ID, err)
		}
		if err := s.collectOpsRef(ctx, group.ID); err != nil {
			return err
		}
		for _, monitor := range groupMonitors {
			if err := s.collectMonitorRef(ctx, monitor); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s Synchronizer) collectOpsRef(ctx context.Context, groupID int64) error {
	snapshot, err := s.Reader.GetOpsSnapshot(ctx, OpsQuery{TimeRange: "24h", GroupID: groupID})
	if err != nil {
		return fmt.Errorf("get Ops snapshot for group %d: %w", groupID, err)
	}
	start, err := time.Parse(time.RFC3339, snapshot.Overview.StartTime)
	if err != nil {
		return fmt.Errorf("Ops snapshot start time malformed")
	}
	end, err := time.Parse(time.RFC3339, snapshot.Overview.EndTime)
	if err != nil {
		return fmt.Errorf("Ops snapshot end time malformed")
	}
	return s.Sink.AppendMetricRef(ctx, MetricRef{
		SourceKind: "sub2api_real_traffic", ExternalID: "group:" + strconv.FormatInt(groupID, 10),
		WindowStart: start.UTC(), WindowEnd: end.UTC(), PayloadHash: payloadHash(snapshot), SchemaVersion: NativeMetricSchemaV1,
	})
}

func (s Synchronizer) collectMonitorRef(ctx context.Context, monitor ChannelMonitor) error {
	history, err := s.Reader.GetChannelMonitorHistory(ctx, monitor.ID, monitor.PrimaryModel, 60)
	if err != nil {
		return fmt.Errorf("get monitor %d history: %w", monitor.ID, err)
	}
	if len(history) == 0 {
		return nil
	}
	start, end, err := historyWindow(history)
	if err != nil {
		return fmt.Errorf("monitor %d history time malformed", monitor.ID)
	}
	if err := s.Sink.AppendMetricRef(ctx, MetricRef{
		SourceKind: "sub2api_native_monitor", ExternalID: "monitor:" + strconv.FormatInt(monitor.ID, 10),
		WindowStart: start, WindowEnd: end, PayloadHash: payloadHash(history), SchemaVersion: NativeMetricSchemaV1,
	}); err != nil {
		return err
	}
	if s.Observer != nil {
		latest, err := latestHistory(history)
		if err != nil {
			return fmt.Errorf("monitor %d latest history malformed", monitor.ID)
		}
		if err := s.Observer.ObserveMonitor(ctx, monitor, latest); err != nil {
			return fmt.Errorf("observe monitor %d: %w", monitor.ID, err)
		}
	}
	return nil
}

func activeChannelIDs(groupID int64, channels []Channel) []int64 {
	ids := make([]int64, 0)
	for _, channel := range channels {
		if channel.Status != "active" {
			continue
		}
		for _, id := range channel.GroupIDs {
			if id == groupID {
				ids = append(ids, channel.ID)
				break
			}
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func matchingMonitors(groupName string, monitors []ChannelMonitor) []ChannelMonitor {
	matched := make([]ChannelMonitor, 0)
	for _, monitor := range monitors {
		if monitor.Enabled && monitor.GroupName == groupName {
			matched = append(matched, monitor)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	return matched
}

func historyWindow(history []MonitorHistory) (time.Time, time.Time, error) {
	var start, end time.Time
	for _, item := range history {
		checkedAt, err := time.Parse(time.RFC3339, item.CheckedAt)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		checkedAt = checkedAt.UTC()
		if start.IsZero() || checkedAt.Before(start) {
			start = checkedAt
		}
		if end.IsZero() || checkedAt.After(end) {
			end = checkedAt
		}
	}
	return start, end, nil
}

func latestHistory(history []MonitorHistory) (MonitorHistory, error) {
	if len(history) == 0 {
		return MonitorHistory{}, fmt.Errorf("empty history")
	}
	latest := history[0]
	latestTime, err := time.Parse(time.RFC3339, latest.CheckedAt)
	if err != nil {
		return MonitorHistory{}, err
	}
	for _, item := range history[1:] {
		checkedAt, err := time.Parse(time.RFC3339, item.CheckedAt)
		if err != nil {
			return MonitorHistory{}, err
		}
		if checkedAt.After(latestTime) {
			latest = item
			latestTime = checkedAt
		}
	}
	return latest, nil
}

func payloadHash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
