#!/usr/bin/env bash
set -euo pipefail

# Read-only production gate. This script deliberately does not invoke Docker,
# migrations, systemctl, Caddy reloads, or any promotion command.

state_file=${SUB2API_RELEASE_STATE:-/var/lib/sub2api/release-state}
release_env=${SUB2API_RELEASE_ENV_FILE:-/opt/sub2api/production/release.env}

emit_failure() {
  local reason=$1
  jq -cn --arg reason "$reason" \
    '{status:"blocked",reason:$reason,update_performed:false,promotion_performed:false}'
  exit 1
}

command -v jq >/dev/null 2>&1 || emit_failure 'jq is required'
command -v awk >/dev/null 2>&1 || emit_failure 'awk is required'
command -v stat >/dev/null 2>&1 || emit_failure 'stat is required'

[[ -f "$state_file" && ! -L "$state_file" && -r "$state_file" ]] \
  || emit_failure 'release state is missing, symlinked, or unreadable'
[[ -f "$release_env" && ! -L "$release_env" && -r "$release_env" ]] \
  || emit_failure 'release.env is missing, symlinked, or unreadable'

mode_of() { stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"; }
[[ "$(mode_of "$state_file")" == 600 ]] || emit_failure 'release state must be mode 0600'
[[ "$(mode_of "$release_env")" == 600 ]] || emit_failure 'release.env must be mode 0600'

jq -e '
  type == "object" and
  (.schema_version == 1 or .schema_version == 2) and
  ((.active_slot == "blue" and .active_upstream == "sub2api-blue:8080") or
   (.active_slot == "green" and .active_upstream == "sub2api-green:8080")) and
  ([.blue_image,.green_image,.worker_image] | all(type == "string" and length > 0)) and
  (.source_commit | type == "string" and test("^[a-f0-9]{40}$")) and
  (.source_tree | type == "string" and test("^[a-f0-9]{40}$")) and
  (.migrations_hash | type == "string" and test("^[a-f0-9]{64}$"))
' "$state_file" >/dev/null 2>&1 || emit_failure 'release state schema or slot/upstream pair is invalid'

env_value() {
  local key=$1 count
  count=$(awk -F= -v key="$key" '$1 == key { count++ } END { print count + 0 }' "$release_env")
  [[ "$count" == 1 ]] || emit_failure "release.env must contain exactly one $key assignment"
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$release_env"
}

active_slot=$(jq -r '.active_slot' "$state_file")
active_upstream=$(jq -r '.active_upstream' "$state_file")
state_image=$(jq -r ".${active_slot}_image" "$state_file")
env_active_upstream=$(env_value SUB2API_ACTIVE_UPSTREAM)
env_blue_image=$(env_value SUB2API_BLUE_IMAGE)
env_green_image=$(env_value SUB2API_GREEN_IMAGE)
env_worker_image=$(env_value SUB2API_WORKER_IMAGE)

[[ "$env_active_upstream" == "$active_upstream" ]] \
  || emit_failure 'release.env active upstream does not match release state'
[[ "$env_blue_image" == "$(jq -r '.blue_image' "$state_file")" ]] \
  || emit_failure 'release.env blue image does not match release state'
[[ "$env_green_image" == "$(jq -r '.green_image' "$state_file")" ]] \
  || emit_failure 'release.env green image does not match release state'
[[ "$env_worker_image" == "$(jq -r '.worker_image' "$state_file")" ]] \
  || emit_failure 'release.env worker image does not match release state'

jq -cn \
  --arg state_file "$state_file" \
  --arg release_env "$release_env" \
  --arg active_slot "$active_slot" \
  --arg active_upstream "$active_upstream" \
  --arg active_image "$state_image" \
  --arg source_commit "$(jq -r '.source_commit' "$state_file")" \
  --arg source_tree "$(jq -r '.source_tree' "$state_file")" \
  --arg migrations_hash "$(jq -r '.migrations_hash' "$state_file")" \
  '{status:"ready",state_file:$state_file,release_env:$release_env,
    active_slot:$active_slot,active_upstream:$active_upstream,active_image:$active_image,
    source_commit:$source_commit,source_tree:$source_tree,migrations_hash:$migrations_hash,
    update_performed:false,promotion_performed:false}'
