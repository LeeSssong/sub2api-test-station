# Task 1 review-fix report

- Added cascade/set-null delete annotations to the user and ledger relations.
- Added a unique `ledger_entry_id` constraint to quota idempotency records.
- Changed the SQL wallet table to use `user_id` as its primary key/foreign key and retained active-user-only initialization with `ON CONFLICT (user_id) DO NOTHING`.
- Migration contract test now strips comments, normalizes SQL whitespace, and checks the concrete primary-key, foreign-key, unique, index, initialization, and no-history-backfill fragments.
- The repository uses Ent v0.14.5, whose `field.ID` API only supports composite edge-schema identifiers (`field.ID(first, second, ...)`), not a single field. The schema therefore keeps `user_id` as a unique Int64 field while the migration enforces the actual single-column database primary key.

## Validation

- `go test ./migrations -run UserQuotaWalletLedger -v` — passed.
- `go test ./ent/schema -run 'Schema' -v` — passed.
- `git diff --check` — passed.

Migration filename is `227_user_quota_wallet_ledger.sql` because migration `225` is already occupied by `225_account_model_detection.sql` in this branch.
