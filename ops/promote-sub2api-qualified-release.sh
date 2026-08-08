#!/usr/bin/env bash
set -euo pipefail
state=${RELEASE_STATE_FILE:-/var/lib/sub2api/release-state/qualification.json}
[[ -f "$state" ]] || { echo 'qualification state missing' >&2; exit 1; }
grep -q '"stage":"ready"' "$state" || { echo 'release is not ready' >&2; exit 1; }
echo 'promotion requires reviewed host executor; no traffic change performed'

