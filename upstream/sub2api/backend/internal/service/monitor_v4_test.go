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
	native := &monitorV4NativeReaderStub{projection: map[int64]MonitorV4GroupProjection{
		7: {AvailabilityBucketCount: 1, TotalBucketCount: 1, TTFTP95MS: 120, LatencyP95MS: 900, SampleCount: 1},
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

func TestMonitorV4AvailabilityThresholdInputsRemainConcrete(t *testing.T) {
	projection := MonitorV4GroupProjection{AvailabilityBucketCount: 17, TotalBucketCount: 20, TTFTP95MS: 0, LatencyP95MS: 0, SampleCount: 0}
	if projection.TotalBucketCount == 0 || projection.TTFTP95MS < 0 || projection.LatencyP95MS < 0 {
		t.Fatal("zero-sample projection must retain concrete metric values")
	}
}
