#!/usr/bin/env bash
set -euo pipefail

script="ops/reset-accounting-baseline.sh"
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

production_output=$(mktemp)
trap 'rm -f "$production_output"' EXIT
if ACCOUNTING_ENV_FILE=infra/.env.example RELAY_OPS_ENVIRONMENT=production \
  bash "$script" --dry-run --start-date 2026-08-02 >"$production_output" 2>&1; then
  echo "reset must refuse a production-labelled environment" >&2
  exit 1
fi
grep -Fq -- 'refusing to run against an environment marked production' "$production_output"
