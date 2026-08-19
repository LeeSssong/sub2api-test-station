package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type monitorV2GroupRepoStub struct {
	GroupRepository
	groups []Group
	err    error
}

func (s *monitorV2GroupRepoStub) ListActive(context.Context) ([]Group, error) {
	return append([]Group(nil), s.groups...), s.err
}

type monitorV2NativeReaderStub struct {
	projection map[int64]MonitorV2NativeGroupProjection
	err        error
	groupIDs   []int64
	start      time.Time
	end        time.Time
	bucketSize time.Duration
}

func (s *monitorV2NativeReaderStub) ProjectMonitorV2Groups(
	_ context.Context,
	groupIDs []int64,
	start, end time.Time,
	bucketSize time.Duration,
) (map[int64]MonitorV2NativeGroupProjection, error) {
	s.groupIDs = append([]int64(nil), groupIDs...)
	s.start = start
	s.end = end
	s.bucketSize = bucketSize
	return s.projection, s.err
}

func TestMonitorV2SnapshotUsesNativeProjectionV7(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	native := &monitorV2NativeReaderStub{projection: map[int64]MonitorV2NativeGroupProjection{
		7: {
			Status:                 MonitorV2StatusOperational,
			OperationalBucketCount: 23,
			TotalBucketCount:       24,
			TTFTP50MS:              floatPtr(10990),
			AverageLatencyMS:       floatPtr(10000),
			TTFTSampleCount:        27,
			LatencySampleCount:     26,
		},
	}}
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 7, Name: "GPT Pro", Platform: PlatformOpenAI, Status: StatusActive}}},
		native,
		nil,
	)

	snapshot, err := svc.Snapshot(context.Background(), MonitorV2Window24H, now)

	require.NoError(t, err)
	require.Equal(t, "7", snapshot.ContractVersion)
	require.Len(t, snapshot.Groups, 1)
	group := snapshot.Groups[0]
	require.Equal(t, MonitorV2StatusOperational, group.Status)
	require.Equal(t, 96.0, *group.Availability.Value)
	require.Equal(t, int64(24), group.Availability.SampleCount)
	require.Equal(t, 10990.0, *group.TTFT.Value)
	require.Equal(t, 10000.0, *group.AverageLatency.Value)
	require.Len(t, group.Timeline, 24)
	require.Equal(t, []int64{7}, native.groupIDs)
	require.Equal(t, time.Hour, native.bucketSize)
}

func TestMonitorV2SnapshotMatchesMicrosecondNativeBucketsWithNanosecondNow(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 123456789, time.UTC)
	start := now.Add(-24 * time.Hour).Truncate(time.Microsecond)
	native := &monitorV2NativeReaderStub{projection: map[int64]MonitorV2NativeGroupProjection{
		7: {
			Status:                 MonitorV2StatusOperational,
			OperationalBucketCount: 1,
			TotalBucketCount:       24,
			Timeline: []MonitorV2NativeTimelinePoint{{
				BucketStart: start,
				Status:      MonitorV2StatusOperational,
			}},
		},
	}}
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 7, Name: "Microsecond bucket", Status: StatusActive}}},
		native,
		nil,
	)

	snapshot, err := svc.Snapshot(context.Background(), MonitorV2Window24H, now)

	require.NoError(t, err)
	require.Equal(t, MonitorV2StatusOperational, snapshot.Groups[0].Timeline[0].Status)
}

func TestMonitorV2SnapshotReturnsFixedUnavailableBucketsForMissingNativeScope(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 7, Name: "No Native Accounts", Status: StatusActive}}},
		&monitorV2NativeReaderStub{projection: map[int64]MonitorV2NativeGroupProjection{}},
		nil,
	)

	for _, test := range []struct {
		window MonitorV2Window
		points int
	}{
		{MonitorV2Window24H, 24},
		{MonitorV2Window7D, 28},
		{MonitorV2Window30D, 30},
	} {
		t.Run(string(test.window), func(t *testing.T) {
			snapshot, err := svc.Snapshot(context.Background(), test.window, now)
			require.NoError(t, err)
			require.Len(t, snapshot.Groups[0].Timeline, test.points)
			require.Equal(t, MonitorV2StatusUnavailable, snapshot.Groups[0].Status)
			require.Equal(t, 0.0, *snapshot.Groups[0].Availability.Value)
			for _, point := range snapshot.Groups[0].Timeline {
				require.Equal(t, MonitorV2StatusUnavailable, point.Status)
			}
		})
	}
}

func TestMonitorV2SnapshotPreservesGroupOrderAndVisibility(t *testing.T) {
	native := &monitorV2NativeReaderStub{projection: map[int64]MonitorV2NativeGroupProjection{}}
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{
			{ID: 1, Name: "Standard", Status: StatusActive},
			{ID: 2, Name: "GPT Pro", Status: StatusActive},
			{ID: 3, Name: "Exclusive", Status: StatusActive, IsExclusive: true},
			{ID: 4, Name: "Disabled", Status: StatusDisabled},
		}},
		native,
		nil,
	)

	publicSnapshot, err := svc.Snapshot(context.Background(), MonitorV2Window7D, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, []int64{publicSnapshot.Groups[0].ID, publicSnapshot.Groups[1].ID})
	require.Equal(t, []int64{1, 2}, native.groupIDs)

	adminSnapshot, err := svc.Snapshot(context.Background(), MonitorV2Window7D, time.Now().UTC(), MonitorV2ScopeAdmin)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, 3}, []int64{adminSnapshot.Groups[0].ID, adminSnapshot.Groups[1].ID, adminSnapshot.Groups[2].ID})
}

func TestMonitorV2SnapshotPropagatesNativeReaderError(t *testing.T) {
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 1, Name: "Standard", Status: StatusActive}}},
		&monitorV2NativeReaderStub{err: errors.New("native unavailable")},
		nil,
	)

	_, err := svc.Snapshot(context.Background(), MonitorV2Window7D, time.Now().UTC())

	require.ErrorContains(t, err, "native unavailable")
}

func TestMonitorV2SnapshotRejectsUnsupportedWindow(t *testing.T) {
	svc := NewMonitorV2Service(&monitorV2GroupRepoStub{}, &monitorV2NativeReaderStub{}, nil)
	_, err := svc.Snapshot(context.Background(), MonitorV2Window("15d"), time.Now().UTC())
	require.ErrorContains(t, err, "unsupported monitor window")
}
