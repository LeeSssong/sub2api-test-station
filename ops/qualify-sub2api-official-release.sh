#!/usr/bin/env bash
set -euo pipefail
target=${1:?target version required}
[[ "$target" != *$'\n'* && "$target" != *$'\r'* && "$target" != *' '* ]] || { echo 'invalid target version' >&2; exit 1; }
state=${RELEASE_STATE_FILE:-/var/lib/sub2api/release-state/qualification.json}
report_command=${QUALIFICATION_COMMAND:-}
[[ -n "$report_command" && "$report_command" = /* && "$report_command" != *$'\n'* ]] || {
  echo 'QUALIFICATION_COMMAND must be an absolute reviewed host command' >&2
  exit 1
}
command -v jq >/dev/null 2>&1 || { echo 'jq is required for qualification evidence' >&2; exit 1; }
mkdir -p "$(dirname "$state")"
tmp=$(mktemp "${state}.XXXXXX")
trap 'rm -f "$tmp"' EXIT
started=$(date -u +%FT%TZ)
printf '{"stage":"qualifying","tag":%s,"started_at":%s}\n' "$(jq -cn --arg v "$target" '$v')" "$(jq -cn --arg v "$started" '$v')" >"$tmp"
mv "$tmp" "$state"
report=$($report_command "$target")
finished=$(date -u +%FT%TZ)
echo "$report" | jq -e --arg target "$target" '
  type == "object" and .tag == $target and
  (.commit | type == "string" and length > 0) and
  (.asset | type == "string" and length > 0) and
  (.checksum | test("^[A-Fa-f0-9]{64}$")) and
  (.adapter_version | type == "string" and length > 0) and
  (.contract_version | type == "number" and . > 0) and
  (.migration_class == "expand_only") and
  (.tests | type == "array" and length > 0) and
  (.data_diff | type == "string" and length > 0) and
  (.passed == true) and
  (.stable_failure // "" == "")' >/dev/null || {
    printf '{"stage":"blocked","tag":%s,"reason":"qualification_evidence_invalid","finished_at":%s}\n' "$(jq -cn --arg v "$target" '$v')" "$(jq -cn --arg v "$finished" '$v')" >"$state"
    exit 1
  }
printf '%s\n' "$report" | jq --arg finished "$finished" --arg started "$started" '
  .stage = "ready" | .started_at = $started | .finished_at = $finished | .ready_until = ((now + 3600) | todateiso8601)' >"$state"
chmod 600 "$state"
