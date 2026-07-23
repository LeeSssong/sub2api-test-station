package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestTwoStoresBoundLockWaitAndPersistAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locking.db")
	first, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	tx, err := first.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(context.Background(), `INSERT INTO settings(key,value) VALUES('lock-holder','active')`); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}

	lockedCtx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := make(chan error, 1)
	go func() { result <- second.SetSetting(lockedCtx, "blocked-writer", "value") }()

	select {
	case err := <-result:
		elapsed := time.Since(started)
		if err == nil {
			_ = tx.Rollback()
			t.Fatal("locked writer unexpectedly succeeded")
		}
		if elapsed > 6500*time.Millisecond {
			_ = tx.Rollback()
			t.Fatalf("lock wait was not bounded: %s", elapsed)
		}
		t.Logf("locked writer failed after %s with 5s SQLite busy timeout", elapsed)
	case <-time.After(7 * time.Second):
		_ = tx.Rollback()
		<-result
		t.Fatal("lock wait exceeded SQLite busy-timeout budget")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}

	if err := second.SetSetting(context.Background(), "after-lock", "persisted"); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, err := reopened.GetSetting(context.Background(), "after-lock"); err != nil || got != "persisted" {
		t.Fatalf("reopened value=%q err=%v", got, err)
	}
}
