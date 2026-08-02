#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'billing_provision status=failed\n' >&2
  exit 1
}

mode_of() { stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"; }

[[ "$(uname -s)" == Linux ]] || fail
[[ ${EUID:-$(id -u)} -eq 0 ]] || fail
[[ -z ${DOCKER_HOST:-} ]] || fail
command -v docker >/dev/null 2>&1 || fail
[[ "$(docker context show)" == default ]] || fail
[[ $# -eq 2 && $1 == --declaration ]] || fail

declaration=$2
[[ "$declaration" == /* && -f "$declaration" && ! -L "$declaration" ]] || fail
[[ "$(mode_of "$declaration")" == 600 ]] || fail
declaration_dir=$(dirname "$declaration")
[[ -d "$declaration_dir" && ! -L "$declaration_dir" ]] || fail
declaration_dir=$(cd "$declaration_dir" && pwd -P)
declaration="$declaration_dir/$(basename "$declaration")"

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
[[ -f "$root/infra/compose.yaml" && ! -L "$root/infra/compose.yaml" ]] || fail

RELAY_OPS_BILLING_SOURCE_DECLARATION_HOST_FILE="$declaration" \
  docker compose --project-directory "$root" --profile provision run --rm --no-deps relay-ops-provision
