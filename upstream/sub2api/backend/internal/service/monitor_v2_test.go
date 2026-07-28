package service

import (
	"context"
	"fmt"
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

type monitorV2ChannelReaderStub struct {
	channels []AvailableChannel
	err      error
}

func (s *monitorV2ChannelReaderStub) ListAvailable(context.Context) ([]AvailableChannel, error) {
	return append([]AvailableChannel(nil), s.channels...), s.err
}

type monitorV2ProbeReaderStub struct {
	views []*UserMonitorView
	err   error
}

func (s *monitorV2ProbeReaderStub) ListUserView(context.Context) ([]*UserMonitorView, error) {
	return append([]*UserMonitorView(nil), s.views...), s.err
}

type monitorV2OpsReaderStub struct {
	overview        *OpsDashboardOverview
	throughputTrend *OpsThroughputTrendResponse
	tokenStats      *OpsOpenAITokenStatsResponse
}

func (s *monitorV2OpsReaderStub) GetDashboardOverview(
	_ context.Context,
	filter *OpsDashboardFilter,
) (*OpsDashboardOverview, error) {
	if s.overview != nil {
		return s.overview, nil
	}
	success := int64(9842)
	slaErrors := int64(68)
	if filter.GroupID != nil && *filter.GroupID == 2 {
		success = 0
		slaErrors = 0
	}
	ttftP50 := 420
	ttftP95 := 880
	durationP50 := 1320
	durationP95 := 2400
	return &OpsDashboardOverview{
		SuccessCount:    success,
		ErrorCountSLA:   slaErrors,
		RequestCountSLA: success + slaErrors,
		Duration: OpsPercentiles{
			P50: &durationP50,
			P95: &durationP95,
		},
		TTFT: OpsPercentiles{
			P50:         &ttftP50,
			P95:         &ttftP95,
			SampleCount: success,
		},
	}, nil
}

func (s *monitorV2OpsReaderStub) GetThroughputTrend(
	_ context.Context,
	filter *OpsDashboardFilter,
	_ int,
) (*OpsThroughputTrendResponse, error) {
	if s.throughputTrend != nil {
		return s.throughputTrend, nil
	}
	if filter.GroupID != nil && *filter.GroupID == 2 {
		return &OpsThroughputTrendResponse{Points: []*OpsThroughputTrendPoint{}}, nil
	}
	return &OpsThroughputTrendResponse{
		Points: []*OpsThroughputTrendPoint{
			{BucketStart: filter.StartTime, RequestCount: 12},
		},
	}, nil
}

func (s *monitorV2OpsReaderStub) GetErrorTrend(
	_ context.Context,
	filter *OpsDashboardFilter,
	_ int,
) (*OpsErrorTrendResponse, error) {
	if filter.GroupID != nil && *filter.GroupID == 2 {
		return &OpsErrorTrendResponse{Points: []*OpsErrorTrendPoint{}}, nil
	}
	return &OpsErrorTrendResponse{
		Points: []*OpsErrorTrendPoint{
			{BucketStart: filter.StartTime, ErrorCountSLA: 1},
		},
	}, nil
}

func (s *monitorV2OpsReaderStub) GetOpenAITokenStats(
	_ context.Context,
	filter *OpsOpenAITokenStatsFilter,
) (*OpsOpenAITokenStatsResponse, error) {
	if s.tokenStats != nil {
		return s.tokenStats, nil
	}
	if filter.GroupID != nil && *filter.GroupID == 2 {
		return &OpsOpenAITokenStatsResponse{Items: []*OpsOpenAITokenStatsItem{}}, nil
	}
	tps := 46.5
	return &OpsOpenAITokenStatsResponse{
		Items: []*OpsOpenAITokenStatsItem{
			{
				Model:           "gpt-5.4",
				RequestCount:    12,
				TPSSampleCount:  12,
				AvgTokensPerSec: &tps,
			},
		},
	}, nil
}

type monitorV2RepoStub struct {
	stats map[int64]MonitorV2CacheStats
}

func (s *monitorV2RepoStub) GetCacheStats(
	context.Context,
	[]int64,
	time.Time,
	time.Time,
) (map[int64]MonitorV2CacheStats, error) {
	out := make(map[int64]MonitorV2CacheStats, len(s.stats))
	for id, stat := range s.stats {
		out[id] = stat
	}
	return out, nil
}

func TestMonitorV2MetricsUseOnlyNativeEligibleSamples(t *testing.T) {
	t.Run("ttft ignores ordinary successes without first-token evidence", func(t *testing.T) {
		ttftP50 := 420
		ttftP95 := 880
		svc := NewMonitorV2Service(
			&monitorV2GroupRepoStub{groups: []Group{{
				ID:          1,
				Name:        "公开标准",
				Platform:    PlatformOpenAI,
				Status:      StatusActive,
				IsExclusive: false,
			}}},
			&monitorV2ChannelReaderStub{},
			&monitorV2ProbeReaderStub{},
			&monitorV2OpsReaderStub{overview: &OpsDashboardOverview{
				SuccessCount:    100,
				RequestCountSLA: 100,
				TTFT: OpsPercentiles{
					P50:         &ttftP50,
					P95:         &ttftP95,
					SampleCount: 3,
				},
			}},
			&monitorV2RepoStub{},
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
	})

	t.Run("tps weights only requests with throughput evidence", func(t *testing.T) {
		fastTPS := 100.0
		slowTPS := 10.0

		metric := monitorV2TPSMetric(&OpsOpenAITokenStatsResponse{
			Items: []*OpsOpenAITokenStatsItem{
				{
					Model:           "gpt-fast",
					RequestCount:    100,
					TPSSampleCount:  1,
					AvgTokensPerSec: &fastTPS,
				},
				{
					Model:           "gpt-slow",
					RequestCount:    4,
					TPSSampleCount:  4,
					AvgTokensPerSec: &slowTPS,
				},
			},
		})

		require.Equal(t, MonitorV2MetricAvailable, metric.State)
		require.Equal(t, int64(5), metric.SampleCount)
		require.NotNil(t, metric.Value)
		require.InDelta(t, 28.0, *metric.Value, 0.0001)
	})

	t.Run("unsupported cache evidence is not reported as zero percent", func(t *testing.T) {
		metric := monitorV2CacheMetric(MonitorV2CacheStats{
			EvidenceAvailable: false,
			RequestCount:      20,
			HitCount:          0,
		})

		require.Equal(t, MonitorV2MetricNotProvided, metric.State)
		require.Equal(t, int64(0), metric.SampleCount)
		require.Nil(t, metric.Value)
	})

	t.Run("supported cache evidence can report an all-miss sample", func(t *testing.T) {
		metric := monitorV2CacheMetric(MonitorV2CacheStats{
			EvidenceAvailable: true,
			RequestCount:      5,
			HitCount:          0,
		})

		require.Equal(t, MonitorV2MetricAvailable, metric.State)
		require.Equal(t, int64(5), metric.SampleCount)
		require.NotNil(t, metric.Value)
		require.Equal(t, 0.0, *metric.Value)
	})
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
	channels := []AvailableChannel{
		{
			ID:     11,
			Name:   "公开渠道",
			Status: StatusActive,
			Groups: []AvailableGroupRef{
				{ID: 1, Name: "公开标准"},
			},
			SupportedModels: []SupportedModel{
				{Name: "gpt-5.4", Platform: PlatformOpenAI},
			},
		},
	}
	probes := []*UserMonitorView{
		{
			ID:            21,
			Name:          "公开标准探针",
			GroupName:     " 公开标准 ",
			PrimaryModel:  "gpt-5.4",
			PrimaryStatus: MonitorStatusOperational,
		},
	}

	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: groups},
		&monitorV2ChannelReaderStub{channels: channels},
		&monitorV2ProbeReaderStub{views: probes},
		&monitorV2OpsReaderStub{},
		&monitorV2RepoStub{stats: map[int64]MonitorV2CacheStats{
			1: {EvidenceAvailable: true, RequestCount: 20, HitCount: 8},
		}},
	)

	snapshot, err := svc.Snapshot(context.Background(), MonitorV2Window7D, now)
	require.NoError(t, err)
	require.Equal(t, MonitorV2ContractVersion, snapshot.ContractVersion)
	require.Equal(t, MonitorV2Window7D, snapshot.Window)
	require.Equal(t, now, snapshot.GeneratedAt)
	require.Len(t, snapshot.Groups, 2)

	first := snapshot.Groups[0]
	require.Equal(t, int64(1), first.ID)
	require.Equal(t, "公开标准", first.Name)
	require.InDelta(t, 0.2, first.RateMultiplier, 0.0001)
	require.Equal(t, MonitorV2StatusOperational, first.Status)
	require.Equal(t, int64(9842), first.Availability.SuccessCount)
	require.Equal(t, int64(9910), first.Availability.EligibleCount)
	require.NotNil(t, first.Availability.Value)
	require.InDelta(t, 99.3138, *first.Availability.Value, 0.0001)
	require.Equal(t, MonitorV2MetricAvailable, first.TTFT.State)
	require.Equal(t, int64(9842), first.TTFT.SampleCount)
	require.Equal(t, 420.0, *first.TTFT.Value)
	require.Equal(t, MonitorV2MetricAvailable, first.TTFTP95.State)
	require.Equal(t, int64(9842), first.TTFTP95.SampleCount)
	require.Equal(t, 880.0, *first.TTFTP95.Value)
	require.Equal(t, MonitorV2MetricAvailable, first.LatencyP95.State)
	require.Equal(t, int64(9842), first.LatencyP95.SampleCount)
	require.Equal(t, 2400.0, *first.LatencyP95.Value)
	require.Equal(t, MonitorV2MetricAvailable, first.TPS.State)
	require.Equal(t, int64(12), first.TPS.SampleCount)
	require.Equal(t, 46.5, *first.TPS.Value)
	require.Equal(t, MonitorV2MetricAvailable, first.CacheHit.State)
	require.Equal(t, int64(20), first.CacheHit.SampleCount)
	require.Equal(t, 40.0, *first.CacheHit.Value)
	require.Equal(t, []MonitorV2Model{{Name: "gpt-5.4", Status: MonitorStatusOperational}}, first.Models)
	require.Len(t, first.Timeline, 1)
	require.Equal(t, int64(12), first.Timeline[0].SuccessCount)
	require.Equal(t, int64(13), first.Timeline[0].EligibleCount)

	second := snapshot.Groups[1]
	require.Equal(t, int64(2), second.ID)
	require.InDelta(t, 0.3, second.RateMultiplier, 0.0001)
	require.Equal(t, MonitorV2StatusUnconfigured, second.Status)
	require.Equal(t, MonitorV2MetricInsufficientData, second.TTFT.State)
	require.Equal(t, MonitorV2MetricInsufficientData, second.Availability.State)
	require.Nil(t, second.Availability.Value)
}

func TestMonitorV2SnapshotPublishesProbeModelsForMatchingPublicGroupWithoutChannel(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{groups: []Group{{
			ID:          17,
			Name:        " 公开探针组 ",
			Platform:    PlatformOpenAI,
			Status:      StatusActive,
			IsExclusive: false,
		}}},
		&monitorV2ChannelReaderStub{},
		&monitorV2ProbeReaderStub{views: []*UserMonitorView{{
			GroupName:     "公开探针组",
			PrimaryModel:  "gpt-5.4",
			PrimaryStatus: MonitorStatusOperational,
		}}},
		nil,
		&monitorV2RepoStub{},
	)

	snapshot, err := svc.Snapshot(context.Background(), MonitorV2Window7D, now)

	require.NoError(t, err)
	require.Len(t, snapshot.Groups, 1)
	require.Equal(t, MonitorV2StatusOperational, snapshot.Groups[0].Status)
	require.Equal(t, []MonitorV2Model{{
		Name:   "gpt-5.4",
		Status: MonitorStatusOperational,
	}}, snapshot.Groups[0].Models)
}

func TestMonitorV2SnapshotRejectsInvalidWindowAndBoundsGroupCount(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	svc := NewMonitorV2Service(
		&monitorV2GroupRepoStub{},
		&monitorV2ChannelReaderStub{},
		&monitorV2ProbeReaderStub{},
		&monitorV2OpsReaderStub{},
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
		&monitorV2ChannelReaderStub{},
		&monitorV2ProbeReaderStub{},
		&monitorV2OpsReaderStub{},
		&monitorV2RepoStub{},
	)

	_, err = svc.Snapshot(context.Background(), MonitorV2Window24H, now)
	require.ErrorContains(t, err, "too many public groups")
}

func TestMonitorV2SnapshotBoundsModelsTimelineAndStrings(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	group := Group{
		ID:          1,
		Name:        "公开标准",
		Platform:    PlatformOpenAI,
		Status:      StatusActive,
		IsExclusive: false,
	}

	t.Run("models", func(t *testing.T) {
		models := make([]SupportedModel, 201)
		for i := range models {
			models[i] = SupportedModel{Name: fmt.Sprintf("gpt-%03d", i)}
		}
		svc := NewMonitorV2Service(
			&monitorV2GroupRepoStub{groups: []Group{group}},
			&monitorV2ChannelReaderStub{channels: []AvailableChannel{{
				Status:          StatusActive,
				Groups:          []AvailableGroupRef{{ID: group.ID, Name: group.Name}},
				SupportedModels: models,
			}}},
			&monitorV2ProbeReaderStub{},
			nil,
			&monitorV2RepoStub{},
		)

		_, err := svc.Snapshot(context.Background(), MonitorV2Window24H, now)
		require.ErrorContains(t, err, "too many models")
	})

	t.Run("timeline", func(t *testing.T) {
		points := make([]*OpsThroughputTrendPoint, 65)
		for i := range points {
			points[i] = &OpsThroughputTrendPoint{
				BucketStart:  now.Add(time.Duration(i) * time.Minute),
				RequestCount: 1,
			}
		}
		svc := NewMonitorV2Service(
			&monitorV2GroupRepoStub{groups: []Group{group}},
			&monitorV2ChannelReaderStub{},
			&monitorV2ProbeReaderStub{},
			&monitorV2OpsReaderStub{
				throughputTrend: &OpsThroughputTrendResponse{Points: points},
			},
			&monitorV2RepoStub{},
		)

		_, err := svc.Snapshot(context.Background(), MonitorV2Window24H, now)
		require.ErrorContains(t, err, "too many timeline points")
	})

	t.Run("group name", func(t *testing.T) {
		oversized := group
		oversized.Name = strings.Repeat("a", 257)
		svc := NewMonitorV2Service(
			&monitorV2GroupRepoStub{groups: []Group{oversized}},
			&monitorV2ChannelReaderStub{},
			&monitorV2ProbeReaderStub{},
			nil,
			&monitorV2RepoStub{},
		)

		_, err := svc.Snapshot(context.Background(), MonitorV2Window24H, now)
		require.ErrorContains(t, err, "group name too long")
	})
}
