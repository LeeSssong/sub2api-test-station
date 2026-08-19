package service

import (
	"context"
	"strings"
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

type monitorV2ProbeReaderStub struct {
	views []*UserMonitorView
	err   error
}

func (s *monitorV2ProbeReaderStub) ListUserView(context.Context) ([]*UserMonitorView, error) {
	return append([]*UserMonitorView(nil), s.views...), s.err
}

type monitorV2RepoStub struct {
	performance map[int64]MonitorV2PerformanceStats
	scopes      []MonitorV2PerformanceScope
	start       time.Time
	end         time.Time
}

func (s *monitorV2RepoStub) GetPerformanceStats(
	_ context.Context,
	scopes []MonitorV2PerformanceScope,
	start time.Time,
	end time.Time,
) (map[int64]MonitorV2PerformanceStats, error) {
	s.scopes = append([]MonitorV2PerformanceScope(nil), scopes...)
	s.start = start
	s.end = end
	out := make(map[int64]MonitorV2PerformanceStats, len(s.performance))
	for id, stat := range s.performance {
		out[id] = stat
	}
	return out, nil
}

func TestMonitorV2SnapshotScopeControlsExclusiveGroups(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	publicGroupID := int64(1)
	exclusiveGroupID := int64(2)
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{
			{ID: 1, Name: "公开组", Platform: PlatformOpenAI, Status: StatusActive},
			{ID: 2, Name: "专属组", Platform: PlatformOpenAI, Status: StatusActive, IsExclusive: true},
			{ID: 3, Name: "停用组", Platform: PlatformOpenAI, Status: StatusDisabled},
		}},
		&monitorV2ProbeReaderStub{views: []*UserMonitorView{
			{GroupID: &publicGroupID},
			{GroupID: &exclusiveGroupID},
		}},
		nil,
	)

	publicSnapshot, err := svc.Snapshot(context.Background(), MonitorV2Window7D, now, MonitorV2ScopePublic)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, []int64{publicSnapshot.Groups[0].ID})

	adminSnapshot, err := svc.Snapshot(context.Background(), MonitorV2Window7D, now, MonitorV2ScopeAdmin)
	require.NoError(t, err)
	require.Len(t, adminSnapshot.Groups, 2)
	require.Equal(t, []int64{1, 2}, []int64{adminSnapshot.Groups[0].ID, adminSnapshot.Groups[1].ID})
}

func TestMonitorV2SnapshotShowsOnlyGroupsWithEnabledMonitors(t *testing.T) {
	now := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	configuredID := int64(20)
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{
			{ID: 20, Name: "GPT-特惠分组", Platform: PlatformOpenAI, Status: StatusActive},
			{ID: 21, Name: "历史分组", Platform: PlatformOpenAI, Status: StatusActive},
		}},
		&monitorV2ProbeReaderStub{views: []*UserMonitorView{{GroupID: &configuredID}}},
		nil,
	)

	snapshot, err := svc.Snapshot(context.Background(), MonitorV2Window7D, now)

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, configuredID, snapshot.Groups[0].ID)
}

func TestMonitorV2SnapshotUsesUnifiedPerformanceScopeForSevenDayWindow(t *testing.T) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	groupID := int64(20)
	repo := &monitorV2RepoStub{}
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{
			ID: groupID, Name: "GPT-特惠分组", Platform: PlatformOpenAI, Status: StatusActive,
		}}},
		&monitorV2ProbeReaderStub{views: []*UserMonitorView{{
			GroupID: &groupID, PrimaryModel: "gpt-5.6-sol", PrimaryCheckedAt: now,
		}}},
		repo,
	)

	_, err := svc.Snapshot(context.Background(), MonitorV2Window7D, now)

	require.NoError(t, err)
	require.Equal(t, []MonitorV2PerformanceScope{{GroupID: groupID, Model: "gpt-5.6-sol"}}, repo.scopes)
	require.Equal(t, now.Add(-7*24*time.Hour), repo.start)
	require.Equal(t, now, repo.end)
}

func TestMonitorV2SnapshotKeepsLatestProbeResults(t *testing.T) {
	now := time.Date(2026, 7, 30, 5, 17, 0, 0, time.UTC)
	group := Group{
		ID:          16,
		Name:        "公开标准",
		Platform:    PlatformAnthropic,
		Status:      StatusActive,
		IsExclusive: false,
	}

	tests := []struct {
		name   string
		window MonitorV2Window
	}{
		{name: "7d", window: MonitorV2Window7D},
		{name: "30d", window: MonitorV2Window30D},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			points := []UserMonitorTimelinePoint{
				{Status: "operational", LatencyMs: intPtr(250), CheckedAt: now.Add(-2 * time.Hour)},
				{Status: "failed", LatencyMs: intPtr(900), CheckedAt: now.Add(-time.Hour)},
			}
			svc := NewMonitorV2Service(
				&monitorV2GroupRepoStub{groups: []Group{group}},
				&monitorV2ProbeReaderStub{views: []*UserMonitorView{{GroupID: int64Ptr(group.ID), Timeline: points}}},
				&monitorV2RepoStub{},
			)

			snapshot, err := svc.Snapshot(context.Background(), tt.window, now)

			require.NoError(t, err)
			require.Len(t, snapshot.Groups, 1)
			require.Len(t, snapshot.Groups[0].Timeline, 2)
			require.Equal(t, points[0].CheckedAt, snapshot.Groups[0].Timeline[0].BucketStart)
			require.Equal(t, MonitorV2StatusOperational, snapshot.Groups[0].Timeline[0].Status)
			require.Equal(t, MonitorV2StatusUnavailable, snapshot.Groups[0].Timeline[1].Status)
		})
	}
}

func TestMonitorV2MetricsShareUnifiedPerformanceSampleCount(t *testing.T) {
	t.Run("insufficient unified samples hide every performance value", func(t *testing.T) {
		ttftP50 := 420
		ttftP95 := 880
		latencyP50 := 1320
		latencyP95 := 2400
		tps := 46.5
		svc := NewMonitorV2Service(
			&monitorV2GroupRepoStub{groups: []Group{{
				ID:          1,
				Name:        "公开标准",
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				IsExclusive: false,
			}}},
			&monitorV2ProbeReaderStub{views: []*UserMonitorView{{
				GroupID: int64Ptr(1), PrimaryModel: "gpt-5.6-sol",
			}}},
			&monitorV2RepoStub{performance: map[int64]MonitorV2PerformanceStats{
				1: {
					SampleCount: 3,
					TTFTP50MS:   &ttftP50, TTFTP95MS: &ttftP95,
					LatencyP50MS: &latencyP50, LatencyP95MS: &latencyP95,
					TPS: &tps,
				},
			}},
		)

		snapshot, err := svc.Snapshot(
			context.Background(),
			MonitorV2Window24H,
			time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC),
		)

		require.NoError(t, err)
		require.Equal(t, MonitorV2MetricInsufficientData, snapshot.Groups[0].TTFT.State)
		require.Equal(t, int64(3), snapshot.Groups[0].TTFT.SampleCount)
		require.Equal(t, MonitorV2MetricInsufficientData, snapshot.Groups[0].TTFTP95.State)
		require.Equal(t, int64(3), snapshot.Groups[0].TTFTP95.SampleCount)
		for _, metric := range []MonitorV2Metric{
			snapshot.Groups[0].TTFT,
			snapshot.Groups[0].TTFTP95,
			snapshot.Groups[0].TPS,
			snapshot.Groups[0].Latency,
			snapshot.Groups[0].LatencyP95,
		} {
			require.Equal(t, MonitorV2MetricInsufficientData, metric.State)
			require.Equal(t, int64(3), metric.SampleCount)
			require.Nil(t, metric.Value)
		}
	})
}

func TestMonitorV2SnapshotRejectsStaleProbeStatus(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{
			ID:          17,
			Name:        "公开探针组",
			Platform:    PlatformOpenAI,
			Status:      StatusActive,
			IsExclusive: false,
		}}},
		&monitorV2ProbeReaderStub{views: []*UserMonitorView{{
			GroupName:        "公开探针组",
			PrimaryModel:     "gpt-5.4",
			PrimaryStatus:    MonitorStatusOperational,
			IntervalSeconds:  60,
			PrimaryCheckedAt: now.Add(-2*time.Minute - time.Second),
			Timeline: []UserMonitorTimelinePoint{{
				Status:    MonitorStatusOperational,
				CheckedAt: now.Add(-2*time.Minute - time.Second),
			}},
		}}},
		&monitorV2RepoStub{},
	)

	snapshot, err := svc.Snapshot(context.Background(), MonitorV2Window7D, now)

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, MonitorV2StatusUnavailable, snapshot.Groups[0].Status)
}

func TestMonitorV2GroupStatusUsesLatestProbeObservation(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)

	t.Run("latest success wins over an older failure", func(t *testing.T) {
		probes := []*UserMonitorView{
			{
				PrimaryStatus:    MonitorStatusFailed,
				IntervalSeconds:  60,
				PrimaryCheckedAt: now.Add(-30 * time.Second),
			},
			{
				PrimaryStatus:    MonitorStatusOperational,
				IntervalSeconds:  60,
				PrimaryCheckedAt: now,
			},
		}

		require.Equal(t, MonitorV2StatusOperational, monitorV2GroupStatus(probes, now))
	})

	t.Run("latest failure with a recent success is unavailable", func(t *testing.T) {
		probes := []*UserMonitorView{{
			PrimaryStatus:    MonitorStatusFailed,
			IntervalSeconds:  60,
			PrimaryCheckedAt: now,
			Timeline: []UserMonitorTimelinePoint{{
				Status:    MonitorStatusOperational,
				CheckedAt: now.Add(-60 * time.Second),
			}},
		}}

		require.Equal(t, MonitorV2StatusUnavailable, monitorV2GroupStatus(probes, now))
	})

	t.Run("degraded probes are presented as operational", func(t *testing.T) {
		probes := []*UserMonitorView{{
			PrimaryStatus:    MonitorStatusDegraded,
			IntervalSeconds:  60,
			PrimaryCheckedAt: now,
		}}

		require.Equal(t, MonitorV2StatusOperational, monitorV2GroupStatus(probes, now))
	})

	t.Run("continuous failures remain unavailable", func(t *testing.T) {
		probes := []*UserMonitorView{{
			PrimaryStatus:    MonitorStatusFailed,
			IntervalSeconds:  60,
			PrimaryCheckedAt: now,
			Timeline: []UserMonitorTimelinePoint{{
				Status:    MonitorStatusFailed,
				CheckedAt: now.Add(-60 * time.Second),
			}},
		}}

		require.Equal(t, MonitorV2StatusUnavailable, monitorV2GroupStatus(probes, now))
	})

	t.Run("expired success does not mask the latest failure", func(t *testing.T) {
		probes := []*UserMonitorView{{
			PrimaryStatus:    MonitorStatusFailed,
			IntervalSeconds:  60,
			PrimaryCheckedAt: now,
			Timeline: []UserMonitorTimelinePoint{{
				Status:    MonitorStatusOperational,
				CheckedAt: now.Add(-2*time.Minute - time.Second),
			}},
		}}

		require.Equal(t, MonitorV2StatusUnavailable, monitorV2GroupStatus(probes, now))
	})
}

func TestMonitorV2SnapshotPutsProFlagshipFirst(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	plusID := int64(1)
	proID := int64(2)
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{
			{ID: plusID, Name: "GPT-Plus", Platform: PlatformOpenAI, Status: StatusActive},
			{ID: proID, Name: "GPT-Pro", Platform: PlatformOpenAI, Status: StatusActive},
		}},
		&monitorV2ProbeReaderStub{views: []*UserMonitorView{
			{GroupID: &plusID, PrimaryModel: "gpt-5.6-sol", PrimaryStatus: MonitorStatusOperational, IntervalSeconds: 60, PrimaryCheckedAt: now},
			{GroupID: &proID, PrimaryModel: "gpt-5.6-sol", PrimaryStatus: MonitorStatusOperational, IntervalSeconds: 60, PrimaryCheckedAt: now},
		}},
		&monitorV2RepoStub{},
	)

	snapshot, err := svc.Snapshot(context.Background(), MonitorV2Window24H, now)

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 2)
	require.Equal(t, proID, snapshot.Groups[0].ID)
}

func TestMonitorV2SnapshotMatchesProbeByGroupIDBeforeLegacyName(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	groupID := int64(16)
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{
			ID:       16,
			Name:     "GPT-PLUS-内测",
			Platform: PlatformOpenAI,
			Status:   StatusActive,
		}}},
		&monitorV2ProbeReaderStub{views: []*UserMonitorView{{
			GroupID:          &groupID,
			GroupName:        "GPT PLUS 内测分组",
			PrimaryModel:     "gpt-5.6-sol",
			PrimaryStatus:    MonitorStatusOperational,
			IntervalSeconds:  60,
			PrimaryCheckedAt: now,
		}}},
		nil,
	)

	snapshot, err := svc.Snapshot(context.Background(), MonitorV2Window7D, now)

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, MonitorV2StatusOperational, snapshot.Groups[0].Status)
}

func TestMonitorV2ProbeTimelineMapsUnavailableToUnavailable(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	points := monitorV2ProbeTimeline([]*UserMonitorView{{
		Timeline: []UserMonitorTimelinePoint{{
			Status:    "unavailable",
			CheckedAt: now,
		}},
	}}, now.Add(-time.Minute), now)

	require.Len(t, points, 1)
	require.Equal(t, MonitorV2StatusUnavailable, points[0].Status)
	require.Nil(t, points[0].LatencyMS)
}

func TestMonitorV2SnapshotDoesNotFallbackFromHiddenStableGroupID(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	hiddenGroupID := int64(17)
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{
			{
				ID:          16,
				Name:        "同名分组",
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				IsExclusive: false,
			},
			{
				ID:          hiddenGroupID,
				Name:        "专属分组",
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				IsExclusive: true,
			},
		}},
		&monitorV2ProbeReaderStub{views: []*UserMonitorView{{
			GroupID:          &hiddenGroupID,
			GroupName:        "同名分组",
			PrimaryModel:     "private-model",
			PrimaryStatus:    MonitorStatusOperational,
			IntervalSeconds:  60,
			PrimaryCheckedAt: now,
		}}},
		nil,
	)

	snapshot, err := svc.Snapshot(context.Background(), MonitorV2Window7D, now)

	require.NoError(t, err)
	require.Empty(t, snapshot.Groups)
}

func TestMonitorV2SnapshotSelectsOnlyActivePublicGroups(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	groups := []Group{
		{
			ID:             1,
			Name:           "公开标准",
			Platform:       PlatformOpenAI,
			Status:         StatusActive,
			IsExclusive:    false,
			RateMultiplier: 0.2,
		},
		{
			ID:               2,
			Name:             "公开订阅",
			Platform:         PlatformOpenAI,
			Status:           StatusActive,
			IsExclusive:      false,
			SubscriptionType: SubscriptionTypeSubscription,
			RateMultiplier:   0.3,
		},
		{
			ID:          3,
			Name:        "专属",
			Platform:    PlatformOpenAI,
			Status:      StatusActive,
			IsExclusive: true,
		},
		{
			ID:          4,
			Name:        "已停用",
			Platform:    PlatformOpenAI,
			Status:      StatusDisabled,
			IsExclusive: false,
		},
	}
	probes := []*UserMonitorView{
		{
			ID:               21,
			Name:             "公开标准探针",
			GroupName:        " 公开标准 ",
			PrimaryModel:     "gpt-5.4",
			PrimaryStatus:    MonitorStatusOperational,
			IntervalSeconds:  60,
			PrimaryCheckedAt: now,
			Timeline:         []UserMonitorTimelinePoint{{Status: "operational", LatencyMs: intPtr(320), CheckedAt: now}},
		},
	}
	ttftP50 := 420
	ttftP95 := 880
	latencyP50 := 1320
	latencyP95 := 2400
	tps := 46.5

	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: groups},
		&monitorV2ProbeReaderStub{views: probes},
		&monitorV2RepoStub{performance: map[int64]MonitorV2PerformanceStats{
			1: {
				SampleCount: 12,
				TTFTP50MS:   &ttftP50, TTFTP95MS: &ttftP95,
				LatencyP50MS: &latencyP50, LatencyP95MS: &latencyP95,
				TPS: &tps,
			},
		}},
	)

	snapshot, err := svc.Snapshot(context.Background(), MonitorV2Window7D, now)
	require.NoError(t, err)
	require.Equal(t, MonitorV2ContractVersion, snapshot.ContractVersion)
	require.Equal(t, MonitorV2Window7D, snapshot.Window)
	require.Equal(t, now, snapshot.GeneratedAt)
	require.Len(t, snapshot.Groups, 1)

	first := snapshot.Groups[0]
	require.Equal(t, int64(1), first.ID)
	require.Equal(t, "公开标准", first.Name)
	require.InDelta(t, 0.2, first.RateMultiplier, 0.0001)
	require.Equal(t, MonitorV2StatusOperational, first.Status)
	require.Equal(t, MonitorV2MetricAvailable, first.TTFT.State)
	require.Equal(t, int64(12), first.TTFT.SampleCount)
	require.Equal(t, 420.0, *first.TTFT.Value)
	require.Equal(t, MonitorV2MetricAvailable, first.TTFTP95.State)
	require.Equal(t, int64(12), first.TTFTP95.SampleCount)
	require.Equal(t, 880.0, *first.TTFTP95.Value)
	require.Equal(t, MonitorV2MetricAvailable, first.LatencyP95.State)
	require.Equal(t, int64(12), first.LatencyP95.SampleCount)
	require.Equal(t, 2400.0, *first.LatencyP95.Value)
	require.Equal(t, MonitorV2MetricAvailable, first.TPS.State)
	require.Equal(t, int64(12), first.TPS.SampleCount)
	require.Equal(t, 46.5, *first.TPS.Value)
	require.Len(t, first.Timeline, 1)
	require.Equal(t, MonitorV2StatusOperational, first.Timeline[0].Status)
	require.Equal(t, 320, *first.Timeline[0].LatencyMS)

}

func TestMonitorV2SnapshotMatchesProbeForPublicGroupWithoutChannel(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{
			ID:          17,
			Name:        " 公开探针组 ",
			Platform:    PlatformOpenAI,
			Status:      StatusActive,
			IsExclusive: false,
		}}},
		&monitorV2ProbeReaderStub{views: []*UserMonitorView{{
			GroupName:        "公开探针组",
			PrimaryModel:     "gpt-5.4",
			PrimaryStatus:    MonitorStatusOperational,
			IntervalSeconds:  60,
			PrimaryCheckedAt: now,
		}}},
		&monitorV2RepoStub{},
	)

	snapshot, err := svc.Snapshot(context.Background(), MonitorV2Window7D, now)

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, MonitorV2StatusOperational, snapshot.Groups[0].Status)
}

func TestMonitorV2SnapshotRejectsInvalidWindowAndBoundsGroupCount(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{},
		&monitorV2ProbeReaderStub{},
		&monitorV2RepoStub{},
	)

	_, err := svc.Snapshot(context.Background(), MonitorV2Window("15d"), now)
	require.ErrorContains(t, err, "unsupported monitor window")

	groups := make([]Group, 101)
	for i := range groups {
		groups[i] = Group{
			ID:          int64(i + 1),
			Name:        "公开",
			Status:      StatusActive,
			IsExclusive: false,
		}
	}
	svc = NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: groups},
		&monitorV2ProbeReaderStub{},
		&monitorV2RepoStub{},
	)

	_, err = svc.Snapshot(context.Background(), MonitorV2Window24H, now)
	require.ErrorContains(t, err, "too many public groups")
}

func TestMonitorV2SnapshotBoundsTimelineAndStrings(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	group := Group{
		ID:          1,
		Name:        "公开标准",
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		IsExclusive: false,
	}

	t.Run("timeline", func(t *testing.T) {
		points := make([]UserMonitorTimelinePoint, 65)
		for i := range points {
			points[i] = UserMonitorTimelinePoint{Status: "operational", CheckedAt: now.Add(-time.Duration(65-i) * time.Minute)}
		}
		svc := NewMonitorV2Service(
			&monitorV2GroupRepoStub{groups: []Group{group}},
			&monitorV2ProbeReaderStub{views: []*UserMonitorView{{GroupID: int64Ptr(group.ID), Timeline: points}}},
			&monitorV2RepoStub{},
		)

		snapshot, err := svc.Snapshot(context.Background(), MonitorV2Window24H, now)
		require.NoError(t, err)
		require.Len(t, snapshot.Groups[0].Timeline, monitorV2MaxTimeline)
	})

	t.Run("group name", func(t *testing.T) {
		oversized := group
		oversized.Name = strings.Repeat("a", 257)
		svc := NewMonitorV2Service(
			&monitorV2GroupRepoStub{groups: []Group{oversized}},
			&monitorV2ProbeReaderStub{views: []*UserMonitorView{{GroupID: int64Ptr(oversized.ID)}}},
			&monitorV2RepoStub{},
		)

		_, err := svc.Snapshot(context.Background(), MonitorV2Window24H, now)
		require.ErrorContains(t, err, "group name too long")
	})
}
