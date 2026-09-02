package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

type monitorV4RefreshStoreStub struct {
	byWindow map[MonitorV4Window]MonitorV4StoredWindow
	loaded   []MonitorV4Window
	replaced []MonitorV4StoredWindow
	err      error
}

func (s *monitorV4RefreshStoreStub) LoadLatestMonitorV4Snapshot(_ context.Context, window MonitorV4Window) (MonitorV4StoredWindow, error) {
	s.loaded = append(s.loaded, window)
	if s.err != nil {
		return MonitorV4StoredWindow{}, s.err
	}
	row, ok := s.byWindow[window]
	if !ok {
		return MonitorV4StoredWindow{}, errors.New("missing snapshot")
	}
	return row, nil
}
func (s *monitorV4RefreshStoreStub) ReplaceMonitorV4Snapshots(_ context.Context, _ string, rows []MonitorV4StoredWindow) error {
	if s.err != nil {
		return s.err
	}
	s.replaced = append([]MonitorV4StoredWindow(nil), rows...)
	return nil
}

func TestMonitorV4SnapshotReadsPersistedProjectionWithoutNativeProjection(t *testing.T) {
	rate := 88.0
	generated := time.Date(2026, 8, 31, 4, 12, 0, 0, time.UTC)
	native := &monitorV4NativeReaderStub{}
	store := &monitorV4RefreshStoreStub{byWindow: map[MonitorV4Window]MonitorV4StoredWindow{
		MonitorV4Window7D: {Window: MonitorV4Window7D, SnapshotID: "snapshot-1", WindowStart: generated.Add(-7 * 24 * time.Hour), WindowEnd: generated, GeneratedAt: generated, ContractVersion: MonitorV4ContractVersion, Groups: map[int64]MonitorV4GroupProjection{7: {SuccessRate: &rate, RequestCount: 2, SuccessCount: 1, RealRequestCount: 2, RealSuccessCount: 1}}},
	}}
	svc := NewMonitorV4Service(&monitorV4GroupRepoStub{groups: []Group{{ID: 7, Name: "Hybrid", Status: StatusActive}}}, &monitorV4AvailableGroupReaderStub{}, native, nil, &monitorV4ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{GroupIDs: []int64{7}}})
	svc.SetSnapshotStore(store)
	snapshot, err := svc.Snapshot(context.Background(), 42, MonitorV4Window7D, generated.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.GeneratedAt.Equal(generated) {
		t.Fatalf("generated_at = %s, want %s", snapshot.GeneratedAt, generated)
	}
	if native.groupIDs != nil {
		t.Fatalf("native projection called with %v", native.groupIDs)
	}
	if snapshot.Groups[0].SuccessRate == nil || *snapshot.Groups[0].SuccessRate != rate {
		t.Fatalf("projection = %#v", snapshot.Groups[0])
	}
}

func TestMonitorV4RefreshUsesOneAsOfAndPublishesOnce(t *testing.T) {
	native := &monitorV4NativeReaderStub{projection: map[int64]MonitorV4GroupProjection{7: {RequestCount: 1, SuccessCount: 1, RealRequestCount: 1, RealSuccessCount: 1}}}
	store := &monitorV4RefreshStoreStub{}
	svc := NewMonitorV4Service(&monitorV4GroupRepoStub{groups: []Group{{ID: 7, Status: StatusActive}, {ID: 8, Status: "inactive"}}}, &monitorV4AvailableGroupReaderStub{}, native, nil, &monitorV4ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{GroupIDs: []int64{7}}})
	svc.SetSnapshotStore(store)
	if err := svc.RefreshMonitorV4Snapshots(context.Background(), time.Date(2026, 8, 31, 4, 12, 37, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if len(store.replaced) != 3 {
		t.Fatalf("replacement windows = %d, want 3", len(store.replaced))
	}
	if len(native.calls) != 3 {
		t.Fatalf("native calls = %d, want 3", len(native.calls))
	}
	for _, call := range native.calls {
		if !call.end.Equal(time.Date(2026, 8, 31, 4, 12, 0, 0, time.UTC)) {
			t.Fatalf("as_of = %s", call.end)
		}
		if len(call.groupIDs) != 1 || call.groupIDs[0] != 7 {
			t.Fatalf("group IDs = %v", call.groupIDs)
		}
	}
	if !native.calls[0].start.Equal(time.Date(2026, 8, 31, 3, 12, 0, 0, time.UTC)) || !native.calls[1].start.Equal(time.Date(2026, 8, 30, 4, 12, 0, 0, time.UTC)) || !native.calls[2].start.Equal(time.Date(2026, 8, 24, 4, 12, 0, 0, time.UTC)) {
		t.Fatalf("window starts = %#v", native.calls)
	}
	if len(store.replaced) != 3 || store.replaced[0].SnapshotID == "" || store.replaced[0].SnapshotID == "pending" || store.replaced[1].SnapshotID != store.replaced[0].SnapshotID || store.replaced[2].SnapshotID != store.replaced[0].SnapshotID {
		t.Fatalf("snapshot IDs = %#v", store.replaced)
	}
}

func TestMonitorV4SnapshotRejectsInvalidProjectionCounts(t *testing.T) {
	store := &monitorV4SnapshotStoreStub{loaded: MonitorV4StoredWindow{
		Window: MonitorV4Window7D, SnapshotID: "snapshot", WindowStart: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), WindowEnd: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), GeneratedAt: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), ContractVersion: MonitorV4ContractVersion,
		Groups: map[int64]MonitorV4GroupProjection{7: {RequestCount: 1, SuccessCount: 2}},
	}}
	svc := NewMonitorV4Service(&monitorV4GroupRepoStub{groups: []Group{{ID: 7, Status: StatusActive}}}, &monitorV4AvailableGroupReaderStub{}, nil, nil, &monitorV4ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{GroupIDs: []int64{7}}})
	svc.SetSnapshotStore(store)
	if _, err := svc.Snapshot(context.Background(), 42, MonitorV4Window7D, time.Now()); err == nil {
		t.Fatal("expected invalid count invariant error")
	}
}

func TestMonitorV4RefreshErrorDoesNotPublish(t *testing.T) {
	native := &monitorV4NativeReaderStub{err: errors.New("projection failed")}
	store := &monitorV4RefreshStoreStub{}
	svc := NewMonitorV4Service(&monitorV4GroupRepoStub{groups: []Group{{ID: 7, Status: StatusActive}}}, &monitorV4AvailableGroupReaderStub{}, native, nil, &monitorV4ConfiguredGroupReaderStub{})
	svc.SetSnapshotStore(store)
	if err := svc.RefreshMonitorV4Snapshots(context.Background(), time.Now()); err == nil {
		t.Fatal("expected refresh error")
	}
	if len(store.replaced) != 0 {
		t.Fatalf("replacement after projection error = %#v", store.replaced)
	}
}

func TestMonitorV4RefreshAllowsMissingProbeTerminalsWithoutSyntheticRequests(t *testing.T) {
	native := &monitorV4NativeReaderStub{projection: map[int64]MonitorV4GroupProjection{7: {RequestCount: 1, SuccessCount: 1, RealRequestCount: 1, RealSuccessCount: 1, ProbeFallbackBucketCount: 0, ProbeFallbackRequestCount: 0, MissingProbeTerminalCount: 1}}}
	store := &monitorV4RefreshStoreStub{}
	svc := NewMonitorV4Service(&monitorV4GroupRepoStub{groups: []Group{{ID: 7, Status: StatusActive}}}, &monitorV4AvailableGroupReaderStub{}, native, nil, &monitorV4ConfiguredGroupReaderStub{})
	svc.SetSnapshotStore(store)
	if err := svc.RefreshMonitorV4Snapshots(context.Background(), time.Now()); err != nil {
		t.Fatalf("expected missing probe terminal projection to be valid: %v", err)
	}
	if len(store.replaced) != 3 {
		t.Fatalf("replacement count = %d, want 3", len(store.replaced))
	}
}

func TestMonitorV4RefreshRejectsInconsistentRequestProjectionBeforePublish(t *testing.T) {
	native := &monitorV4NativeReaderStub{projection: map[int64]MonitorV4GroupProjection{7: {RequestCount: 1, SuccessCount: 1, RealRequestCount: 2, RealSuccessCount: 1}}}
	store := &monitorV4RefreshStoreStub{}
	svc := NewMonitorV4Service(&monitorV4GroupRepoStub{groups: []Group{{ID: 7, Status: StatusActive}}}, &monitorV4AvailableGroupReaderStub{}, native, nil, &monitorV4ConfiguredGroupReaderStub{})
	svc.SetSnapshotStore(store)
	if err := svc.RefreshMonitorV4Snapshots(context.Background(), time.Now()); err == nil {
		t.Fatal("expected inconsistent request projection error")
	}
	if len(store.replaced) != 0 {
		t.Fatalf("replacement after invalid projection = %#v", store.replaced)
	}
}

func TestMonitorV4RefreshStoreErrorDoesNotPublish(t *testing.T) {
	native := &monitorV4NativeReaderStub{projection: map[int64]MonitorV4GroupProjection{7: {RequestCount: 1, SuccessCount: 1, RealRequestCount: 1, RealSuccessCount: 1}}}
	store := &monitorV4RefreshStoreStub{err: errors.New("store failed")}
	svc := NewMonitorV4Service(&monitorV4GroupRepoStub{groups: []Group{{ID: 7, Status: StatusActive}}}, &monitorV4AvailableGroupReaderStub{}, native, nil, &monitorV4ConfiguredGroupReaderStub{})
	svc.SetSnapshotStore(store)
	if err := svc.RefreshMonitorV4Snapshots(context.Background(), time.Now()); err == nil {
		t.Fatal("expected store error")
	}
	if len(store.replaced) != 0 {
		t.Fatalf("successful publish after store error = %#v", store.replaced)
	}
}

type monitorV4ExclusiveAvailableStub struct{}

func (*monitorV4ExclusiveAvailableStub) GetAvailableGroups(context.Context, int64) ([]Group, error) {
	return nil, nil
}

func TestMonitorV4SnapshotHidesExclusiveGroupWithoutUserAvailability(t *testing.T) {
	generated := time.Date(2026, 8, 31, 4, 12, 0, 0, time.UTC)
	store := &monitorV4SnapshotStoreStub{loaded: MonitorV4StoredWindow{Window: MonitorV4Window7D, SnapshotID: "snapshot", WindowStart: generated.Add(-7 * 24 * time.Hour), WindowEnd: generated, GeneratedAt: generated, ContractVersion: MonitorV4ContractVersion, Groups: map[int64]MonitorV4GroupProjection{7: {RequestCount: 1, SuccessCount: 1, RealRequestCount: 1, RealSuccessCount: 1}}}}
	svc := NewMonitorV4Service(&monitorV4GroupRepoStub{groups: []Group{{ID: 7, Status: StatusActive, IsExclusive: true}}}, &monitorV4ExclusiveAvailableStub{}, nil, nil, &monitorV4ConfiguredGroupReaderStub{config: &ChannelMonitorV2Config{GroupIDs: []int64{7}}})
	svc.SetSnapshotStore(store)
	snapshot, err := svc.Snapshot(context.Background(), 42, MonitorV4Window7D, generated)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Groups) != 0 {
		t.Fatalf("exclusive groups exposed = %#v", snapshot.Groups)
	}
}

func TestMonitorV4SnapshotMissingStoreFailsClosed(t *testing.T) {
	svc := NewMonitorV4Service(&monitorV4GroupRepoStub{groups: []Group{{ID: 7, Status: StatusActive}}}, &monitorV4AvailableGroupReaderStub{}, nil, nil, &monitorV4ConfiguredGroupReaderStub{})
	if _, err := svc.Snapshot(context.Background(), 42, MonitorV4Window7D, time.Now()); err == nil {
		t.Fatal("expected missing store error")
	}
}
