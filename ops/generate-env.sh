#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TEMPLATE="$ROOT/infra/.env.example"
TARGET=${1:-"$ROOT/infra/.env"}

[[ -f "$TEMPLATE" ]] || {
  printf 'missing template: %s\n' "$TEMPLATE" >&2
  exit 1
}

[[ ! -e "$TARGET" ]] || {
  printf 'refusing to overwrite: %s\n' "$TARGET" >&2
  exit 1
}

mkdir -p "$(dirname "$TARGET")"
umask 077
TEMP_FILE=$(mktemp "${TARGET}.tmp.XXXXXX")
trap 'rm -f "$TEMP_FILE"' EXIT

POSTGRES_PASSWORD=$(openssl rand -hex 32)
REDIS_PASSWORD=$(openssl rand -hex 32)
ADMIN_PASSWORD=$(openssl rand -hex 32)
JWT_SECRET=$(openssl rand -hex 32)
TOTP_ENCRYPTION_KEY=$(openssl rand -hex 32)

while IFS= read -r line || [[ -n "$line" ]]; do
  case "$line" in
    POSTGRES_PASSWORD=*) printf 'POSTGRES_PASSWORD=%s\n' "$POSTGRES_PASSWORD" ;;
    REDIS_PASSWORD=*) printf 'REDIS_PASSWORD=%s\n' "$REDIS_PASSWORD" ;;
    ADMIN_PASSWORD=*) printf 'ADMIN_PASSWORD=%s\n' "$ADMIN_PASSWORD" ;;
    JWT_SECRET=*) printf 'JWT_SECRET=%s\n' "$JWT_SECRET" ;;
    TOTP_ENCRYPTION_KEY=*) printf 'TOTP_ENCRYPTION_KEY=%s\n' "$TOTP_ENCRYPTION_KEY" ;;
    *) printf '%s\n' "$line" ;;
  esac
done < "$TEMPLATE" > "$TEMP_FILE"

chmod 600 "$TEMP_FILE"
mv "$TEMP_FILE" "$TARGET"
trap - EXIT
printf 'created %s\n' "$TARGET"
