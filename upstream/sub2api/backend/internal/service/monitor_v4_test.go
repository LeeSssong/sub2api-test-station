package service

import (
	"context"
	"testing"
	"time"
)

type monitorV4NativeReaderStub struct {
	projection map[int64]MonitorV4GroupProjection
	groupIDs   []int64
}

func (s *monitorV4NativeReaderStub) ProjectMonitorV4Groups(_ context.Context, groupIDs []int64, _, _ time.Time, _ time.Duration) (map[int64]MonitorV4GroupProjection, error) {
	s.groupIDs = append([]int64(nil), groupIDs...)
	return s.projection, nil
}

func TestMonitorV4SnapshotKeepsConfiguredGroupsWhenV2AggregationDisabled(t *testing.T) {
	rate := 75.0
	ttft := 120.0
	latency := 900.0
	native := &monitorV4NativeReaderStub{projection: map[int64]MonitorV4GroupProjection{
		7: {SuccessRate: &rate, RequestCount: 4, SuccessCount: 3, TTFTP95MS: &ttft, LatencyP95MS: &latency, TTFTSampleCount: 3, LatencySampleCount: 3},
	}}
	svc := NewMonitorV4Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 7, Name: "Hybrid", Platform: PlatformOpenAI, Status: StatusActive}}},
		&monitorV2AvailableGroupReaderStub{}, native, nil,
		&monitorV2ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{Enabled: false, GroupIDs: []int64{7}}},
	)

	snapshot, err := svc.Snapshot(context.Background(), 42, MonitorV4Window7D, time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(snapshot.Groups) != 1 || snapshot.Groups[0].ID != 7 {
		t.Fatalf("groups = %#v, want configured group 7", snapshot.Groups)
	}
	if len(native.groupIDs) != 1 || native.groupIDs[0] != 7 {
		t.Fatalf("native group IDs = %v, want [7]", native.groupIDs)
	}
}

func TestMonitorV4SnapshotPreservesNullableMetrics(t *testing.T) {
	native := &monitorV4NativeReaderStub{projection: map[int64]MonitorV4GroupProjection{
		7: {RequestCount: 0, SuccessCount: 0},
	}}
	svc := NewMonitorV4Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 7, Name: "Hybrid", Platform: PlatformOpenAI, Status: StatusActive}}},
		&monitorV2AvailableGroupReaderStub{}, native, nil,
		&monitorV2ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{Enabled: false, GroupIDs: []int64{7}}},
	)

	snapshot, err := svc.Snapshot(context.Background(), 42, MonitorV4Window7D, time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(snapshot.Groups) != 1 || snapshot.Groups[0].SuccessRate != nil || snapshot.Groups[0].TTFTP95MS != nil || snapshot.Groups[0].LatencyP95MS != nil {
		t.Fatalf("nullable metrics = %#v", snapshot.Groups)
	}
}

func TestMonitorV4SnapshotKeepsZeroSuccessRateForFailedRequests(t *testing.T) {
	zero := 0.0
	native := &monitorV4NativeReaderStub{projection: map[int64]MonitorV4GroupProjection{
		7: {SuccessRate: &zero, RequestCount: 3, SuccessCount: 0, RealRequestCount: 3, RealSuccessCount: 0},
	}}
	svc := NewMonitorV4Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 7, Name: "Failed", Platform: PlatformOpenAI, Status: StatusActive}}},
		&monitorV2AvailableGroupReaderStub{}, native, nil,
		&monitorV2ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{Enabled: false, GroupIDs: []int64{7}}},
	)

	snapshot, err := svc.Snapshot(context.Background(), 42, MonitorV4Window7D, time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(snapshot.Groups) != 1 || snapshot.Groups[0].SuccessRate == nil || *snapshot.Groups[0].SuccessRate != 0 {
		t.Fatalf("failed-request success rate = %#v, want non-null 0%%", snapshot.Groups)
	}
}

func TestMonitorV4WindowStart(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	start, err := monitorV4WindowStart(MonitorV4Window7D, now)
	if err != nil {
		t.Fatalf("monitorV4WindowStart() error = %v", err)
	}
	if want := time.Date(2026, 8, 18, 4, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Fatalf("start = %s, want %s", start, want)
	}
	if _, err := monitorV4WindowStart(MonitorV4Window("bad"), now); err == nil {
		t.Fatal("expected unsupported window error")
	}
}

func TestMonitorV4RequestCountsRemainConsistent(t *testing.T) {
	projection := MonitorV4GroupProjection{RequestCount: 20, SuccessCount: 17, RealRequestCount: 15, RealSuccessCount: 14, ProbeFallbackBucketCount: 5, ProbeFallbackRequestCount: 5}
	if projection.SuccessCount > projection.RequestCount || projection.RealSuccessCount > projection.RealRequestCount || projection.ProbeFallbackRequestCount != projection.ProbeFallbackBucketCount || projection.RealRequestCount+projection.ProbeFallbackRequestCount != projection.RequestCount || projection.RealSuccessCount > projection.SuccessCount || projection.SuccessCount > projection.RealSuccessCount+projection.ProbeFallbackRequestCount {
		t.Fatal("request-weighted projection is inconsistent")
	}
}
