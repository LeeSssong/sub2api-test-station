# Whole-site accounting ledger activation runbook

This runbook activates the CNY whole-site accounting ledger after a controlled
baseline reset. Run the apply path only after the formal activation
authorization identifies the intended database target.

## What the report means

- **Customer revenue** is external usage revenue for the Shanghai calendar day.
  One consumed USD of customer site balance is reported as one CNY of revenue.
- **Resource cost** is the usage cost of all traffic, including internal traffic:
  `COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)`.
- **Operating gross profit** is `customer revenue - resource cost`. It measures
  the operating result of serving traffic and is independent of when cash was
  paid.
- **Cash outflow** is the sum of recorded purchase, upstream top-up, refund, and
  fee events for their actual `paid_at` day. A refund is recorded as a negative
  amount or as an explicit reversing event.
- **Cash net result** is `customer revenue - cash outflow`. A negative result on
  a purchase/top-up day is expected; the ledger does not amortize cash events.

## The six cash-event inputs

The protected accounting page accepts exactly these six fields:

1. `event_type`: `account_purchase`, `upstream_topup`, `refund`, or `fee`.
2. `paid_at`: actual payment timestamp, entered with the Asia/Shanghai offset.
3. `amount_cny`: positive CNY amount for purchases, top-ups, and fees; refunds
   use a negative amount or a separate reversing event.
4. `source_kind`: `owned_oauth` or `upstream_apikey`.
5. `account_id`: optional positive Sub2API account ID. Leave it empty when an
   upstream payment cannot be linked to one account.
6. `notes`: optional non-sensitive note (maximum 500 UTF-8 bytes). Never put an
   API key, cookie, token, password, OAuth credential, supplier secret, or
   other credential in this field.

Do not add batch IDs, supplier names, scope, external references, or secrets to
the event. To correct an event, insert a reversing event; do not edit or remove
the original cash event.

## Customer and internal traffic

Set `RELAY_OPS_ACCOUNTING_ENABLED=true` after activation authorization, and set
`RELAY_OPS_ACCOUNTING_LEDGER_START_DATE` to the confirmed first ledger date.
Configure administrator/internal traffic with
the comma-separated positive IDs in:

- `RELAY_OPS_ACCOUNTING_INTERNAL_USER_IDS`
- `RELAY_OPS_ACCOUNTING_INTERNAL_API_KEY_IDS`

Internal requests are excluded from customer revenue but remain included in
resource cost. If no internal IDs are configured, no traffic is excluded.

When importing an account, default `accounts.type = oauth` to
`source_kind=owned_oauth` and `accounts.type = apikey` to
`source_kind=upstream_apikey`. Correct the source only when the default is
known to be wrong. An unlinked upstream payment may keep `account_id` empty and
is reported as shared/unlinked cash outflow.

## First activation

1. Obtain formal activation authorization for the database target and ensure the
   operator's `infra/.env` contains its connection values. The reset script
   requires this file and never prints its password.
2. Choose the first ledger date, for example `2026-08-02`, and run the dry run:

   ```bash
   bash ops/reset-accounting-baseline.sh --dry-run --start-date 2026-08-02
   ```

   Review every per-table count. Dry-run performs no writes and creates no
   backup. It prints the exact `RELAY_OPS_ACCOUNTING_LEDGER_START_DATE` value to
   set.

3. If the counts and target are correct, stop or quiesce every database writer,
   including Sub2API, Relay Ops, scheduled jobs, imports, and any direct SQL
   maintenance process. Keep every writer stopped through post-commit
   verification and relay-ops reconfiguration.

4. Run the explicit apply command with the exact matching confirmation date:

   ```bash
   bash ops/reset-accounting-baseline.sh \
     --apply \
     --start-date 2026-08-02 \
     --confirm-ledger-start-date 2026-08-02
   ```

   The script writes a timestamped custom-format PostgreSQL archive under
   `ACCOUNTING_BACKUP_DIR` when set, or
   `backups/accounting-baseline/` by default. The archive is created before
   the transaction. Apply deletes only historical accounting/usage rows; it
   preserves accounts, groups, API keys, users, channels, settings,
   credentials, routes, and model pricing.

5. Set the four accounting environment variables, including the exact start
   date printed by the script, then restart/recreate only the relay-ops service
   using the normal deployment procedure. Never put credentials or purchase
   costs in `infra/.env.example` or Compose configuration.

6. For the first three Shanghai calendar days, verify after the scheduled
   00:10 run that each daily snapshot is present, customer revenue excludes
   configured internal IDs, resource cost includes all traffic, and cash
   events reconcile to the operator's evidence. Late usage and late event entry
   are expected to converge when the scheduler recomputes recent days.

The prior **2026-07-11 through 2026-07-26 administrator-only usage is excluded
from new revenue**. It is not backfilled by this activation and should not be
treated as customer revenue in the first snapshots.

## Daily operator workflow

1. Import the account into Sub2API.
2. Open the protected Relay Ops accounting page.
3. Create one cash event for the actual purchase/top-up/refund/fee.
4. Check the next daily snapshot and investigate any unlinked cash outflow or
   unexpected source-cost row.

Keep the database backup archive, the confirmed ledger start date, the first
daily snapshot, and the complete cash-event history as activation evidence.
