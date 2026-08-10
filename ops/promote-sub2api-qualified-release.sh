#!/usr/bin/env bash
set -euo pipefail
state=${RELEASE_STATE_FILE:-/var/lib/sub2api/release-state/qualification.json}
[[ -f "$state" ]] || { echo 'qualification state missing' >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo 'jq is required for promotion gate' >&2; exit 1; }
jq -e 'type == "object" and .stage == "ready" and .passed == true and (.ready_until | fromdateiso8601 > now)' "$state" >/dev/null || { echo 'release is not ready or readiness expired' >&2; exit 1; }
executor=${SUB2API_HOST_EXECUTOR:-}
[[ -n "$executor" && "$executor" = /* && -x "$executor" ]] || { echo 'SUB2API_HOST_EXECUTOR must be an executable reviewed host executor' >&2; exit 1; }
exec "$executor" --qualification-state "$state"
