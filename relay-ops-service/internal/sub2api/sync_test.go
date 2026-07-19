package sub2api

import (
	"context"
	"testing"
	"time"
)

func TestSynchronizerKeepsOnlyCustomerVisibleGroupsAndNativeReferences(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{
		channels: []Channel{{ID: 7, Name: "GPT", Status: "active", GroupIDs: []int64{3, 4}}},
		groups: []Group{
			{ID: 3, Name: "GPT-Pro", Platform: "openai", RateMultiplier: 1, Status: "active"},
			{ID: 4, Name: "private", Platform: "openai", RateMultiplier: 0.5, IsExclusive: true, Status: "active"},
		},
		monitors: []ChannelMonitor{{ID: 9, Name: "GPT-Pro", GroupName: "GPT-Pro", Enabled: true, PrimaryModel: "gpt-5.6-sol"}},
		ops:      OpsSnapshot{GeneratedAt: "2026-07-19T08:00:00Z", Overview: OpsOverview{StartTime: "2026-07-18T08:00:00Z", EndTime: "2026-07-19T08:00:00Z", SLA: 99, TTFT: Percentiles{P95MS: 1400}}},
		history:  []MonitorHistory{{ID: 11, Model: "gpt-5.6-sol", Status: "operational", CheckedAt: "2026-07-19T08:00:00Z"}},
	}
	sink := &fakeSink{}
	syncer := Synchronizer{Reader: reader, Sink: sink, Now: func() time.Time { return time.Date(2026, 7, 19, 8, 5, 0, 0, time.UTC) }}
	if err := syncer.Sync(context.Background()); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(sink.groups) != 1 {
		t.Fatalf("groups = %#v", sink.groups)
	}
	group := sink.groups[0]
	if group.GroupID != 3 || len(group.ChannelIDs) != 1 || group.ChannelIDs[0] != 7 || len(group.MonitorIDs) != 1 || group.MonitorIDs[0] != 9 {
		t.Fatalf("group = %#v", group)
	}
	if group.HealthGate != "pending" || group.SourceRevision == "" {
		t.Fatalf("qualification fields = %#v", group)
	}
	if len(sink.refs) != 2 {
		t.Fatalf("metric refs = %#v", sink.refs)
	}
	for _, ref := range sink.refs {
		if ref.PayloadHash == "" || ref.WindowStart.IsZero() || ref.WindowEnd.IsZero() || ref.SchemaVersion != NativeMetricSchemaV1 {
			t.Fatalf("invalid metric ref: %#v", ref)
		}
	}
}

type fakeReader struct {
	channels []Channel
	groups   []Group
	monitors []ChannelMonitor
	ops      OpsSnapshot
	history  []MonitorHistory
}

func (f *fakeReader) ListChannels(context.Context) ([]Channel, error) { return f.channels, nil }
func (f *fakeReader) ListGroups(context.Context) ([]Group, error)     { return f.groups, nil }
func (f *fakeReader) ListChannelMonitors(context.Context) ([]ChannelMonitor, error) {
	return f.monitors, nil
}
func (f *fakeReader) GetChannelMonitorHistory(context.Context, int64, string, int) ([]MonitorHistory, error) {
	return f.history, nil
}
func (f *fakeReader) GetOpsSnapshot(context.Context, OpsQuery) (OpsSnapshot, error) {
	return f.ops, nil
}
func (f *fakeReader) GetUsageStats(context.Context, UsageQuery) (UsageStats, error) {
	return UsageStats{}, nil
}

type fakeSink struct {
	groups []PublicGroupRecord
	refs   []MetricRef
}

func (f *fakeSink) UpsertPublicGroup(_ context.Context, group PublicGroupRecord) error {
	f.groups = append(f.groups, group)
	return nil
}

func (f *fakeSink) AppendMetricRef(_ context.Context, ref MetricRef) error {
	f.refs = append(f.refs, ref)
	return nil
}
