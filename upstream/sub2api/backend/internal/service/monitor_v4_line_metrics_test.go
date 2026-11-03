package service

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type lineMetricsReaderStub struct {
	MonitorV4ProjectionReader
	rows       []MonitorV4LineMetric
	err        error
	ids        []int64
	start, end time.Time
	deadline   time.Duration
}

func (s *lineMetricsReaderStub) ReadMonitorV4LineMetrics(ctx context.Context, ids []int64, start, end time.Time) ([]MonitorV4LineMetric, error) {
	s.ids, s.start, s.end = ids, start, end
	if deadline, ok := ctx.Deadline(); ok {
		s.deadline = time.Until(deadline)
	}
	return s.rows, s.err
}

func lineMetricsService(native MonitorV4ProjectionReader) (*MonitorV4Service, *monitorV4RefreshStoreStub) {
	end := time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)
	store := &monitorV4RefreshStoreStub{byWindow: map[MonitorV4Window]MonitorV4StoredWindow{
		MonitorV4Window24H: {Window: MonitorV4Window24H, SnapshotID: "snapshot", WindowStart: end.Add(-24 * time.Hour), WindowEnd: end, GeneratedAt: end.Add(7 * time.Minute), ContractVersion: MonitorV4ContractVersion},
	}}
	svc := NewMonitorV4Service(&monitorV4GroupRepoStub{groups: []Group{{ID: 7, Status: StatusActive}, {ID: 99, Status: StatusActive}}}, &monitorV4AvailableGroupReaderStub{}, native, nil, &monitorV4ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{GroupIDs: []int64{7}}})
	svc.SetSnapshotStore(store)
	return svc, store
}

func TestMonitorV4LineSelectionExactBoundsVisibilityAndZero(t *testing.T) {
	zero := 0.0
	native := &lineMetricsReaderStub{rows: []MonitorV4LineMetric{{ID: 7, SLARequestCount: 4, SLASuccessCount: 3, TTFTP50MS: &zero, TTFTSampleCount: 2}}}
	svc, store := lineMetricsService(native)
	value, err := svc.LineSelection(context.Background(), 42, time.Now())
	require.NoError(t, err)
	require.Equal(t, []MonitorV4Window{MonitorV4Window24H}, store.loaded)
	require.Equal(t, []int64{7}, native.ids)
	stored := store.byWindow[MonitorV4Window24H]
	require.Equal(t, stored.WindowStart, native.start)
	require.Equal(t, stored.WindowEnd, native.end)
	require.Equal(t, stored.GeneratedAt, value.GeneratedAt)
	require.Equal(t, stored.WindowStart, value.WindowStart)
	require.Equal(t, stored.WindowEnd, value.WindowEnd)
	require.Equal(t, MonitorV4Window24H, value.Window)
	require.Equal(t, "1", value.ContractVersion)
	require.Equal(t, 0.0, *value.Groups[0].TTFTP50MS)
	require.Greater(t, native.deadline, time.Duration(0))
	require.LessOrEqual(t, native.deadline, 10*time.Second)
}

func TestMonitorV4LineSelectionFailsClosed(t *testing.T) {
	ptr := func(v float64) *float64 { return &v }
	for _, tc := range []struct {
		name string
		rows []MonitorV4LineMetric
	}{
		{"unexpected group", []MonitorV4LineMetric{{ID: 99}}},
		{"missing group", nil},
		{"duplicate", []MonitorV4LineMetric{{ID: 7}, {ID: 7}}},
		{"negative count", []MonitorV4LineMetric{{ID: 7, SLARequestCount: -1}}},
		{"success over requests", []MonitorV4LineMetric{{ID: 7, SLARequestCount: 1, SLASuccessCount: 2}}},
		{"samples over success", []MonitorV4LineMetric{{ID: 7, TTFTSampleCount: 1, TTFTP50MS: ptr(1)}}},
		{"null with samples", []MonitorV4LineMetric{{ID: 7, SLARequestCount: 1, SLASuccessCount: 1, TTFTSampleCount: 1}}},
		{"value without samples", []MonitorV4LineMetric{{ID: 7, TTFTP50MS: ptr(0)}}},
		{"negative metric", []MonitorV4LineMetric{{ID: 7, SLARequestCount: 1, SLASuccessCount: 1, TTFTSampleCount: 1, TTFTP50MS: ptr(-1)}}},
		{"nan", []MonitorV4LineMetric{{ID: 7, SLARequestCount: 1, SLASuccessCount: 1, TTFTSampleCount: 1, TTFTP50MS: ptr(math.NaN())}}},
		{"infinity", []MonitorV4LineMetric{{ID: 7, SLARequestCount: 1, SLASuccessCount: 1, TTFTSampleCount: 1, TTFTP50MS: ptr(math.Inf(1))}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := lineMetricsService(&lineMetricsReaderStub{rows: tc.rows})
			value, err := svc.LineSelection(context.Background(), 42, time.Now())
			require.Error(t, err)
			require.Nil(t, value)
		})
	}
	svc, _ := lineMetricsService(&monitorV4NativeReaderStub{})
	_, err := svc.LineSelection(context.Background(), 42, time.Now())
	require.Error(t, err)
	svc, store := lineMetricsService(&lineMetricsReaderStub{err: errors.New("query failed")})
	_, err = svc.LineSelection(context.Background(), 42, time.Now())
	require.ErrorContains(t, err, "query failed")
	store.err = errors.New("snapshot failed")
	_, err = svc.LineSelection(context.Background(), 42, time.Now())
	require.ErrorContains(t, err, "snapshot failed")
	store.err = nil
	row := store.byWindow[MonitorV4Window24H]
	row.WindowStart = row.WindowEnd.Add(-time.Hour)
	store.byWindow[MonitorV4Window24H] = row
	_, err = svc.LineSelection(context.Background(), 42, time.Now())
	require.ErrorContains(t, err, "bounds")
	_, err = svc.LineSelection(context.Background(), 0, time.Now())
	require.Error(t, err)
}

func TestMonitorV4LineSelectionNoSamplesAndEmptyVisibility(t *testing.T) {
	svc, _ := lineMetricsService(&lineMetricsReaderStub{rows: []MonitorV4LineMetric{{ID: 7}}})
	value, err := svc.LineSelection(context.Background(), 42, time.Now())
	require.NoError(t, err)
	require.Nil(t, value.Groups[0].TTFTP50MS)
	require.Zero(t, value.Groups[0].TTFTSampleCount)
	svc.groupRepo = &monitorV4GroupRepoStub{}
	svc.native = &lineMetricsReaderStub{rows: []MonitorV4LineMetric{}}
	value, err = svc.LineSelection(context.Background(), 42, time.Now())
	require.NoError(t, err)
	require.NotNil(t, value.Groups)
	require.Empty(t, value.Groups)
}

func TestMonitorV4LineSelectionPreservesEntitledAndLinkedGroups(t *testing.T) {
	native := &lineMetricsReaderStub{rows: []MonitorV4LineMetric{{ID: 7}, {ID: 8}, {ID: 9}}}
	svc, _ := lineMetricsService(native)
	svc.groupRepo = &monitorV4GroupRepoStub{groups: []Group{
		{ID: 7, Status: StatusActive}, {ID: 8, Status: StatusActive, IsExclusive: true},
		{ID: 99, Status: StatusActive, IsExclusive: true},
	}}
	svc.configured = &monitorV4ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{GroupIDs: []int64{7, 8, 99}}}
	svc.available = linkedAvailableStub{active: []Group{{ID: 8, Status: StatusActive, IsExclusive: true}}, linked: []Group{{ID: 9, Status: "disabled"}}}
	value, err := svc.LineSelection(context.Background(), 42, time.Now())
	require.NoError(t, err)
	require.Equal(t, []int64{7, 8, 9}, native.ids)
	require.Len(t, value.Groups, 3)
}

type lineMetricsAccountRepoStub struct {
	AccountMonitorRepository
	*lineMetricsReaderStub
}

func TestAccountMonitorLineMetricsAdapter(t *testing.T) {
	start := time.Date(2026, 11, 2, 4, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	reader := &lineMetricsReaderStub{rows: []MonitorV4LineMetric{{ID: 7}}}
	svc := &AccountMonitorService{repo: &lineMetricsAccountRepoStub{lineMetricsReaderStub: reader}}
	rows, err := svc.ReadMonitorV4LineMetrics(context.Background(), []int64{7}, start, end)
	require.NoError(t, err)
	require.Equal(t, reader.rows, rows)
	require.Equal(t, start, reader.start)
	require.Equal(t, end, reader.end)
	svc.repo = nil
	_, err = svc.ReadMonitorV4LineMetrics(context.Background(), []int64{7}, start, end)
	require.Error(t, err)
	svc = nil
	_, err = svc.ReadMonitorV4LineMetrics(context.Background(), []int64{7}, start, end)
	require.Error(t, err)
}
