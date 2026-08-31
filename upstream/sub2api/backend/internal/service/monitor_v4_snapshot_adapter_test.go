package service

import (
	"context"
	"testing"
)

type monitorV4SnapshotStoreStub struct {
	accountMonitorRepoStub
	loaded     MonitorV4StoredWindow
	loadErr    error
	replacedID string
	replaced   []MonitorV4StoredWindow
}

func (s *monitorV4SnapshotStoreStub) LoadLatestMonitorV4Snapshot(context.Context, MonitorV4Window) (MonitorV4StoredWindow, error) {
	return s.loaded, s.loadErr
}
func (s *monitorV4SnapshotStoreStub) ReplaceMonitorV4Snapshots(_ context.Context, snapshotID string, snapshots []MonitorV4StoredWindow) error {
	s.replacedID, s.replaced = snapshotID, snapshots
	return nil
}

func TestAccountMonitorV4SnapshotAdapterForwards(t *testing.T) {
	store := &monitorV4SnapshotStoreStub{}
	want := MonitorV4StoredWindow{Window: MonitorV4Window24H, SnapshotID: "snapshot"}
	store.loaded = want
	svc := NewAccountMonitorService(store, &accountMonitorAccountRepoStub{}, nil, nil, nil)
	got, err := svc.LoadLatestMonitorV4Snapshot(context.Background(), MonitorV4Window24H)
	if err != nil || got.SnapshotID != want.SnapshotID {
		t.Fatalf("load = %#v, %v", got, err)
	}
	if err := svc.ReplaceMonitorV4Snapshots(context.Background(), want.SnapshotID, []MonitorV4StoredWindow{want}); err != nil {
		t.Fatal(err)
	}
	if store.replacedID != want.SnapshotID || len(store.replaced) != 1 {
		t.Fatalf("replace args = %#v, %q", store.replaced, store.replacedID)
	}
}

func TestAccountMonitorV4SnapshotAdapterReturnsUnavailableWithoutStore(t *testing.T) {
	svc := NewAccountMonitorService(&accountMonitorRepoStub{}, &accountMonitorAccountRepoStub{}, nil, nil, nil)
	if _, err := svc.LoadLatestMonitorV4Snapshot(context.Background(), MonitorV4Window24H); err == nil || err.Error() != "account monitor v4 snapshot store unavailable" {
		t.Fatalf("load error = %v", err)
	}
	if err := svc.ReplaceMonitorV4Snapshots(context.Background(), "snapshot", nil); err == nil || err.Error() != "account monitor v4 snapshot store unavailable" {
		t.Fatalf("replace error = %v", err)
	}
}
