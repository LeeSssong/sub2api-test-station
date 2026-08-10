#!/usr/bin/env bash
set -euo pipefail
executor=${SUB2API_HOST_EXECUTOR:-}
[[ -n "$executor" && "$executor" = /* && -x "$executor" ]] || { echo 'SUB2API_HOST_EXECUTOR must be an executable reviewed host executor' >&2; exit 1; }
exec "$executor" --rollback
