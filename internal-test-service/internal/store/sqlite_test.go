package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"example.invalid/internal-test-service/internal/domain"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestBackupSQLiteIncludesCommittedWALState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	destinationPath := filepath.Join(dir, "backup.db")
	db, err := sql.Open("sqlite", sourcePath+"?_pragma=journal_mode(WAL)&_pragma=wal_autocheckpoint(0)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE TABLE accounts(id INTEGER PRIMARY KEY, email TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO accounts(email) VALUES('launch@example.test')`); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sourcePath + "-wal"); err != nil {
		t.Fatalf("expected live WAL file: %v", err)
	}

	if err := BackupSQLite(ctx, sourcePath, destinationPath); err != nil {
		t.Fatal(err)
	}
	copyDB, err := sql.Open("sqlite", destinationPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer copyDB.Close()
	var integrity, email string
	if err := copyDB.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if err := copyDB.QueryRowContext(ctx, `SELECT email FROM accounts WHERE id = 1`).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" || email != "launch@example.test" {
		t.Fatalf("integrity=%q email=%q", integrity, email)
	}
}

func TestBackupSQLiteRefusesExistingDestination(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	destinationPath := filepath.Join(dir, "backup.db")
	if err := os.WriteFile(sourcePath, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destinationPath, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := BackupSQLite(context.Background(), sourcePath, destinationPath)
	if err == nil || err.Error() != "backup destination already exists" {
		t.Fatalf("err=%v", err)
	}
	contents, err := os.ReadFile(destinationPath)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("contents=%q err=%v", contents, err)
	}
}

func TestStorePersistsAndDeduplicatesGrants(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	ok, err := s.CreateGrant(ctx, Grant{UserID: 1, Kind: domain.GrantCheckin, Amount: domain.CheckinGrant, GrantDate: sql.NullString{String: "2026-07-19", Valid: true}, IdempotencyKey: "d04-checkin-1-2026-07-19", Status: domain.TaskSucceeded, CreatedAt: now})
	if err != nil || !ok {
		t.Fatalf("first grant: %v %v", ok, err)
	}
	ok, err = s.CreateGrant(ctx, Grant{UserID: 1, Kind: domain.GrantCheckin, Amount: domain.CheckinGrant, GrantDate: sql.NullString{String: "2026-07-19", Valid: true}, IdempotencyKey: "duplicate", Status: domain.TaskSucceeded, CreatedAt: now})
	if err != nil || ok {
		t.Fatalf("duplicate date accepted: %v %v", ok, err)
	}
	ok, err = s.CreateGrant(ctx, Grant{UserID: 1, Kind: domain.GrantReferral, Amount: domain.ReferralGrant, InviteeUserID: sql.NullInt64{Int64: 2, Valid: true}, IdempotencyKey: "d04-referral-2", Status: domain.TaskSucceeded, CreatedAt: now})
	if err != nil || !ok {
		t.Fatalf("referral: %v %v", ok, err)
	}
	ok, err = s.CreateGrant(ctx, Grant{UserID: 1, Kind: domain.GrantReferral, Amount: domain.ReferralGrant, InviteeUserID: sql.NullInt64{Int64: 2, Valid: true}, IdempotencyKey: "different", Status: domain.TaskSucceeded, CreatedAt: now})
	if err != nil || ok {
		t.Fatalf("duplicate invitee accepted: %v %v", ok, err)
	}
	ok, err = s.CreateGrant(ctx, Grant{UserID: 1, Kind: domain.GrantDailyLogin, Amount: domain.DailyLoginCredit, GrantDate: sql.NullString{String: "2026-07-19", Valid: true}, IdempotencyKey: "d04-login-1-2026-07-19", Status: domain.TaskSucceeded, CreatedAt: now})
	if err != nil || !ok {
		t.Fatalf("different same-day grant kind rejected: %v %v", ok, err)
	}
	b, err := s.GetUserBalanceSnapshot(ctx, 1)
	if err != nil || b.GrantTotal != 45_000_000 {
		t.Fatalf("snapshot: %+v %v", b, err)
	}
}

func TestOpenMigratesLegacyGrantUniquenessWithoutLosingRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	legacy := `CREATE TABLE credit_grants (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		kind TEXT NOT NULL,
		amount_micro_usd INTEGER NOT NULL,
		grant_date TEXT,
		invitee_user_id INTEGER,
		idempotency_key TEXT NOT NULL UNIQUE,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at TEXT NOT NULL,
		applied_at TEXT,
		note TEXT,
		UNIQUE(user_id, grant_date),
		UNIQUE(kind, invitee_user_id)
	)`
	if _, err := db.Exec(legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO credit_grants(user_id,kind,amount_micro_usd,grant_date,idempotency_key,status,created_at) VALUES(1,'daily_checkin',20000000,'2026-07-19','legacy','succeeded','2026-07-19T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	st, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ok, err := st.CreateGrant(context.Background(), Grant{UserID: 1, Kind: domain.GrantDailyLogin, Amount: domain.DailyLoginCredit, GrantDate: sql.NullString{String: "2026-07-19", Valid: true}, IdempotencyKey: "new-kind", Status: domain.TaskSucceeded, CreatedAt: time.Now()})
	if err != nil || !ok {
		t.Fatalf("migrated insert=%v err=%v", ok, err)
	}
	grants, err := st.ListAllGrants(context.Background())
	if err != nil || len(grants) != 2 || grants[0].IdempotencyKey != "legacy" {
		t.Fatalf("grants=%+v err=%v", grants, err)
	}
}

func TestStoreInvitationAndUsers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	id, err := s.RegisterInvitation(ctx, Invitation{JoinID: "join-1", CodeCiphertext: "code", CodeHash: "hash", IssuerUserID: 1, AffCode: "aff", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("missing id")
	}
	if err := s.RegisterUser(ctx, InternalUser{UserID: 2, InviterUserID: sql.NullInt64{Int64: 1, Valid: true}, InvitationID: sql.NullInt64{Int64: id, Valid: true}, JoinedAt: now}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.CountRegisteredUsers(ctx); got != 1 {
		t.Fatalf("count %d", got)
	}
	if err := s.MarkInvitationUsed(ctx, "hash", 2); err != nil {
		t.Fatal(err)
	}
	inv, err := s.GetInvitation(ctx, "join-1")
	if err != nil || !inv.UsedBy.Valid || inv.UsedBy.Int64 != 2 {
		t.Fatalf("invitation %+v %v", inv, err)
	}
}

func TestCompleteRegistrationIsAtomicAndIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	id, err := s.RegisterInvitation(ctx, Invitation{JoinID: "join-atomic", ProviderCodeID: 10, CodeCiphertext: "ciphertext", CodeHash: "hash-atomic", IssuerUserID: 1, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	user := InternalUser{UserID: 2, InviterUserID: sql.NullInt64{Int64: 1, Valid: true}, InvitationID: sql.NullInt64{Int64: id, Valid: true}, JoinedAt: now}
	grant := &Grant{UserID: 1, Kind: domain.GrantReferral, Amount: domain.ReferralGrant, InviteeUserID: sql.NullInt64{Int64: 2, Valid: true}, IdempotencyKey: "d04-referral-2", Status: "reserved", CreatedAt: now}
	for i := 0; i < 2; i++ {
		if err := s.CompleteRegistration(ctx, "hash-atomic", user, grant); err != nil {
			t.Fatal(err)
		}
	}
	if count, _ := s.CountRegisteredUsers(ctx); count != 1 {
		t.Fatalf("users=%d", count)
	}
	grants, err := s.ListAllGrants(ctx)
	if err != nil || len(grants) != 1 {
		t.Fatalf("grants=%+v err=%v", grants, err)
	}
	inv, err := s.GetInvitation(ctx, "join-atomic")
	if err != nil || !inv.UsedBy.Valid || inv.UsedBy.Int64 != 2 {
		t.Fatalf("invitation=%+v err=%v", inv, err)
	}
}

func TestEnrollLaunchUserEnforcesHardCapAndIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	for id := int64(1); id <= 15; id++ {
		created, err := s.EnrollLaunchUser(ctx, InternalUser{UserID: id, JoinedAt: now}, 15)
		if err != nil || !created {
			t.Fatalf("id=%d created=%v err=%v", id, created, err)
		}
	}
	created, err := s.EnrollLaunchUser(ctx, InternalUser{UserID: 15, JoinedAt: now}, 15)
	if err != nil || created {
		t.Fatalf("idempotent created=%v err=%v", created, err)
	}
	if _, err := s.EnrollLaunchUser(ctx, InternalUser{UserID: 16, JoinedAt: now}, 15); !errors.Is(err, ErrLaunchFull) {
		t.Fatalf("expected full, got %v", err)
	}
}

func TestConcurrentLaunchEnrollmentNeverExceedsFifteen(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	start := make(chan struct{})
	var wg sync.WaitGroup
	for id := int64(1); id <= 20; id++ {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()
			<-start
			_, err := s.EnrollLaunchUser(ctx, InternalUser{UserID: userID, JoinedAt: time.Now()}, 15)
			if err != nil && !errors.Is(err, ErrLaunchFull) {
				t.Errorf("user=%d err=%v", userID, err)
			}
		}(id)
	}
	close(start)
	wg.Wait()
	if count, err := s.CountRegisteredUsers(ctx); err != nil || count != 15 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestRegistrationSlotsEnforceCapacityAcrossStoreInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
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

	start := make(chan struct{})
	results := make(chan error, 2)
	for i, st := range []*Store{first, second} {
		go func(index int, storeInstance *Store) {
			<-start
			results <- storeInstance.ReserveRegistrationSlot(
				context.Background(), fmt.Sprintf("slot-%d", index), 1, time.Now(),
			)
		}(i, st)
	}
	close(start)
	var reserved, full int
	for i := 0; i < 2; i++ {
		switch err := <-results; {
		case err == nil:
			reserved++
		case errors.Is(err, ErrLaunchFull):
			full++
		default:
			t.Fatalf("unexpected reservation error: %v", err)
		}
	}
	if reserved != 1 || full != 1 {
		t.Fatalf("reserved=%d full=%d", reserved, full)
	}
}
