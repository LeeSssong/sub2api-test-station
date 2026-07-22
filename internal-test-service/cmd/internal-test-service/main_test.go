package main

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunBackupCommandCreatesSQLiteSnapshotWithoutServiceConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	destinationPath := filepath.Join(dir, "backup.db")
	db, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(context.Background(), `CREATE TABLE users(id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	handled, exitCode := runBackupCommand(
		[]string{"backup-sqlite", sourcePath, destinationPath},
		func(string) string { return "" },
		&stderr,
	)

	if !handled || exitCode != 0 || stderr.Len() != 0 {
		t.Fatalf("handled=%v exit=%d stderr=%q", handled, exitCode, stderr.String())
	}
	copyDB, err := sql.Open("sqlite", destinationPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	var count int
	if err := copyDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("users table count=%d", count)
	}
}
