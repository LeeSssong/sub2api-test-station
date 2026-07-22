package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"example.invalid/internal-test-service/internal/domain"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

type Invitation struct {
	ID             int64
	JoinID         string
	ProviderCodeID int64
	CodeCiphertext string
	CodeHash       string
	IssuerUserID   int64
	AffCode        string
	UsedBy         sql.NullInt64
	CreatedAt      time.Time
	ClosedAt       sql.NullTime
}

type InternalUser struct {
	UserID        int64
	InviterUserID sql.NullInt64
	InvitationID  sql.NullInt64
	JoinedAt      time.Time
	FirstUsageAt  sql.NullTime
}

type Grant struct {
	ID             int64
	UserID         int64
	Kind           string
	Amount         domain.MicroUSD
	GrantDate      sql.NullString
	InviteeUserID  sql.NullInt64
	IdempotencyKey string
	Status         string
	CreatedAt      time.Time
	AppliedAt      sql.NullTime
	Note           string
}

type UsageRecord struct {
	UsageID    int64
	UserID     int64
	Amount     domain.MicroUSD
	Successful bool
	RecordedAt time.Time
}

type BalanceSnapshot struct {
	GrantTotal domain.MicroUSD
	UsageTotal domain.MicroUSD
}

type Store struct{ db *sql.DB }

var ErrLaunchFull = errors.New("launch roster is full")
var ErrRegistrationSlotMissing = errors.New("registration slot is missing")

func BackupSQLite(ctx context.Context, sourcePath, destinationPath string) (err error) {
	if !filepath.IsAbs(sourcePath) || !filepath.IsAbs(destinationPath) {
		return errors.New("backup paths must be absolute")
	}
	if filepath.Clean(sourcePath) == filepath.Clean(destinationPath) {
		return errors.New("backup destination must differ from source")
	}
	if _, statErr := os.Stat(destinationPath); statErr == nil {
		return errors.New("backup destination already exists")
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("inspect backup destination: %w", statErr)
	}

	db, err := sql.Open("sqlite", readOnlySQLiteDSN(sourcePath))
	if err != nil {
		return fmt.Errorf("open backup source: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	complete := false
	defer func() {
		if !complete {
			_ = os.Remove(destinationPath)
		}
	}()
	if _, err = db.ExecContext(ctx, `VACUUM INTO ?`, destinationPath); err != nil {
		return fmt.Errorf("create sqlite backup: %w", err)
	}

	copyDB, err := sql.Open("sqlite", readOnlySQLiteDSN(destinationPath))
	if err != nil {
		return fmt.Errorf("open sqlite backup: %w", err)
	}
	defer copyDB.Close()
	var integrity string
	if err = copyDB.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return fmt.Errorf("verify sqlite backup: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("verify sqlite backup: %s", integrity)
	}
	complete = true
	return nil
}

func readOnlySQLiteDSN(path string) string {
	location := &url.URL{Scheme: "file", Path: path}
	query := location.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "busy_timeout(5000)")
	location.RawQuery = query.Encode()
	return location.String()
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	contents, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.ExecContext(ctx, string(contents)); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	if err = migrateLegacyGrantUniqueness(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("schema migration: %w", err)
	}
	return &Store{db: db}, nil
}

func migrateLegacyGrantUniqueness(ctx context.Context, db *sql.DB) error {
	var definition string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='credit_grants'`).Scan(&definition); err != nil {
		return err
	}
	if !contains(definition, "UNIQUE(user_id, grant_date)") {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		_ = tx.Rollback()
		return cause
	}
	statements := []string{
		`ALTER TABLE credit_grants RENAME TO credit_grants_legacy`,
		`CREATE TABLE credit_grants (
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
			UNIQUE(user_id, kind, grant_date),
			UNIQUE(kind, invitee_user_id)
		)`,
		`INSERT INTO credit_grants(id,user_id,kind,amount_micro_usd,grant_date,invitee_user_id,idempotency_key,status,created_at,applied_at,note)
		 SELECT id,user_id,kind,amount_micro_usd,grant_date,invitee_user_id,idempotency_key,status,created_at,applied_at,note FROM credit_grants_legacy`,
		`DROP TABLE credit_grants_legacy`,
		`CREATE INDEX IF NOT EXISTS idx_grants_user ON credit_grants(user_id, created_at)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return rollback(err)
		}
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) RegisterInvitation(ctx context.Context, inv Invitation) (int64, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO invitations(join_id,provider_code_id,code_ciphertext,code_hash,issuer_user_id,aff_code,created_at) VALUES(?,?,?,?,?,?,?)`, inv.JoinID, inv.ProviderCodeID, inv.CodeCiphertext, inv.CodeHash, inv.IssuerUserID, inv.AffCode, inv.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func scanInvitation(row interface{ Scan(...any) error }) (Invitation, error) {
	var inv Invitation
	var created, closed string
	err := row.Scan(&inv.ID, &inv.JoinID, &inv.ProviderCodeID, &inv.CodeCiphertext, &inv.CodeHash, &inv.IssuerUserID, &inv.AffCode, &inv.UsedBy, &created, &closed)
	if err != nil {
		return inv, err
	}
	inv.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if closed != "" {
		inv.ClosedAt = sql.NullTime{Time: mustTime(closed), Valid: true}
	}
	return inv, nil
}

func mustTime(value string) time.Time { t, _ := time.Parse(time.RFC3339Nano, value); return t }

func invitationSelect() string {
	return `SELECT id,join_id,provider_code_id,code_ciphertext,code_hash,issuer_user_id,aff_code,used_by,created_at,COALESCE(closed_at,'') FROM invitations`
}

func (s *Store) GetInvitation(ctx context.Context, joinID string) (Invitation, error) {
	return scanInvitation(s.db.QueryRowContext(ctx, invitationSelect()+` WHERE join_id = ?`, joinID))
}

func (s *Store) GetInvitationByHash(ctx context.Context, codeHash string) (Invitation, error) {
	return scanInvitation(s.db.QueryRowContext(ctx, invitationSelect()+` WHERE code_hash = ?`, codeHash))
}

func (s *Store) ListInvitationUses(ctx context.Context) ([]Invitation, error) {
	rows, err := s.db.QueryContext(ctx, invitationSelect()+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Invitation
	for rows.Next() {
		inv, err := scanInvitation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, inv)
	}
	return result, rows.Err()
}

func (s *Store) MarkInvitationUsed(ctx context.Context, codeHash string, userID int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE invitations SET used_by=? WHERE code_hash=? AND used_by IS NULL`, userID, codeHash)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return fmt.Errorf("invitation already used or missing")
	}
	return nil
}

func (s *Store) CloseUnusedInvitations(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE invitations SET closed_at=? WHERE used_by IS NULL AND closed_at IS NULL`, now.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) RegisterUser(ctx context.Context, user InternalUser) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO internal_users(user_id,inviter_user_id,invitation_id,joined_at) VALUES(?,?,?,?)`, user.UserID, user.InviterUserID, user.InvitationID, user.JoinedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) EnrollLaunchUser(ctx context.Context, user InternalUser, maxUsers int) (bool, error) {
	created := false
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM internal_users WHERE user_id=?`, user.UserID).Scan(&exists); err != nil {
			return err
		}
		if exists == 1 {
			return nil
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM internal_users`).Scan(&count); err != nil {
			return err
		}
		if count >= maxUsers {
			return ErrLaunchFull
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO internal_users(user_id,joined_at) VALUES(?,?)`, user.UserID, user.JoinedAt.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		created = rows == 1
		return err
	})
	return created, err
}

func (s *Store) ReserveRegistrationSlot(ctx context.Context, slotID string, maxUsers int, reservedAt time.Time) error {
	if slotID == "" || maxUsers < 1 {
		return fmt.Errorf("invalid registration reservation")
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO registration_slots(slot_id,reserved_at)
		SELECT ?, ?
		WHERE (SELECT COUNT(*) FROM internal_users) + (SELECT COUNT(*) FROM registration_slots) < ?`,
		slotID, reservedAt.UTC().Format(time.RFC3339Nano), maxUsers)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrLaunchFull
	}
	return nil
}

func (s *Store) ReleaseRegistrationSlot(ctx context.Context, slotID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM registration_slots WHERE slot_id=?`, slotID)
	return err
}

func (s *Store) CompleteRegistrationSlot(ctx context.Context, slotID string, user InternalUser) (bool, error) {
	created := false
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM registration_slots WHERE slot_id=?`, slotID).Scan(&exists); err != nil {
			return err
		}
		if exists != 1 {
			return ErrRegistrationSlotMissing
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO internal_users(user_id,joined_at) VALUES(?,?) ON CONFLICT(user_id) DO NOTHING`, user.UserID, user.JoinedAt.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		created = rows == 1
		_, err = tx.ExecContext(ctx, `DELETE FROM registration_slots WHERE slot_id=?`, slotID)
		return err
	})
	return created, err
}

func (s *Store) CountLaunchCapacity(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM internal_users) + (SELECT COUNT(*) FROM registration_slots)`).Scan(&count)
	return count, err
}

func (s *Store) CompleteRegistration(ctx context.Context, codeHash string, user InternalUser, referral *Grant) error {
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		var usedBy sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT used_by FROM invitations WHERE code_hash=?`, codeHash).Scan(&usedBy); err != nil {
			return err
		}
		if usedBy.Valid && usedBy.Int64 != user.UserID {
			return fmt.Errorf("invitation already used by another user")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE invitations SET used_by=? WHERE code_hash=?`, user.UserID, codeHash); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO internal_users(user_id,inviter_user_id,invitation_id,joined_at) VALUES(?,?,?,?) ON CONFLICT(user_id) DO NOTHING`, user.UserID, nullInt(user.InviterUserID), nullInt(user.InvitationID), user.JoinedAt.UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		if referral == nil {
			return nil
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO credit_grants(user_id,kind,amount_micro_usd,grant_date,invitee_user_id,idempotency_key,status,created_at,note) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`, referral.UserID, referral.Kind, referral.Amount, nullString(referral.GrantDate), nullInt(referral.InviteeUserID), referral.IdempotencyKey, referral.Status, referral.CreatedAt.UTC().Format(time.RFC3339Nano), referral.Note)
		return err
	})
}

func (s *Store) MarkFirstUsage(ctx context.Context, userID int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE internal_users SET first_usage_at=COALESCE(first_usage_at,?) WHERE user_id=?`, at.UTC().Format(time.RFC3339Nano), userID)
	return err
}

func (s *Store) CountRegisteredUsers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM internal_users`).Scan(&count)
	return count, err
}

func (s *Store) ListInternalUsers(ctx context.Context) ([]InternalUser, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id,inviter_user_id,invitation_id,joined_at,COALESCE(first_usage_at,'') FROM internal_users ORDER BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []InternalUser
	for rows.Next() {
		var u InternalUser
		var joined, first string
		if err := rows.Scan(&u.UserID, &u.InviterUserID, &u.InvitationID, &joined, &first); err != nil {
			return nil, err
		}
		u.JoinedAt = mustTime(joined)
		if first != "" {
			u.FirstUsageAt = sql.NullTime{Time: mustTime(first), Valid: true}
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) GetInternalUser(ctx context.Context, userID int64) (InternalUser, error) {
	var u InternalUser
	var joined, first string
	err := s.db.QueryRowContext(ctx, `SELECT user_id,inviter_user_id,invitation_id,joined_at,COALESCE(first_usage_at,'') FROM internal_users WHERE user_id=?`, userID).Scan(&u.UserID, &u.InviterUserID, &u.InvitationID, &joined, &first)
	if err != nil {
		return u, err
	}
	u.JoinedAt = mustTime(joined)
	if first != "" {
		u.FirstUsageAt = sql.NullTime{Time: mustTime(first), Valid: true}
	}
	return u, nil
}

func (s *Store) CreateGrant(ctx context.Context, grant Grant) (bool, error) {
	result, err := s.db.ExecContext(ctx, `INSERT INTO credit_grants(user_id,kind,amount_micro_usd,grant_date,invitee_user_id,idempotency_key,status,created_at,note) VALUES(?,?,?,?,?,?,?,?,?)`, grant.UserID, grant.Kind, grant.Amount, nullString(grant.GrantDate), nullInt(grant.InviteeUserID), grant.IdempotencyKey, grant.Status, grant.CreatedAt.UTC().Format(time.RFC3339Nano), grant.Note)
	if err != nil {
		if isConstraint(err) {
			return false, nil
		}
		return false, err
	}
	_, err = result.LastInsertId()
	return err == nil, err
}

func isConstraint(err error) bool {
	return err != nil && (contains(err.Error(), "UNIQUE") || contains(err.Error(), "constraint"))
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
func nullString(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}
func nullInt(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}

func (s *Store) FindGrantByIdempotencyKey(ctx context.Context, key string) (Grant, error) {
	var g Grant
	var date, applied string
	var amount int64
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id,user_id,kind,amount_micro_usd,COALESCE(grant_date,''),invitee_user_id,idempotency_key,status,created_at,COALESCE(applied_at,''),COALESCE(note,'') FROM credit_grants WHERE idempotency_key=?`, key).Scan(&g.ID, &g.UserID, &g.Kind, &amount, &date, &g.InviteeUserID, &g.IdempotencyKey, &g.Status, &created, &applied, &g.Note)
	if err != nil {
		return g, err
	}
	g.Amount = domain.MicroUSD(amount)
	if date != "" {
		g.GrantDate = sql.NullString{String: date, Valid: true}
	}
	g.CreatedAt = mustTime(created)
	if applied != "" {
		g.AppliedAt = sql.NullTime{Time: mustTime(applied), Valid: true}
	}
	return g, nil
}

func (s *Store) MarkGrantApplied(ctx context.Context, key string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE credit_grants SET status=?,applied_at=? WHERE idempotency_key=?`, domain.TaskSucceeded, at.UTC().Format(time.RFC3339Nano), key)
	return err
}

func (s *Store) ListGrants(ctx context.Context, userID int64) ([]Grant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_id,kind,amount_micro_usd,COALESCE(grant_date,''),invitee_user_id,idempotency_key,status,created_at,COALESCE(applied_at,''),COALESCE(note,'') FROM credit_grants WHERE user_id=? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Grant
	for rows.Next() {
		var g Grant
		var amount int64
		var date, created, applied string
		if err := rows.Scan(&g.ID, &g.UserID, &g.Kind, &amount, &date, &g.InviteeUserID, &g.IdempotencyKey, &g.Status, &created, &applied, &g.Note); err != nil {
			return nil, err
		}
		g.Amount = domain.MicroUSD(amount)
		if date != "" {
			g.GrantDate = sql.NullString{String: date, Valid: true}
		}
		g.CreatedAt = mustTime(created)
		if applied != "" {
			g.AppliedAt = sql.NullTime{Time: mustTime(applied), Valid: true}
		}
		list = append(list, g)
	}
	return list, rows.Err()
}

func (s *Store) ListAllGrants(ctx context.Context) ([]Grant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_id,kind,amount_micro_usd,COALESCE(grant_date,''),invitee_user_id,idempotency_key,status,created_at,COALESCE(applied_at,''),COALESCE(note,'') FROM credit_grants ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Grant
	for rows.Next() {
		var g Grant
		var amount int64
		var date, created, applied string
		if err := rows.Scan(&g.ID, &g.UserID, &g.Kind, &amount, &date, &g.InviteeUserID, &g.IdempotencyKey, &g.Status, &created, &applied, &g.Note); err != nil {
			return nil, err
		}
		g.Amount = domain.MicroUSD(amount)
		if date != "" {
			g.GrantDate = sql.NullString{String: date, Valid: true}
		}
		g.CreatedAt = mustTime(created)
		if applied != "" {
			g.AppliedAt = sql.NullTime{Time: mustTime(applied), Valid: true}
		}
		list = append(list, g)
	}
	return list, rows.Err()
}

func (s *Store) FindReferralReservation(ctx context.Context, inviteeUserID int64) (Grant, error) {
	var g Grant
	var amount int64
	var date, created, applied string
	err := s.db.QueryRowContext(ctx, `SELECT id,user_id,kind,amount_micro_usd,COALESCE(grant_date,''),invitee_user_id,idempotency_key,status,created_at,COALESCE(applied_at,''),COALESCE(note,'') FROM credit_grants WHERE kind=? AND invitee_user_id=?`, domain.GrantReferral, inviteeUserID).Scan(&g.ID, &g.UserID, &g.Kind, &amount, &date, &g.InviteeUserID, &g.IdempotencyKey, &g.Status, &created, &applied, &g.Note)
	if err != nil {
		return g, err
	}
	g.Amount = domain.MicroUSD(amount)
	if date != "" {
		g.GrantDate = sql.NullString{String: date, Valid: true}
	}
	g.CreatedAt = mustTime(created)
	if applied != "" {
		g.AppliedAt = sql.NullTime{Time: mustTime(applied), Valid: true}
	}
	return g, nil
}

func (s *Store) ListPendingReferralReservations(ctx context.Context) ([]Grant, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,user_id,kind,amount_micro_usd,COALESCE(grant_date,''),invitee_user_id,idempotency_key,status,created_at,COALESCE(applied_at,''),COALESCE(note,'') FROM credit_grants WHERE kind=? AND status=? ORDER BY id`, domain.GrantReferral, "reserved")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Grant
	for rows.Next() {
		var g Grant
		var amount int64
		var date, created, applied string
		if err := rows.Scan(&g.ID, &g.UserID, &g.Kind, &amount, &date, &g.InviteeUserID, &g.IdempotencyKey, &g.Status, &created, &applied, &g.Note); err != nil {
			return nil, err
		}
		g.Amount = domain.MicroUSD(amount)
		if date != "" {
			g.GrantDate = sql.NullString{String: date, Valid: true}
		}
		g.CreatedAt = mustTime(created)
		if applied != "" {
			g.AppliedAt = sql.NullTime{Time: mustTime(applied), Valid: true}
		}
		list = append(list, g)
	}
	return list, rows.Err()
}

func (s *Store) SumSuccessfulUsage(ctx context.Context) (domain.MicroUSD, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_micro_usd),0) FROM usage_records WHERE successful=1`).Scan(&n)
	return domain.MicroUSD(n), err
}

func (s *Store) RecordUsage(ctx context.Context, record UsageRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO usage_records(usage_id,user_id,amount_micro_usd,successful,recorded_at) VALUES(?,?,?,?,?) ON CONFLICT(usage_id) DO NOTHING`, record.UsageID, record.UserID, record.Amount, boolInt(record.Successful), record.RecordedAt.UTC().Format(time.RFC3339Nano))
	return err
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Store) GetUsageCursor(ctx context.Context, userID int64) (int64, error) {
	var after int64
	err := s.db.QueryRowContext(ctx, `SELECT after_id FROM usage_cursors WHERE user_id=?`, userID).Scan(&after)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return after, err
}
func (s *Store) SetUsageCursor(ctx context.Context, userID, after int64, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO usage_cursors(user_id,after_id,updated_at) VALUES(?,?,?) ON CONFLICT(user_id) DO UPDATE SET after_id=excluded.after_id,updated_at=excluded.updated_at`, userID, after, now.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetUserBalanceSnapshot(ctx context.Context, userID int64) (BalanceSnapshot, error) {
	var b BalanceSnapshot
	var grants, usage int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT SUM(amount_micro_usd) FROM credit_grants WHERE user_id=? AND status=?),0),COALESCE((SELECT SUM(amount_micro_usd) FROM usage_records WHERE user_id=? AND successful=1),0)`, userID, domain.TaskSucceeded, userID).Scan(&grants, &usage)
	b.GrantTotal = domain.MicroUSD(grants)
	b.UsageTotal = domain.MicroUSD(usage)
	return b, err
}

func (s *Store) SetReadOnlyReason(ctx context.Context, reason string) error {
	return s.SetSetting(ctx, "read_only_reason", reason)
}
func (s *Store) GetReadOnlyReason(ctx context.Context) (string, error) {
	return s.GetSetting(ctx, "read_only_reason")
}
func (s *Store) Audit(ctx context.Context, kind string, userID *int64, metadata map[string]string, now time.Time) error {
	b, _ := json.Marshal(metadata)
	var id any
	if userID != nil {
		id = *userID
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit_events(kind,user_id,metadata,created_at) VALUES(?,?,?,?)`, kind, id, string(b), now.UTC().Format(time.RFC3339Nano))
	return err
}
