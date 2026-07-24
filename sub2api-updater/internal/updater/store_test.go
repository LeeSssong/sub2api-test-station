package updater

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sampleOperation() Operation {
	return Operation{SchemaVersion: 1, OperationID: "op-1", ActorID: 7, Mode: "schedule", TargetVersion: "1.2.3", Image: "weishaw/sub2api:1.2.3@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ScheduledAt: time.Date(2026, 7, 25, 2, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 7, 25, 1, 0, 0, 0, time.UTC), Stage: "scheduled"}
}

func TestStoreSavesAtomicallyWithPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	store := NewStore(path)
	want := sampleOperation()
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("permissions = %o, want 0600", got)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.OperationID != want.OperationID || got.Image != want.Image {
		t.Fatalf("loaded operation = %#v", got)
	}
}

func TestStoreFailsClosedForCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewStore(path).Load()
	if !errors.Is(err, ErrCorruptState) {
		t.Fatalf("error = %v, want ErrCorruptState", err)
	}
}
