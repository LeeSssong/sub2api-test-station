#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage:
  ops/reset-accounting-baseline.sh --dry-run --start-date YYYY-MM-DD
  ops/reset-accounting-baseline.sh --apply --start-date YYYY-MM-DD --confirm-ledger-start-date YYYY-MM-DD
USAGE
}

die() {
  echo "reset-accounting-baseline: $*" >&2
  exit 1
}

mode=''
start_date=''
confirm_start_date=''
while (($#)); do
  case "$1" in
    --dry-run)
      [[ -z "$mode" ]] || die 'choose exactly one of --dry-run or --apply'
      mode='dry-run'
      ;;
    --apply)
      [[ -z "$mode" ]] || die 'choose exactly one of --dry-run or --apply'
      mode='apply'
      ;;
    --start-date)
      (($# >= 2)) || die '--start-date requires YYYY-MM-DD'
      start_date=$2
      shift
      ;;
    --confirm-ledger-start-date)
      (($# >= 2)) || die '--confirm-ledger-start-date requires YYYY-MM-DD'
      confirm_start_date=$2
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      die "unknown argument: $1"
      ;;
  esac
  shift
done

[[ -n "$mode" ]] || { usage; exit 1; }
[[ -n "$start_date" ]] || die '--start-date is required'
[[ "$start_date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || die '--start-date must be YYYY-MM-DD'
if [[ "$mode" == 'apply' ]]; then
  [[ "$confirm_start_date" == "$start_date" ]] || die '--confirm-ledger-start-date must exactly match --start-date'
else
  [[ -z "$confirm_start_date" ]] || die '--confirm-ledger-start-date is only valid with --apply'
fi

root_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
env_file=${ACCOUNTING_ENV_FILE:-"$root_dir/infra/.env"}
[[ -f "$env_file" ]] || die "required environment file is missing: $env_file"

# infra/.env is an operator-owned dotenv file. It is sourced without tracing;
# no values are echoed, and the password is passed only through PGPASSWORD.
set -a
# shellcheck disable=SC1090
source "$env_file"
set +a

for deployment_env in "${RELAY_OPS_DEPLOYMENT_ENV:-}" "${RELAY_OPS_ENVIRONMENT:-}" "${APP_ENV:-}" "${ENVIRONMENT:-}" "${NODE_ENV:-}"; do
  case "$(printf '%s' "$deployment_env" | tr '[:upper:]' '[:lower:]')" in
    production|prod)
      die 'refusing to run against an environment marked production'
      ;;
  esac
done

command -v psql >/dev/null 2>&1 || die 'psql is required'
if [[ "$mode" == 'apply' ]]; then
  command -v pg_dump >/dev/null 2>&1 || die 'pg_dump is required for --apply'
fi

pg_host=${POSTGRES_HOST:-${DATABASE_HOST:-127.0.0.1}}
pg_port=${POSTGRES_PORT:-${DATABASE_PORT:-5432}}
pg_user=${POSTGRES_USER:-${DATABASE_USER:-}}
pg_db=${POSTGRES_DB:-${DATABASE_DBNAME:-}}
pg_password=${POSTGRES_PASSWORD:-${DATABASE_PASSWORD:-}}
[[ -n "$pg_user" ]] || die 'POSTGRES_USER (or DATABASE_USER) is required in infra/.env'
[[ -n "$pg_db" ]] || die 'POSTGRES_DB (or DATABASE_DBNAME) is required in infra/.env'
[[ -n "$pg_password" ]] || die 'POSTGRES_PASSWORD (or DATABASE_PASSWORD) is required in infra/.env'

export PGHOST="$pg_host" PGPORT="$pg_port" PGUSER="$pg_user" PGDATABASE="$pg_db" PGPASSWORD="$pg_password"
psql_args=(-X -v ON_ERROR_STOP=1)

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

count_table() {
  local table=$1 count
  count=$(psql "${psql_args[@]}" -Atqc "SELECT count(*) FROM ${table};") || die "count failed for $table"
  [[ "$count" =~ ^[0-9]+$ ]] || die "invalid count returned for $table"
  printf '%s\t%s\n' "$table" "$count"
}

echo "Accounting baseline reset ($mode) for start date $start_date"
echo 'Table counts:'
for table in "${reset_tables[@]}"; do
  count_table "$table"
done

if [[ "$mode" == 'dry-run' ]]; then
  echo 'Dry run complete; no rows were changed and no backup was created.'
  echo "Set RELAY_OPS_ACCOUNTING_LEDGER_START_DATE=$start_date before restarting relay-ops."
  exit 0
fi

backup_dir=${ACCOUNTING_BACKUP_DIR:-"$root_dir/backups/accounting-baseline"}
umask 077
mkdir -p -- "$backup_dir"
chmod 700 -- "$backup_dir"
backup_path="$backup_dir/accounting-baseline-$(date -u +%Y%m%dT%H%M%SZ).dump"
echo "Creating PostgreSQL backup archive: $backup_path"
if ! pg_dump -Fc >"$backup_path"; then
  rm -f -- "$backup_path"
  die 'pg_dump failed; apply was not attempted'
fi
[[ -s "$backup_path" ]] || die 'pg_dump produced an empty archive; apply was not attempted'

if ! psql "${psql_args[@]}" <<'SQL'
BEGIN;
DELETE FROM public.billing_usage_entries;
DELETE FROM public.usage_billing_dedup;
DELETE FROM public.usage_billing_dedup_archive;
DELETE FROM public.usage_dashboard_hourly_users;
DELETE FROM public.usage_dashboard_daily_users;
DELETE FROM public.usage_dashboard_hourly;
DELETE FROM public.usage_dashboard_daily;
DELETE FROM public.usage_logs;
DELETE FROM public.user_affiliate_ledger;
DELETE FROM public.payment_orders;
DELETE FROM relay_ops.accounting_cash_events;
DELETE FROM relay_ops.accounting_daily_snapshots;
COMMIT;
SQL
then
  die 'accounting reset transaction failed; transaction was not committed'
fi

echo 'Post-commit verification:'
for table in "${reset_tables[@]}"; do
  verification=$(count_table "$table")
  echo "$verification"
  [[ "${verification##*$'\t'}" == '0' ]] || die "verification failed for $table"
done

echo "Backup archive: $backup_path"
echo "Set RELAY_OPS_ACCOUNTING_LEDGER_START_DATE=$start_date before restarting relay-ops."
echo 'Accounting baseline reset applied successfully.'
