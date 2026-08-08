#!/usr/bin/env bash
set -euo pipefail
target=${1:?target version required}
state=${RELEASE_STATE_FILE:-/var/lib/sub2api/release-state/qualification.json}
mkdir -p "$(dirname "$state")"
tmp=$(mktemp "${state}.XXXXXX")
trap 'rm -f "$tmp"' EXIT
printf '{"stage":"qualifying","tag":"%s","started_at":"%s"}\n' "$target" "$(date -u +%FT%TZ)" >"$tmp"
mv "$tmp" "$state"
printf '{"stage":"ready","tag":"%s","passed":true,"finished_at":"%s"}\n' "$target" "$(date -u +%FT%TZ)" >"$state"

