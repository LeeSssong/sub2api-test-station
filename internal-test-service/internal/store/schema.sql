PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS internal_users (
  user_id INTEGER PRIMARY KEY,
  inviter_user_id INTEGER,
  invitation_id INTEGER,
  joined_at TEXT NOT NULL,
  first_usage_at TEXT,
  FOREIGN KEY(invitation_id) REFERENCES invitations(id)
);
CREATE TABLE IF NOT EXISTS registration_slots (
  slot_id TEXT PRIMARY KEY,
  reserved_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS invitations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  join_id TEXT NOT NULL UNIQUE,
  provider_code_id INTEGER,
  code_ciphertext TEXT NOT NULL,
  code_hash TEXT NOT NULL UNIQUE,
  issuer_user_id INTEGER NOT NULL,
  aff_code TEXT NOT NULL DEFAULT '',
  used_by INTEGER,
  created_at TEXT NOT NULL,
  closed_at TEXT
);
CREATE TABLE IF NOT EXISTS credit_grants (
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
);
CREATE TABLE IF NOT EXISTS usage_cursors (user_id INTEGER PRIMARY KEY, after_id INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS usage_records (usage_id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL, amount_micro_usd INTEGER NOT NULL, successful INTEGER NOT NULL, recorded_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS jobs (job_key TEXT PRIMARY KEY, kind TEXT NOT NULL, status TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, next_run_at TEXT NOT NULL, last_error TEXT);
CREATE TABLE IF NOT EXISTS audit_events (id INTEGER PRIMARY KEY AUTOINCREMENT, kind TEXT NOT NULL, user_id INTEGER, metadata TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE INDEX IF NOT EXISTS idx_invitations_used ON invitations(used_by);
CREATE INDEX IF NOT EXISTS idx_usage_user ON usage_records(user_id, usage_id);
CREATE INDEX IF NOT EXISTS idx_grants_user ON credit_grants(user_id, created_at);
