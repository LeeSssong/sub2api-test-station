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
		MonitorV4Window7D: {Window: MonitorV4Window7D, SnapshotID: "snapshot-1", WindowStart: generated.Add(-7 * 24 * time.Hour), WindowEnd: generated, GeneratedAt: generated, ContractVersion: MonitorV4ContractVersion, Groups: map[int64]MonitorV4GroupProjection{7: {SuccessRate: &rate, RequestCount: 2, SuccessCount: 1}}},
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
	native := &monitorV4NativeReaderStub{projection: map[int64]MonitorV4GroupProjection{7: {RequestCount: 1}}}
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
}
