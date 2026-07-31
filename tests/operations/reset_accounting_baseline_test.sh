#!/usr/bin/env bash
set -euo pipefail

script="ops/reset-accounting-baseline.sh"
runbook="docs/runbooks/whole-site-accounting-ledger.md"
grep -Fq -- '--dry-run' "$script"
grep -Fq -- '--apply' "$script"
grep -Fq -- 'pg_dump' "$script"
grep -Fq -- 'usage_logs' "$script"
grep -Fq -- 'billing_usage_entries' "$script"
grep -Fq -- 'usage_dashboard_daily' "$script"
grep -Fq -- 'accounting_cash_events' "$script"
grep -Fq -- 'accounting_daily_snapshots' "$script"
grep -Fq -- 'user_affiliate_ledger' "$script"
if grep -Eq 'DELETE FROM public\.(accounts|groups|api_keys|users)' "$script"; then
  echo "reset must preserve runtime configuration" >&2
  exit 1
fi

reset_tables=(
  'public.billing_usage_entries'
  'public.usage_billing_dedup'
  'public.usage_billing_dedup_archive'
  'public.usage_dashboard_hourly_users'
  'public.usage_dashboard_daily_users'
  'public.usage_dashboard_hourly'
  'public.usage_dashboard_daily'
  'public.usage_logs'
  'public.user_affiliate_ledger'
  'public.payment_orders'
  'relay_ops.accounting_cash_events'
  'relay_ops.accounting_daily_snapshots'
)
for table in "${reset_tables[@]}"; do
  grep -Fq -- "LOCK TABLE $table IN ACCESS EXCLUSIVE MODE;" "$script"
done
grep -Fq -- 'DO $$' "$script"
grep -Fq -- 'reset verification failed' "$script"
grep -Fq -- 'stop or quiesce every database writer' "$runbook"
grep -Fq -- 'Keep every writer stopped through post-commit' "$runbook"
grep -Fq -- 'verification and relay-ops reconfiguration' "$runbook"
if grep -Fq -- 'refusing to run against an environment marked production' "$script"; then
  echo "reset must not reject the formally authorized production activation" >&2
  exit 1
fi
if grep -Fq -- 'non-production' "$runbook"; then
  echo "runbook must not restrict the formally authorized activation to non-production" >&2
  exit 1
fi

fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
mkdir -p "$fixture/bin"
printf '%s\n' \
  'POSTGRES_USER=test-user' \
  'POSTGRES_PASSWORD=test-password' \
  'POSTGRES_DB=test-db' >"$fixture/infra.env"
cat >"$fixture/bin/psql" <<'PSQL'
#!/usr/bin/env bash
printf 'psql\n' >>"$RESET_TEST_CALL_LOG"
printf '0\n'
PSQL
cat >"$fixture/bin/pg_dump" <<'PG_DUMP'
#!/usr/bin/env bash
printf 'pg_dump\n' >>"$RESET_TEST_CALL_LOG"
printf 'fake backup\n'
PG_DUMP
chmod +x "$fixture/bin/psql" "$fixture/bin/pg_dump"

run_reset() {
  ACCOUNTING_ENV_FILE="$fixture/infra.env" \
    RESET_TEST_CALL_LOG="$fixture/calls" \
    PATH="$fixture/bin:$PATH" \
    bash "$script" "$@"
}

: >"$fixture/calls"
run_reset --dry-run --start-date 2024-02-29 >/dev/null
grep -Fq -- 'psql' "$fixture/calls"

: >"$fixture/calls"
run_reset --dry-run --start-date 2025-02-28 >/dev/null
grep -Fq -- 'psql' "$fixture/calls"

for invalid_date in 2026-02-30 2025-02-29; do
  : >"$fixture/calls"
  invalid_output="$fixture/$invalid_date.out"
  if ACCOUNTING_ENV_FILE="$fixture/missing.env" \
    RESET_TEST_CALL_LOG="$fixture/calls" \
    PATH="$fixture/bin:$PATH" \
    bash "$script" --apply --start-date "$invalid_date" \
      --confirm-ledger-start-date "$invalid_date" >"$invalid_output" 2>&1; then
    echo "invalid date $invalid_date must be rejected" >&2
    exit 1
  fi
  grep -Fq -- '--start-date must be a real Gregorian calendar date' "$invalid_output"
  [[ ! -s "$fixture/calls" ]]
done
