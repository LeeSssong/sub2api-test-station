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
	calls      int
}

type monitorV2AvailableGroupReaderStub struct {
	groups  []Group
	err     error
	userIDs []int64
}

type monitorV2ConfiguredGroupReaderStub struct {
	config *ChannelMonitorV2Config
	err    error
	calls  int
}

func (s *monitorV2ConfiguredGroupReaderStub) GetConfig(context.Context) (*ChannelMonitorV2Config, error) {
	s.calls++
	return s.config, s.err
}

func (s *monitorV2AvailableGroupReaderStub) GetAvailableGroups(_ context.Context, userID int64) ([]Group, error) {
	s.userIDs = append(s.userIDs, userID)
	return append([]Group(nil), s.groups...), s.err
}

func TestMonitorV2VisibleGroupsKeepsPublicAndAuthorizedExclusiveInRepositoryOrder(t *testing.T) {
	allGroups := []Group{
		{ID: 1, Name: "Public subscription", Status: StatusActive},
		{ID: 2, Name: "Allowed exclusive", Status: StatusActive, IsExclusive: true},
		{ID: 3, Name: "Denied exclusive", Status: StatusActive, IsExclusive: true},
		{ID: 4, Name: "Inactive exclusive", Status: StatusDisabled, IsExclusive: true},
		{ID: 5, Name: "Public absent from available", Status: StatusActive},
	}
	availableGroups := []Group{{ID: 2}, {ID: 2}, {ID: 4}}

	visible, ids := monitorV2VisibleGroups(allGroups, availableGroups, []int64{1, 2, 3, 4, 5}, false)

	require.Equal(t, []int64{1, 2, 5}, ids)
	require.Len(t, visible, 3)
	require.Equal(t, []string{"Public subscription", "Allowed exclusive", "Public absent from available"}, []string{
		visible[0].Name, visible[1].Name, visible[2].Name,
	})
}

func TestMonitorV2VisibleGroupsOnlyKeepsConfiguredActiveGroups(t *testing.T) {
	allGroups := []Group{
		{ID: 1, Name: "Configured public", Status: StatusActive},
		{ID: 2, Name: "Unconfigured public", Status: StatusActive},
		{ID: 3, Name: "Configured exclusive", Status: StatusActive, IsExclusive: true},
		{ID: 4, Name: "Disabled configured", Status: StatusDisabled},
	}
	availableGroups := []Group{{ID: 3}}

	visible, ids := monitorV2VisibleGroups(allGroups, availableGroups, []int64{1, 3, 4, 3}, false)

	require.Equal(t, []int64{1, 3}, ids)
	require.Equal(t, []string{"Configured public", "Configured exclusive"}, []string{visible[0].Name, visible[1].Name})
}

func TestMonitorV2VisibleGroupsTreatsEmptyConfigAsAllActiveGroups(t *testing.T) {
	allGroups := []Group{
		{ID: 1, Name: "Public", Status: StatusActive},
		{ID: 2, Name: "Exclusive", Status: StatusActive, IsExclusive: true},
		{ID: 3, Name: "Disabled", Status: StatusDisabled},
	}

	visible, ids := monitorV2VisibleGroups(allGroups, []Group{{ID: 2}}, nil, true)

	require.Equal(t, []int64{1, 2}, ids)
	require.Len(t, visible, 2)
}

func (s *monitorV2NativeReaderStub) ProjectMonitorV2Groups(
	_ context.Context,
	groupIDs []int64,
	start, end time.Time,
	bucketSize time.Duration,
) (map[int64]MonitorV2NativeGroupProjection, error) {
	s.calls++
	s.groupIDs = append([]int64(nil), groupIDs...)
	s.start = start
	s.end = end
	s.bucketSize = bucketSize
	return s.projection, s.err
}

func TestMonitorV2SnapshotUsesNativeProjectionV8(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	latestCheckedAt := now.Add(-45 * time.Second)
	native := &monitorV2NativeReaderStub{projection: map[int64]MonitorV2NativeGroupProjection{
		7: {
			Status:                 MonitorV2StatusOperational,
			SourceUpdatedAt:        &latestCheckedAt,
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
		&monitorV2AvailableGroupReaderStub{},
		native,
		nil,
		&monitorV2ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{Enabled: true, GroupIDs: []int64{7}}},
	)

	snapshot, err := svc.Snapshot(context.Background(), 42, MonitorV2Window24H, now)

	require.NoError(t, err)
	require.Equal(t, "8", snapshot.ContractVersion)
	require.Len(t, snapshot.Groups, 1)
	group := snapshot.Groups[0]
	require.Equal(t, MonitorV2StatusOperational, group.Status)
	require.NotNil(t, group.SourceUpdatedAt)
	require.Equal(t, latestCheckedAt, *group.SourceUpdatedAt)
	require.Equal(t, 96.0, *group.Availability.Value)
	require.Equal(t, int64(24), group.Availability.SampleCount)
	require.Equal(t, 10990.0, *group.TTFT.Value)
	require.Equal(t, 10000.0, *group.AverageLatency.Value)
	require.Len(t, group.Timeline, 24)
	require.Equal(t, []int64{7}, native.groupIDs)
	require.Equal(t, time.Hour, native.bucketSize)
}

func TestMonitorV2SnapshotUsesCurrentUserAvailableGroupsBeforeNativeProjection(t *testing.T) {
	available := &monitorV2AvailableGroupReaderStub{groups: []Group{{ID: 3}}}
	native := &monitorV2NativeReaderStub{projection: map[int64]MonitorV2NativeGroupProjection{}}
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{
			{ID: 1, Name: "Public", Status: StatusActive},
			{ID: 2, Name: "Denied exclusive", Status: StatusActive, IsExclusive: true},
			{ID: 3, Name: "Allowed exclusive", Status: StatusActive, IsExclusive: true},
		}}, available, native, nil, &monitorV2ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{Enabled: true, GroupIDs: []int64{1, 2, 3}}},
	)

	snapshot, err := svc.Snapshot(context.Background(), 42, MonitorV2Window7D, time.Now().UTC())

	require.NoError(t, err)
	require.Equal(t, []int64{42}, available.userIDs)
	require.Equal(t, []int64{1, 3}, native.groupIDs)
	require.Equal(t, []int64{1, 3}, []int64{snapshot.Groups[0].ID, snapshot.Groups[1].ID})
}

func TestMonitorV2SnapshotStopsBeforeNativeProjectionWhenAuthorizationFails(t *testing.T) {
	available := &monitorV2AvailableGroupReaderStub{err: errors.New("authorization unavailable")}
	native := &monitorV2NativeReaderStub{projection: map[int64]MonitorV2NativeGroupProjection{}}
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 1, Name: "Public", Status: StatusActive}}},
		available, native, nil, &monitorV2ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{Enabled: true, GroupIDs: []int64{1}}},
	)

	_, err := svc.Snapshot(context.Background(), 42, MonitorV2Window7D, time.Now().UTC())

	require.ErrorContains(t, err, "authorization unavailable")
	require.Nil(t, native.groupIDs)
	require.Zero(t, native.calls)
}

func TestMonitorV2SnapshotReturnsEmptyWhenChannelMonitorV2Disabled(t *testing.T) {
	native := &monitorV2NativeReaderStub{projection: map[int64]MonitorV2NativeGroupProjection{}}
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 1, Name: "Public", Status: StatusActive}}},
		&monitorV2AvailableGroupReaderStub{}, native, nil,
		&monitorV2ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{Enabled: false}},
	)

	snapshot, err := svc.Snapshot(context.Background(), 42, MonitorV2Window7D, time.Now().UTC())

	require.NoError(t, err)
	require.Empty(t, snapshot.Groups)
	require.Zero(t, native.calls)
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
				HasResult:   true,
			}},
		},
	}}
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 7, Name: "Microsecond bucket", Status: StatusActive}}},
		&monitorV2AvailableGroupReaderStub{},
		native,
		nil,
		&monitorV2ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{Enabled: true, GroupIDs: []int64{7}}},
	)

	snapshot, err := svc.Snapshot(context.Background(), 42, MonitorV2Window24H, now)

	require.NoError(t, err)
	require.Equal(t, MonitorV2StatusOperational, snapshot.Groups[0].Timeline[0].Status)
	require.True(t, snapshot.Groups[0].Timeline[0].HasResult)
}

func TestMonitorV2SnapshotReturnsFixedUnavailableBucketsForMissingNativeScope(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 7, Name: "No Native Accounts", Status: StatusActive}}},
		&monitorV2AvailableGroupReaderStub{},
		&monitorV2NativeReaderStub{projection: map[int64]MonitorV2NativeGroupProjection{}},
		nil,
		&monitorV2ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{Enabled: true, GroupIDs: []int64{7}}},
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
			snapshot, err := svc.Snapshot(context.Background(), 42, test.window, now)
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
	available := &monitorV2AvailableGroupReaderStub{groups: []Group{{ID: 3}}}
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{
			{ID: 1, Name: "Standard", Status: StatusActive},
			{ID: 2, Name: "GPT Pro", Status: StatusActive},
			{ID: 3, Name: "Exclusive", Status: StatusActive, IsExclusive: true},
			{ID: 4, Name: "Disabled", Status: StatusDisabled},
		}},
		available,
		native,
		nil,
		&monitorV2ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{Enabled: true, GroupIDs: []int64{1, 2, 3}}},
	)

	snapshot, err := svc.Snapshot(context.Background(), 42, MonitorV2Window7D, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, 3}, []int64{snapshot.Groups[0].ID, snapshot.Groups[1].ID, snapshot.Groups[2].ID})
	require.Equal(t, []int64{42}, available.userIDs)
	require.Equal(t, []int64{1, 2, 3}, native.groupIDs)
}

func TestMonitorV2SnapshotPropagatesNativeReaderError(t *testing.T) {
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{ID: 1, Name: "Standard", Status: StatusActive}}},
		&monitorV2AvailableGroupReaderStub{},
		&monitorV2NativeReaderStub{err: errors.New("native unavailable")},
		nil,
		&monitorV2ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{Enabled: true, GroupIDs: []int64{1}}},
	)

	_, err := svc.Snapshot(context.Background(), 42, MonitorV2Window7D, time.Now().UTC())

	require.ErrorContains(t, err, "native unavailable")
}

func TestMonitorV2SnapshotRejectsUnsupportedWindow(t *testing.T) {
	svc := NewMonitorV2Service(&monitorV2GroupRepoStub{}, &monitorV2AvailableGroupReaderStub{}, &monitorV2NativeReaderStub{}, nil, &monitorV2ConfiguredGroupReaderStub{})
	_, err := svc.Snapshot(context.Background(), 42, MonitorV2Window("15d"), time.Now().UTC())
	require.ErrorContains(t, err, "unsupported monitor window")
}
