#!/usr/bin/env bash
set -euo pipefail

umask 077

fail() {
  printf 'blue-green deploy failed: %s\n' "$1" >&2
  exit 1
}

monotonic_millis() {
  perl -MTime::HiRes=clock_gettime,CLOCK_MONOTONIC -e \
    'printf "%.0f\n", clock_gettime(CLOCK_MONOTONIC) * 1000'
}

trace_event() {
  [[ -n "${RELEASE_EVENT_LOG:-}" ]] || return 0
  if [[ "${maintenance_stopped:-false}" == true || "${rollback_in_progress:-false}" == true ]]; then
    run_post_stop_operation 'printf "%s\n" "$1" >>"$2"' "$1" "$RELEASE_EVENT_LOG"
    return
  fi
  printf '%s\n' "$1" >>"$RELEASE_EVENT_LOG"
}

mode=''
requested_image=''
source_commit=''
source_tree=''
tested_tree=''
migrations_hash=''
deadline_epoch=''
maintenance_authorized=false
maintenance_from_hash=''
preloaded_archive=''
preloaded_archive_sha256=''
preloaded_image_id=''

# Only these migration transitions are permitted by the maintenance path.
# The hashes cover complete normalized migration sets, so any other change
# fails closed before production is stopped.
readonly MAINTENANCE_1_OLD_MIGRATIONS_HASH=ac8b0b33d7ea31a1a4f0117716ba56efec4bd66be9c38267a88d4c512d01bf39
readonly MAINTENANCE_1_NEW_MIGRATIONS_HASH=0204f39423f3218ffa0c8d4e3d665f7113c4990610e0dd22e9f5910c4d578c6d
readonly MAINTENANCE_2_OLD_MIGRATIONS_HASH=aee795202a3dd14c191c5e395add6beb58942950bf530d9961ae80a359998429
readonly MAINTENANCE_2_NEW_MIGRATIONS_HASH=5cc825b23a35f64ecb2b2def9ae73170c7c512015f112d0773ba232e5ab85703
readonly MAINTENANCE_3_OLD_MIGRATIONS_HASH=5cc825b23a35f64ecb2b2def9ae73170c7c512015f112d0773ba232e5ab85703
readonly MAINTENANCE_3_NEW_MIGRATIONS_HASH=9caff81ff628266bf6cdcdf21aac716b1fa400a37681cfc5921845cf2ec3aad0
readonly MAINTENANCE_4_OLD_MIGRATIONS_HASH=9caff81ff628266bf6cdcdf21aac716b1fa400a37681cfc5921845cf2ec3aad0
readonly MAINTENANCE_4_NEW_MIGRATIONS_HASH=1f47135fedc31788d5ea690ec7f2dbb2dcac7b743a46bc50305143b621b5ee98
readonly MAINTENANCE_5_OLD_MIGRATIONS_HASH=1f47135fedc31788d5ea690ec7f2dbb2dcac7b743a46bc50305143b621b5ee98
readonly MAINTENANCE_5_NEW_MIGRATIONS_HASH=fadb98d43e3d8e8b41178203638912cc32592a1368091e4cb44399926daead5d
readonly MAINTENANCE_6_OLD_MIGRATIONS_HASH=fadb98d43e3d8e8b41178203638912cc32592a1368091e4cb44399926daead5d
readonly MAINTENANCE_6_NEW_MIGRATIONS_HASH=f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc
readonly MAINTENANCE_7_OLD_MIGRATIONS_HASH=f1b1f3537d518c30dc2fe99d75e9f2d7a5a27452f59ce4a50a1e81277c8cfbcc
readonly MAINTENANCE_7_NEW_MIGRATIONS_HASH=6a0e141eb4788460a99fc3e108ce5b46c866fd2c45b9a7265ea66b0ef8faaf71
readonly MAINTENANCE_8_OLD_MIGRATIONS_HASH=6a0e141eb4788460a99fc3e108ce5b46c866fd2c45b9a7265ea66b0ef8faaf71
readonly MAINTENANCE_8_NEW_MIGRATIONS_HASH=d3fe99bba69b0cf0cca8a7f5ec45499921f3496f58dd74c3a671d90a653589b5
readonly MAINTENANCE_9_OLD_MIGRATIONS_HASH=d3fe99bba69b0cf0cca8a7f5ec45499921f3496f58dd74c3a671d90a653589b5
readonly MAINTENANCE_9_NEW_MIGRATIONS_HASH=ef1213846cba597cbc5cd64238558a3c392585df3568acb321f3227776e88bc5
readonly MAINTENANCE_10_OLD_MIGRATIONS_HASH=ef1213846cba597cbc5cd64238558a3c392585df3568acb321f3227776e88bc5
readonly MAINTENANCE_10_NEW_MIGRATIONS_HASH=aaebed88f7fb712e1f518e73cc89bd44eb214f365f3b49f003598c93883a4604
readonly MAINTENANCE_11_OLD_MIGRATIONS_HASH=aaebed88f7fb712e1f518e73cc89bd44eb214f365f3b49f003598c93883a4604
readonly MAINTENANCE_11_NEW_MIGRATIONS_HASH=bb6ebff31f0ffe9be5ad204ba79ef896d98522ccdd7b3933843c94d6c9ad5951
readonly MAINTENANCE_12_OLD_MIGRATIONS_HASH=bb6ebff31f0ffe9be5ad204ba79ef896d98522ccdd7b3933843c94d6c9ad5951
readonly MAINTENANCE_12_NEW_MIGRATIONS_HASH=18c4ac1fc83294634c42c6d08c6511c01515406f296d40b54840f3dae726949f
readonly MAINTENANCE_13_OLD_MIGRATIONS_HASH=18c4ac1fc83294634c42c6d08c6511c01515406f296d40b54840f3dae726949f
readonly MAINTENANCE_13_NEW_MIGRATIONS_HASH=6f09256a674503b164a359a1a6d5245a530c53d2389fe7861fdd363403c7dc20
readonly MAINTENANCE_14_OLD_MIGRATIONS_HASH=6f09256a674503b164a359a1a6d5245a530c53d2389fe7861fdd363403c7dc20
readonly MAINTENANCE_14_NEW_MIGRATIONS_HASH=2b656ebf94fac6e81a1630d40561eccf105b5925ac939c0c6e87181bd20ea4c9
readonly MAINTENANCE_15_OLD_MIGRATIONS_HASH=18c4ac1fc83294634c42c6d08c6511c01515406f296d40b54840f3dae726949f
readonly MAINTENANCE_15_NEW_MIGRATIONS_HASH=2b656ebf94fac6e81a1630d40561eccf105b5925ac939c0c6e87181bd20ea4c9
readonly MAINTENANCE_16_OLD_MIGRATIONS_HASH=2b656ebf94fac6e81a1630d40561eccf105b5925ac939c0c6e87181bd20ea4c9
readonly MAINTENANCE_16_NEW_MIGRATIONS_HASH=59628d84dd909c8a91949eab2015dc216a8fe76027a2bcc8c996b504eb055e80

while (($#)); do
  case "$1" in
    --mode) (($# >= 2)) || fail '--mode requires a value'; [[ -z "$mode" ]] || fail '--mode may be supplied once'; mode=$2; shift 2 ;;
    --image) (($# >= 2)) || fail '--image requires a value'; [[ -z "$requested_image" ]] || fail '--image may be supplied once'; requested_image=$2; shift 2 ;;
    --source-commit) (($# >= 2)) || fail '--source-commit requires a value'; [[ -z "$source_commit" ]] || fail '--source-commit may be supplied once'; source_commit=$2; shift 2 ;;
    --source-tree) (($# >= 2)) || fail '--source-tree requires a value'; [[ -z "$source_tree" ]] || fail '--source-tree may be supplied once'; source_tree=$2; shift 2 ;;
    --tested-tree) (($# >= 2)) || fail '--tested-tree requires a value'; [[ -z "$tested_tree" ]] || fail '--tested-tree may be supplied once'; tested_tree=$2; shift 2 ;;
    --migrations-hash) (($# >= 2)) || fail '--migrations-hash requires a value'; [[ -z "$migrations_hash" ]] || fail '--migrations-hash may be supplied once'; migrations_hash=$2; shift 2 ;;
		--deadline-epoch) (($# >= 2)) || fail '--deadline-epoch requires a value'; [[ -z "$deadline_epoch" ]] || fail '--deadline-epoch may be supplied once'; deadline_epoch=$2; shift 2 ;;
		--preloaded-archive) (($# >= 2)) || fail '--preloaded-archive requires a value'; [[ -z "$preloaded_archive" ]] || fail '--preloaded-archive may be supplied once'; preloaded_archive=$2; shift 2 ;;
		--preloaded-archive-sha256) (($# >= 2)) || fail '--preloaded-archive-sha256 requires a value'; [[ -z "$preloaded_archive_sha256" ]] || fail '--preloaded-archive-sha256 may be supplied once'; preloaded_archive_sha256=$2; shift 2 ;;
		--preloaded-image-id) (($# >= 2)) || fail '--preloaded-image-id requires a value'; [[ -z "$preloaded_image_id" ]] || fail '--preloaded-image-id may be supplied once'; preloaded_image_id=$2; shift 2 ;;
		--maintenance-authorized) [[ "$maintenance_authorized" == false ]] || fail '--maintenance-authorized may be supplied once'; maintenance_authorized=true; shift ;;
		--maintenance-from-hash) (($# >= 2)) || fail '--maintenance-from-hash requires a value'; [[ -z "$maintenance_from_hash" ]] || fail '--maintenance-from-hash may be supplied once'; maintenance_from_hash=$2; shift 2 ;;
    *) fail "unknown argument: $1" ;;
  esac
done

[[ "$mode" == rehearsal || "$mode" == production ]] || fail '--mode must be rehearsal or production'
[[ "$maintenance_authorized" == false || "$mode" == production ]] || fail '--maintenance-authorized is only valid in production mode'
[[ -z "$maintenance_from_hash" || "$maintenance_from_hash" =~ ^[a-f0-9]{64}$ ]] || fail '--maintenance-from-hash must be 64 lowercase hex'
if [[ "$maintenance_authorized" == true ]]; then
  [[ "$maintenance_from_hash" =~ ^[a-f0-9]{64}$ ]] \
    || fail '--maintenance-from-hash must identify an approved active migration set'
fi

approved_maintenance_transition() {
  local from_hash=$1 to_hash=$2
  [[ "$from_hash" == "$MAINTENANCE_1_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_1_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_2_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_2_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_3_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_3_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_4_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_4_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_5_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_5_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_6_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_6_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_7_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_7_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_8_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_8_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_9_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_9_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_10_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_10_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_11_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_11_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_12_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_12_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_13_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_13_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_14_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_14_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_15_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_15_NEW_MIGRATIONS_HASH" \
    || "$from_hash" == "$MAINTENANCE_16_OLD_MIGRATIONS_HASH" && "$to_hash" == "$MAINTENANCE_16_NEW_MIGRATIONS_HASH" ]]
}
preloaded_image=${RELEASE_PRELOADED_IMAGE:-false}
[[ "$preloaded_image" == true || "$preloaded_image" == false ]] \
  || fail 'RELEASE_PRELOADED_IMAGE must be true or false'
release_staging_root=${RELEASE_STAGING_ROOT:-/var/lib/sub2api/release-staging}
[[ "$release_staging_root" == /* && "$release_staging_root" != */ && ! -L "$release_staging_root" ]] \
  || fail 'RELEASE_STAGING_ROOT is invalid'
if [[ "$preloaded_image" == true ]]; then
  [[ "$requested_image" =~ ^[^[:space:]@]+:release-[a-f0-9]{40}-[a-f0-9]{64}$ ]] \
    || fail '--image must be an image-ID-bound release tag in preloaded mode'
  [[ "$preloaded_archive" == "$release_staging_root"/* && "$(basename "$preloaded_archive")" =~ ^[A-Za-z0-9._-]+\.tar$ ]] \
    || fail '--preloaded-archive is invalid'
  [[ "$preloaded_archive_sha256" =~ ^[a-f0-9]{64}$ ]] \
    || fail '--preloaded-archive-sha256 is invalid'
  [[ "$preloaded_image_id" =~ ^sha256:[a-f0-9]{64}$ ]] \
    || fail '--preloaded-image-id is invalid'
else
  [[ "$requested_image" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]] \
    || fail '--image must be an immutable repository sha256 digest'
  [[ -z "$preloaded_archive$preloaded_archive_sha256$preloaded_image_id" ]] \
    || fail 'preloaded arguments require RELEASE_PRELOADED_IMAGE=true'
fi
[[ "$source_commit" =~ ^[a-f0-9]{40}$ ]] || fail '--source-commit must be 40 lowercase hex'
[[ "$source_tree" =~ ^[a-f0-9]{40}$ ]] || fail '--source-tree must be 40 lowercase hex'
[[ "$tested_tree" =~ ^[a-f0-9]{40}$ ]] || fail '--tested-tree must be 40 lowercase hex'
[[ "$migrations_hash" =~ ^[a-f0-9]{64}$ ]] || fail '--migrations-hash must be 64 lowercase hex'
[[ "$deadline_epoch" =~ ^[1-9][0-9]{9}$ ]] || fail '--deadline-epoch must be a Unix epoch'
[[ "$source_tree" == "$tested_tree" ]] || fail 'source tree does not equal tested tree'
if [[ "$preloaded_image" == true ]]; then
  [[ "$requested_image" == *":release-$source_commit-${preloaded_image_id#sha256:}" ]] \
    || fail 'preloaded release tag does not match source commit and image ID'
fi

for required_command in docker curl jq df awk date stat mktemp find sort uniq chmod mv mkdir cp tr grep rm dirname basename sleep perl head od; do
  command -v "$required_command" >/dev/null 2>&1 || fail "$required_command is required"
done
if [[ "$preloaded_image" == true ]]; then
  command -v sha256sum >/dev/null 2>&1 || command -v shasum >/dev/null 2>&1 \
    || fail 'sha256sum or shasum is required for preloaded releases'
fi

check_deadline() {
	local now
	now=$(date -u +%s) || fail 'release deadline clock failed'
	[[ "$now" =~ ^[0-9]+$ && "$now" -lt "$deadline_epoch" ]] || fail 'release exceeded its end-to-end deadline'
}

check_maintenance_deadline() {
  [[ -z "$maintenance_deadline_epoch" ]] && return 0
  local now elapsed_millis elapsed remaining_window
  if [[ "${maintenance_started_millis:-}" =~ ^[0-9]+$ && "${maintenance_window_seconds:-}" =~ ^[1-9][0-9]*$ ]]; then
    elapsed_millis=$(( $(monotonic_millis) - maintenance_started_millis ))
    elapsed=$((elapsed_millis / 1000))
    remaining_window=$((maintenance_window_seconds - elapsed))
    (( remaining_window > 0 )) || fail 'maintenance unavailable window expired'
  fi
  now=$(date -u +%s) || fail 'maintenance deadline clock failed'
  [[ "$now" =~ ^[0-9]+$ && "$now" -lt "$maintenance_deadline_epoch" ]] \
    || fail 'maintenance unavailable window expired'
}

run_post_stop_command() {
  local deadline now remaining elapsed_millis elapsed remaining_window phase_window_seconds=''
  if [[ -z "${maintenance_deadline_epoch:-}" && "${rollback_in_progress:-false}" != true ]]; then
    "$@"
    return
  fi
  deadline=$deadline_epoch
  if [[ -n "${maintenance_deadline_epoch:-}" ]]; then
    deadline=$maintenance_forward_deadline_epoch
    phase_window_seconds=$maintenance_forward_window_seconds
    if [[ "${rollback_in_progress:-false}" == true ]]; then
      deadline=$maintenance_rollback_deadline_epoch
      phase_window_seconds=$maintenance_rollback_window_seconds
    fi
    if [[ "${finalization_in_progress:-false}" == true ]]; then
      deadline=$maintenance_deadline_epoch
      phase_window_seconds=$maintenance_window_seconds
    fi
  fi
  if [[ -n "$phase_window_seconds" && "${maintenance_started_millis:-}" =~ ^[0-9]+$ ]]; then
    elapsed_millis=$(( $(monotonic_millis) - maintenance_started_millis ))
    elapsed=$((elapsed_millis / 1000))
    remaining_window=$((phase_window_seconds - elapsed))
    (( remaining_window > 0 )) || return 124
  fi
  now=$(date -u +%s) || return 1
  [[ "$now" =~ ^[0-9]+$ ]] || return 1
  remaining=$((deadline - now))
  if [[ -n "$phase_window_seconds" && "${maintenance_started_millis:-}" =~ ^[0-9]+$ ]]; then
    elapsed_millis=$(( $(monotonic_millis) - maintenance_started_millis ))
    elapsed=$((elapsed_millis / 1000))
    remaining_window=$((phase_window_seconds - elapsed))
    (( remaining_window < remaining )) && remaining=$remaining_window
  fi
  (( remaining > 0 )) || return 124
  # Run each post-stop operation in its own process group.  A plain
  # `alarm/exec` watchdog only terminates the wrapper process; a hung
  # docker/curl child can keep inherited pipes open and exceed the maintenance
  # window.  The parent timer therefore terminates the whole group and waits
  # for it before returning a timeout status.
  perl -MPOSIX=setpgid,WIFEXITED,WEXITSTATUS -e '
    my $timeout = shift @ARGV;
    my $pid = fork();
    die "fork failed: $!" unless defined $pid;
    if ($pid == 0) {
      setpgid(0, 0) or die "setpgid failed: $!";
      exec @ARGV or die "exec failed: $!";
    }
    $SIG{ALRM} = sub {
      kill "TERM", -$pid;
      select undef, undef, undef, 0.1;
      kill "KILL", -$pid;
      waitpid($pid, 0);
      exit 124;
    };
    alarm $timeout;
    waitpid($pid, 0);
    alarm 0;
    if (WIFEXITED($?)) { exit WEXITSTATUS($?); }
    exit 1;
  ' "$remaining" "$@"
}

run_post_stop_operation() {
  local operation=$1
  shift
  run_post_stop_command bash -o pipefail -c "$operation" sub2api-bounded-operation "$@"
}

parent_pid=$$
deadline_watchdog_pid=''

stop_deadline_watchdog() {
	if [[ -n "${deadline_watchdog_pid:-}" ]]; then
		kill "$deadline_watchdog_pid" 2>/dev/null || true
		wait "$deadline_watchdog_pid" 2>/dev/null || true
		deadline_watchdog_pid=''
	fi
}

arm_deadline_watchdog() {
  local seconds=$1
  [[ "$seconds" =~ ^[1-9][0-9]*$ ]] || fail 'release deadline watchdog budget is invalid'
  stop_deadline_watchdog
  perl -e '($pid, $seconds) = @ARGV; sleep $seconds; kill "TERM", $pid' "$parent_pid" "$seconds" &
  deadline_watchdog_pid=$!
}

check_deadline
deadline_remaining=$((deadline_epoch - $(date -u +%s)))
(( deadline_remaining > 0 )) || fail 'release exceeded its end-to-end deadline'
release_deadline_window_seconds=$deadline_remaining
release_deadline_started_millis=$(monotonic_millis)
arm_deadline_watchdog "$deadline_remaining"
trap stop_deadline_watchdog EXIT

canonical_directory() {
  local value=$1 label=$2 physical
  [[ "$value" == /* && -d "$value" && ! -L "$value" ]] || fail "$label must be an absolute non-symlink directory"
  physical=$(cd "$value" && pwd -P)
  [[ "$physical" == "$value" ]] || fail "$label must be canonical"
  printf '%s\n' "$value"
}

canonical_file() {
  local value=$1 label=$2 parent physical canonical
  [[ "$value" == /* && -f "$value" && -r "$value" && ! -L "$value" ]] || fail "$label must be an absolute readable non-symlink file"
  parent=$(dirname "$value")
  physical=$(cd "$parent" && pwd -P)
  canonical="$physical/$(basename "$value")"
  [[ "$canonical" == "$value" ]] || fail "$label must be canonical"
  printf '%s\n' "$value"
}

canonical_optional_file_path() {
  local value=$1 label=$2 parent physical canonical
  [[ "$value" == /* && ! -L "$value" ]] || fail "$label must be absolute and non-symlinked"
  parent=$(dirname "$value")
  [[ -d "$parent" && ! -L "$parent" ]] || fail "$label parent must be a non-symlink directory"
  physical=$(cd "$parent" && pwd -P)
  canonical="$physical/$(basename "$value")"
  [[ "$canonical" == "$value" ]] || fail "$label must be canonical"
  printf '%s\n' "$value"
}

mode_of() {
  stat -c '%a' "$1" 2>/dev/null || stat -f '%Lp' "$1"
}

owner_of() {
  stat -c '%u' "$1" 2>/dev/null || stat -f '%u' "$1"
}

secure_directory() {
  local value=$1 label=$2 mode
  canonical_directory "$value" "$label" >/dev/null
  [[ "$(owner_of "$value")" == 0 ]] || fail "$label must be root-owned"
  mode=$(mode_of "$value")
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || fail "$label mode is invalid"
  (( (8#$mode & 8#022) == 0 )) || fail "$label must not be group/other writable"
}

secure_file() {
  local value=$1 label=$2 mode
  canonical_file "$value" "$label" >/dev/null
  [[ "$(owner_of "$value")" == 0 ]] || fail "$label must be root-owned"
  mode=$(mode_of "$value")
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || fail "$label mode is invalid"
  (( (8#$mode & 8#022) == 0 )) || fail "$label must not be group/other writable"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

deploy_root=$(canonical_directory "${DEPLOY_ROOT:?DEPLOY_ROOT is required}" 'DEPLOY_ROOT')
base_compose=$(canonical_file "${BASE_COMPOSE:?BASE_COMPOSE is required}" 'BASE_COMPOSE')
secret_env=$(canonical_file "${SECRET_ENV:?SECRET_ENV is required}" 'SECRET_ENV')
release_env=$(canonical_file "${RELEASE_ENV:?RELEASE_ENV is required}" 'RELEASE_ENV')
release_state=$(canonical_optional_file_path "${RELEASE_STATE:?RELEASE_STATE is required}" 'RELEASE_STATE')
record_root=$(canonical_directory "${RELEASE_RECORD_ROOT:?RELEASE_RECORD_ROOT is required}" 'RELEASE_RECORD_ROOT')
admin_key_file=$(canonical_file "${ADMIN_API_KEY_FILE:?ADMIN_API_KEY_FILE is required}" 'ADMIN_API_KEY_FILE')
gateway_key_file=$(canonical_file "${GATEWAY_API_KEY_FILE:?GATEWAY_API_KEY_FILE is required}" 'GATEWAY_API_KEY_FILE')
base_url=${BASE_URL:?BASE_URL is required}

[[ "$(mode_of "$release_env")" == 600 ]] || fail 'RELEASE_ENV mode must be 0600'
[[ "$(mode_of "$secret_env")" == 600 ]] || fail 'SECRET_ENV mode must be 0600'
[[ "$(mode_of "$admin_key_file")" == 600 ]] || fail 'ADMIN_API_KEY_FILE mode must be 0600'
[[ "$(mode_of "$gateway_key_file")" == 600 ]] || fail 'GATEWAY_API_KEY_FILE mode must be 0600'

if [[ "$mode" == production ]]; then
	[[ "$base_url" == https://* ]] || fail 'production BASE_URL must be HTTPS'
  [[ "$(uname -s)" == Linux ]] || fail 'production deployment must run on Linux'
  [[ -z "${DOCKER_HOST:-}" ]] || fail 'production deployment must not use DOCKER_HOST'
  [[ "${DOCKER_CONTEXT:-default}" == default ]] || fail 'production DOCKER_CONTEXT must be default'
  [[ "$(docker context show)" == default ]] || fail 'production Docker context must be default'
  compose_project=${COMPOSE_PROJECT_NAME:-sub2api}
  [[ "$compose_project" == sub2api ]] || fail 'production COMPOSE_PROJECT_NAME must be sub2api'
else
  compose_project=${COMPOSE_PROJECT_NAME:-sub2api-blue-green-rehearsal}
	[[ "$compose_project" == sub2api-blue-green-rehearsal ]] \
		|| fail 'rehearsal COMPOSE_PROJECT_NAME must be sub2api-blue-green-rehearsal'
	rehearsal_root=$(canonical_directory "${REHEARSAL_ROOT:?REHEARSAL_ROOT is required in rehearsal mode}" 'REHEARSAL_ROOT')
	[[ "$base_compose" == "$deploy_root/compose.sub2api-rehearsal.yaml" ]] \
		|| fail 'rehearsal BASE_COMPOSE must be the isolated rehearsal topology'
	case "$secret_env" in "$rehearsal_root"/*) ;; *) fail 'rehearsal SECRET_ENV must be inside REHEARSAL_ROOT' ;; esac
	case "$release_env" in "$rehearsal_root"/*) ;; *) fail 'rehearsal RELEASE_ENV must be inside REHEARSAL_ROOT' ;; esac
	case "$release_state" in "$rehearsal_root"/*) ;; *) fail 'rehearsal RELEASE_STATE must be inside REHEARSAL_ROOT' ;; esac
	case "$record_root" in "$rehearsal_root"/*) ;; *) fail 'rehearsal RELEASE_RECORD_ROOT must be inside REHEARSAL_ROOT' ;; esac
	case "$admin_key_file" in "$rehearsal_root"/*) ;; *) fail 'rehearsal ADMIN_API_KEY_FILE must be inside REHEARSAL_ROOT' ;; esac
	case "$gateway_key_file" in "$rehearsal_root"/*) ;; *) fail 'rehearsal GATEWAY_API_KEY_FILE must be inside REHEARSAL_ROOT' ;; esac
	[[ "$release_state" == "$record_root/release-state.json" ]] \
		|| fail 'rehearsal RELEASE_STATE must use the rehearsal record namespace'
	[[ "$base_url" =~ ^https?://(localhost|127\.0\.0\.1)(:[1-9][0-9]{0,4})?$ ]] \
		|| fail 'rehearsal BASE_URL must be localhost-only'
fi
caddy_config=''
if [[ "$mode" == production ]]; then
  caddy_config=$(canonical_file "$deploy_root/Caddyfile" 'CADDY_CONFIG')
fi

network_curl_image=${NETWORK_CURL_IMAGE:-}
network_curl_allowlist=${NETWORK_CURL_IMAGE_ALLOWLIST:-}
if [[ "$mode" == production ]]; then
  [[ "$network_curl_image" =~ ^[^[:space:]@]+@sha256:[a-f0-9]{64}$ ]] \
    || fail 'production NETWORK_CURL_IMAGE must be an approved immutable sha256 digest'
  [[ -n "$network_curl_allowlist" ]] || fail 'production NETWORK_CURL_IMAGE_ALLOWLIST is required'
  network_curl_approved=false
  while IFS= read -r approved_network_curl_image; do
    [[ -z "$approved_network_curl_image" ]] && continue
    [[ "$approved_network_curl_image" == "$network_curl_image" ]] && network_curl_approved=true
  done <<<"$network_curl_allowlist"
  [[ "$network_curl_approved" == true ]] || fail 'production NETWORK_CURL_IMAGE is not allowlisted'
else
  network_curl_image=${network_curl_image:-curlimages/curl:8.12.1}
fi
network_probe_pull_args=()
if [[ "$preloaded_image" == true ]]; then
  network_probe_pull_args=(--pull never)
fi

lock_dir="$record_root/.blue-green.lock"
lock_owner_path="$lock_dir/owner.pid"
lock_owned=false

cleanup_lock() {
	stop_deadline_watchdog
  if [[ "${lock_owned:-false}" == true ]]; then
    if run_post_stop_operation 'rm -f -- "$1" && rmdir "$2"' "$lock_owner_path" "$lock_dir" 2>/dev/null; then
      lock_owned=false
    fi
  fi
}

persist_lock_owner() {
  local temporary="$lock_dir/.owner.pid.$$"
  if ! { printf '%s\n' "$$" >"$temporary" && chmod 0600 "$temporary" && mv "$temporary" "$lock_owner_path"; }; then
    rm -f -- "$temporary"
    rmdir "$lock_dir" 2>/dev/null || true
    lock_owned=false
    fail 'could not persist blue-green lock ownership'
  fi
}

acquire_lock() {
  local owner_pid
  if mkdir "$lock_dir" 2>/dev/null; then
    lock_owned=true
    persist_lock_owner
    return 0
  fi

  [[ -d "$lock_dir" && ! -L "$lock_dir" ]] || fail 'blue-green release lock is invalid'
  if [[ ! -e "$lock_owner_path" ]]; then
    fail 'blue-green release lock owner is missing; manual recovery is required'
  fi
  [[ -f "$lock_owner_path" && ! -L "$lock_owner_path" ]] || fail 'blue-green release lock owner is invalid'
  [[ "$(mode_of "$lock_owner_path")" == 600 ]] || fail 'blue-green release lock owner mode must be 0600'
  owner_pid=$(awk 'NR == 1 { owner=$0 } END { if (NR != 1) exit 1; print owner }' "$lock_owner_path") \
    || fail 'blue-green release lock owner is invalid'
  [[ "$owner_pid" =~ ^[1-9][0-9]*$ ]] || fail 'blue-green release lock owner is invalid'
  if kill -0 "$owner_pid" 2>/dev/null; then
    fail 'another blue-green release is in progress'
  fi
  fail 'blue-green release lock owner is stale; manual recovery is required'
}

acquire_lock
partial_path=''
record_finalized=false
cutover_attempted=false
cutover_applied=false
state_persisted=false
persistence_started=false
worker_update_started=false
maintenance_transition=false
maintenance_stopped=false
maintenance_identity_refresh=false
maintenance_deadline_epoch=''
maintenance_window_seconds=''
maintenance_started_millis=''
maintenance_forward_deadline_epoch=''
maintenance_forward_window_seconds=''
maintenance_rollback_deadline_epoch=''
maintenance_rollback_window_seconds=''
rollback_in_progress=false
finalization_in_progress=false
rollback_completed=false
failure_reason='unexpected_exit'
candidate_slot=''
candidate_upstream=''
candidate_image_id=''
previous_slot=''
previous_upstream=''
previous_worker_image=''
previous_worker_image_id=''
rollback_blue_image=''
rollback_green_image=''
rollback_blue_image_id=''
rollback_green_image_id=''
rollback_active_image_id=''
rollback_source_commit=''
rollback_source_tree=''
rollback_migrations_hash=''
rollback_postgres_id=''
rollback_redis_id=''
rollback_caddy_id=''
candidate_env=''
rollback_env=''
admin_header=''
gateway_header=''
attempt_id="$(date -u +%Y%m%dT%H%M%SZ)-$mode-$$"
started_epoch=$(date -u +%s)
record_path="$record_root/$attempt_id.json"

detector_compose_backup=''
detector_secret_backup=''
detector_topology_changed=false
detector_enabled=false
[[ "$mode" == rehearsal ]] && detector_enabled=true

restore_detector_topology() {
  [[ "$detector_topology_changed" == true ]] || return 0
  if [[ -n "$detector_compose_backup" && -f "$detector_compose_backup" ]]; then
    cp "$detector_compose_backup" "$base_compose" && chmod 0600 "$base_compose"
  fi
  if [[ -n "$detector_secret_backup" && -f "$detector_secret_backup" ]]; then
    cp "$detector_secret_backup" "$secret_env" && chmod 0600 "$secret_env"
  fi
  rm -f -- "$detector_compose_backup" "$detector_secret_backup"
  detector_compose_backup=''
  detector_secret_backup=''
  detector_topology_changed=false
}

configure_detector_topology() {
  [[ "$mode" == production ]] || return 0
  if ! jq -e 'type == "object" and (.services | type == "object")' "$base_compose" >/dev/null 2>&1; then
    grep -Eq '^[[:space:]]+model-detector:' "$base_compose" && detector_enabled=true
    return 0
  fi
  detector_compose_backup="$record_root/.$attempt_id.compose.backup"
  detector_secret_backup="$record_root/.$attempt_id.secret.backup"
  cp "$base_compose" "$detector_compose_backup" && chmod 0600 "$detector_compose_backup"
  cp "$secret_env" "$detector_secret_backup" && chmod 0600 "$detector_secret_backup"
  detector_topology_changed=true
  detector_token_count=$(awk -F= '$1 == "SUB2API_MODEL_DETECTOR_TOKEN" { count++ } END { print count + 0 }' "$secret_env")
  [[ "$detector_token_count" -le 1 ]] || fail 'secret environment contains duplicate detector tokens'
  detector_token=$(awk -F= '$1 == "SUB2API_MODEL_DETECTOR_TOKEN" { value=$0; sub(/^[^=]*=/, "", value); print value; exit }' "$secret_env")
  if [[ -z "$detector_token" ]]; then
    [[ "$detector_token_count" -eq 0 ]] || fail 'configured detector token is empty'
    detector_token=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d '[:space:]')
    [[ "$detector_token" =~ ^[a-f0-9]{64}$ ]] || fail 'could not generate detector token'
    printf '\nSUB2API_MODEL_DETECTOR_TOKEN=%s\n' "$detector_token" >>"$secret_env"
    chmod 0600 "$secret_env"
  fi
  [[ "$detector_token" =~ ^[a-f0-9]{64}$ ]] || fail 'configured detector token must be 64 lowercase hex'
  detector_models=${MODEL_DETECTOR_MODELS:-gpt-5.6-terra,gpt-5.6-sol,gpt-5.4,gpt-5.6,gpt-5.6-codex,claude-3-7-sonnet}
  detector_version=${MODEL_DETECTOR_VERSION:-4.1.1}
  temporary="$base_compose.tmp.$attempt_id"
  jq --arg image "$requested_image" --arg token "$detector_token" \
    --arg models "$detector_models" --arg version "$detector_version" '
    .services["model-detector"] = {
      "command": ["/app/model-detector"],
      "depends_on": {"postgres": {"condition": "service_healthy"}, "redis": {"condition": "service_healthy"}},
      "environment": {
        "MODEL_DETECTOR_LISTEN_ADDRESS": ":8090",
        "MODEL_DETECTOR_MODELS": $models,
        "MODEL_DETECTOR_VERSION": $version,
        "SUB2API_MODEL_DETECTOR_TOKEN": $token
      },
      "expose": ["8090"],
      "healthcheck": {"test": ["CMD", "wget", "-q", "-T", "5", "-O", "/dev/null", "http://localhost:8090/healthz"], "interval": "30s", "timeout": "5s", "retries": 3, "start_period": "10s"},
      "image": $image,
      "logging": {"driver": "json-file", "options": {"max-file": "5", "max-size": "20m"}},
      "networks": {"default": null},
      "restart": "unless-stopped"
    } |
    .services |= with_entries(
      if (.key == "sub2api-blue" or .key == "sub2api-green" or .key == "sub2api-worker") then
        .value.environment.SUB2API_MODEL_DETECTOR_URL = "http://model-detector:8090" |
        .value.environment.SUB2API_MODEL_DETECTOR_TOKEN = $token |
        .value.depends_on["model-detector"] = {"condition": "service_healthy"}
      else . end
    )
  ' "$base_compose" >"$temporary" || fail 'detector Compose patch failed'
  chmod 0600 "$temporary" && mv "$temporary" "$base_compose"
  detector_enabled=true
}

compose_current=(docker compose --project-name "$compose_project" --project-directory "$deploy_root"
  --env-file "$secret_env" --env-file "$release_env" -f "$base_compose")
compose_pull_args=()
[[ "$preloaded_image" == true ]] && compose_pull_args=(--pull never)
export SUB2API_RELEASE_ENV_FILE="$release_env"

run_caddy_config_command() {
  local upstream=$1 action=$2
  validate_upstream "$upstream" || return 1
  if [[ "$mode" == production ]]; then
    case "$action" in
      validate)
        run_post_stop_command "${compose_current[@]}" exec -T -e "SUB2API_ACTIVE_UPSTREAM=$upstream" caddy \
          caddy validate --config - --adapter caddyfile <"$caddy_config"
        ;;
      reload)
        run_post_stop_command "${compose_current[@]}" exec -T -e "SUB2API_ACTIVE_UPSTREAM=$upstream" caddy \
          caddy reload --config - --adapter caddyfile <"$caddy_config"
        ;;
      *) return 1 ;;
    esac
    return
  fi

  case "$action" in
    validate)
      run_post_stop_command "${compose_current[@]}" exec -T -e "SUB2API_ACTIVE_UPSTREAM=$upstream" caddy \
        caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
      ;;
    reload)
      run_post_stop_command "${compose_current[@]}" exec -T -e "SUB2API_ACTIVE_UPSTREAM=$upstream" caddy \
        caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
      ;;
    *) return 1 ;;
  esac
}

write_final_record() {
  local result=$1 state=$2 reason=$3 temporary
  [[ "$record_finalized" == false ]] || return 0
  temporary="$record_root/.$attempt_id.record.tmp"
  run_post_stop_operation '
    temporary=$1
    record_path=$2
    shift 2
    jq -n "$@" >"$temporary" && chmod 0600 "$temporary" && mv "$temporary" "$record_path"
  ' "$temporary" "$record_path" \
    --arg attempt_id "$attempt_id" \
    --arg mode "$mode" \
    --arg image "$requested_image" \
    --arg source_commit "$source_commit" \
    --arg source_tree "$source_tree" \
    --arg tested_tree "$tested_tree" \
    --arg migrations_hash "$migrations_hash" \
    --arg result "$result" \
    --arg state "$state" \
    --arg reason "$reason" \
    --argjson rolled_back "$rollback_completed" \
    '{schema_version:1, attempt_id:$attempt_id, mode:$mode,
      requested:{image:$image, source_commit:$source_commit, source_tree:$source_tree,
        tested_tree:$tested_tree, migrations_hash:$migrations_hash},
      result:$result, state:$state, reason:$reason, rolled_back:$rolled_back}'
  record_finalized=true
}

write_release_env_values_to() {
  local target=$1 blue_image=$2 green_image=$3 worker_image=$4 active_upstream=$5 active_slot=$6 previous=$7 temporary awk_program
  temporary="${target%/*}/.${target##*/}.$attempt_id.tmp"
  awk_program='
    BEGIN {
      values["SUB2API_BLUE_IMAGE"] = blue
      values["SUB2API_GREEN_IMAGE"] = green
      values["SUB2API_WORKER_IMAGE"] = worker
      values["SUB2API_ACTIVE_UPSTREAM"] = upstream
      values["SUB2API_ACTIVE_SLOT"] = active
      values["SUB2API_PREVIOUS_SLOT"] = previous
    }
    /^[[:space:]]*[A-Za-z_][A-Za-z0-9_]*[[:space:]]*=/ {
      key=$0
      sub(/^[[:space:]]*/, "", key)
      sub(/[[:space:]]*=.*/, "", key)
      if (key in values) {
        print key "=" values[key]
        seen[key]=1
        next
      }
    }
    { print }
    END {
      for (key in values) if (!(key in seen)) print key "=" values[key]
    }
  '
  run_post_stop_operation '
    awk_program=$1 temporary=$2 target=$3 release_env=$4 blue_image=$5 green_image=$6
    worker_image=$7 active_upstream=$8 active_slot=$9
    shift 9
    previous=$1
    awk \
      -v blue="$blue_image" -v green="$green_image" -v worker="$worker_image" \
      -v upstream="$active_upstream" -v active="$active_slot" -v previous="$previous" \
      "$awk_program" "$release_env" >"$temporary" &&
      chmod 0600 "$temporary" && mv "$temporary" "$target"
  ' "$awk_program" "$temporary" "$target" "$release_env" "$blue_image" "$green_image" \
    "$worker_image" "$active_upstream" "$active_slot" "$previous"
}

write_release_env_values() {
  write_release_env_values_to "$release_env" "$@"
}

write_state_values() {
  local active_slot=$1 active_upstream=$2 blue_image=$3 green_image=$4 worker_image=$5 \
    commit=$6 tree=$7 migrations=$8 postgres_id=$9
  shift 9
  local redis_id=$1 caddy_id=$2 blue_image_id=$3 green_image_id=$4 worker_image_id=$5 temporary
  temporary="${release_state%/*}/.${release_state##*/}.$attempt_id.tmp"
  if [[ ! "$blue_image_id" =~ ^sha256:[a-f0-9]{64}$ || ! "$green_image_id" =~ ^sha256:[a-f0-9]{64}$ || ! "$worker_image_id" =~ ^sha256:[a-f0-9]{64}$ ]]; then
    run_post_stop_operation '
      temporary=$1
      release_state=$2
      shift 2
      jq -n "$@" >"$temporary" && chmod 0600 "$temporary" && mv "$temporary" "$release_state"
    ' "$temporary" "$release_state" \
      --arg active_slot "$active_slot" --arg active_upstream "$active_upstream" \
      --arg blue_image "$blue_image" --arg green_image "$green_image" --arg worker_image "$worker_image" \
      --arg source_commit "$commit" --arg source_tree "$tree" --arg migrations_hash "$migrations" \
      --arg postgres_id "$postgres_id" --arg redis_id "$redis_id" --arg caddy_id "$caddy_id" \
      '{schema_version:1, active_slot:$active_slot, active_upstream:$active_upstream,
        blue_image:$blue_image, green_image:$green_image, worker_image:$worker_image,
        source_commit:$source_commit, source_tree:$source_tree, migrations_hash:$migrations,
        postgres_id:$postgres_id, redis_id:$redis_id, caddy_id:$caddy_id}'
    return
  fi
  run_post_stop_operation '
    temporary=$1
    release_state=$2
    shift 2
    jq -n "$@" >"$temporary" && chmod 0600 "$temporary" && mv "$temporary" "$release_state"
  ' "$temporary" "$release_state" \
    --arg active_slot "$active_slot" --arg active_upstream "$active_upstream" \
    --arg blue_image "$blue_image" --arg green_image "$green_image" --arg worker_image "$worker_image" \
    --arg source_commit "$commit" --arg source_tree "$tree" --arg migrations_hash "$migrations" \
    --arg postgres_id "$postgres_id" --arg redis_id "$redis_id" --arg caddy_id "$caddy_id" \
    --arg blue_image_id "$blue_image_id" --arg green_image_id "$green_image_id" --arg worker_image_id "$worker_image_id" \
    '{schema_version:2, active_slot:$active_slot, active_upstream:$active_upstream,
      blue_image:$blue_image, green_image:$green_image, worker_image:$worker_image,
      blue_image_id:$blue_image_id, green_image_id:$green_image_id, worker_image_id:$worker_image_id,
      source_commit:$source_commit, source_tree:$source_tree, migrations_hash:$migrations_hash,
      postgres_id:$postgres_id, redis_id:$redis_id, caddy_id:$caddy_id}'
}

write_partial() {
  local phase=$1 temporary
  [[ -n "$partial_path" ]] || return 0
  temporary="$partial_path.tmp"
  run_post_stop_operation '
    temporary=$1
    partial_path=$2
    shift 2
    jq -n "$@" >"$temporary" && chmod 0600 "$temporary" && mv "$temporary" "$partial_path"
  ' "$temporary" "$partial_path" \
    --arg attempt_id "$attempt_id" --arg mode "$mode" --argjson started_epoch "$started_epoch" \
    --arg phase "$phase" --argjson cutover_attempted "$cutover_attempted" \
    --argjson cutover_applied "$cutover_applied" \
    --argjson worker_updated "$worker_update_started" \
    --arg previous_slot "$previous_slot" --arg previous_upstream "$previous_upstream" \
    --arg previous_blue_image "$rollback_blue_image" --arg previous_green_image "$rollback_green_image" \
    --arg previous_worker_image "$previous_worker_image" \
    --arg previous_blue_image_id "$rollback_blue_image_id" --arg previous_green_image_id "$rollback_green_image_id" \
    --arg previous_worker_image_id "$previous_worker_image_id" \
    --arg previous_source_commit "$rollback_source_commit" --arg previous_source_tree "$rollback_source_tree" \
    --arg previous_migrations_hash "$rollback_migrations_hash" \
    --arg previous_postgres_id "$rollback_postgres_id" --arg previous_redis_id "$rollback_redis_id" \
    --arg previous_caddy_id "$rollback_caddy_id" \
    --arg candidate_slot "$candidate_slot" --arg candidate_upstream "$candidate_upstream" \
    --arg candidate_image "$requested_image" --arg candidate_image_id "$candidate_image_id" \
    '{schema_version:1, attempt_id:$attempt_id, mode:$mode, started_epoch:$started_epoch,
      phase:$phase, cutover_attempted:$cutover_attempted, cutover_applied:$cutover_applied,
      worker_updated:$worker_updated,
      previous:{active_slot:$previous_slot, active_upstream:$previous_upstream,
        blue_image:$previous_blue_image, green_image:$previous_green_image, worker_image:$previous_worker_image,
        blue_image_id:$previous_blue_image_id, green_image_id:$previous_green_image_id, worker_image_id:$previous_worker_image_id,
        source_commit:$previous_source_commit, source_tree:$previous_source_tree,
        migrations_hash:$previous_migrations_hash, postgres_id:$previous_postgres_id,
        redis_id:$previous_redis_id, caddy_id:$previous_caddy_id},
      candidate:{slot:$candidate_slot, upstream:$candidate_upstream, image:$candidate_image, image_id:$candidate_image_id}}'
}

gate() {
  local reason_code=$1 reason=$2 seconds=${3:-300}
  jq -n --arg reason_code "$reason_code" --arg reason "$reason" --argjson seconds "$seconds" '
    {schema_version:1, downtime_required:true, reason_code:$reason_code, reason:$reason,
      estimated_unavailable_seconds:$seconds,
      rollback:["keep current active slot", "do not start candidate", "prepare an authorized maintenance release"]}'
  cleanup_lock
  exit 2
}

validate_upstream() {
  case "$1" in
    sub2api-blue:8080|sub2api-green:8080) return 0 ;;
    *) return 1 ;;
  esac
}

managed_env_value() {
  local key=$1 count value
  count=$(awk -v key="$key" '
    $0 ~ "^[[:space:]]*" key "[[:space:]]*=" { count++ }
    END { print count + 0 }
  ' "$release_env")
  [[ "$count" == 1 ]] || fail "RELEASE_ENV must contain exactly one $key assignment"
  value=$(awk -v key="$key" '
    $0 ~ "^[[:space:]]*" key "[[:space:]]*=" {
      value=$0
      sub("^[[:space:]]*" key "[[:space:]]*=[[:space:]]*", "", value)
      sub(/[[:space:]]+$/, "", value)
      print value
    }
  ' "$release_env")
  printf '%s\n' "$value"
}

resolve_container_id() {
  local service=$1
  run_post_stop_operation '
    service=$1
    shift
    "$@" ps -q "$service" | awk '\''NF { id=$0; count++ } END { if (count != 1) exit 1; print id }'\''
  ' "$service" "${compose_current[@]}"
}

resolve_image_id() {
  local image=$1 image_id
  image_id=$(run_post_stop_operation '
    docker image inspect --format "{{.Id}}" "$1" 2>/dev/null | tr -d "[:space:]"
  ' "$image") || return 1
  [[ "$image_id" =~ ^sha256:[a-f0-9]{64}$ ]] || return 1
  printf '%s\n' "$image_id"
}

wait_for_worker_healthy() {
  local timeout=${WORKER_HEALTH_TIMEOUT_SECONDS:-90} poll=${WORKER_HEALTH_POLL_SECONDS:-1}
  local deadline now remaining attempts=0 max_attempts worker_status
  [[ "$timeout" =~ ^[1-9][0-9]*$ && "$poll" =~ ^[1-9][0-9]*$ ]] || return 1
  deadline=$(( $(date -u +%s) + timeout ))
  max_attempts=$((timeout / poll + 1))
  while true; do
    worker_status=$(run_post_stop_command docker inspect "$(resolve_container_id sub2api-worker)" --format '{{.State.Health.Status}}') || return 1
    [[ "$worker_status" == healthy ]] && return 0
    attempts=$((attempts + 1))
    [[ "$attempts" -lt "$max_attempts" ]] || return 1
    now=$(date -u +%s)
    [[ "$now" =~ ^[0-9]+$ && "$now" -lt "$deadline" ]] || return 1
    remaining=$((deadline - now))
    if [[ "$poll" -lt "$remaining" ]]; then run_post_stop_command sleep "$poll"; else run_post_stop_command sleep "$remaining"; fi
  done
}

wait_for_detector_healthy() {
  local timeout=${DETECTOR_HEALTH_TIMEOUT_SECONDS:-90} poll=${DETECTOR_HEALTH_POLL_SECONDS:-1}
  local deadline now remaining attempts=0 max_attempts status id
  [[ "$timeout" =~ ^[1-9][0-9]*$ && "$poll" =~ ^[1-9][0-9]*$ ]] || return 1
  deadline=$(( $(date -u +%s) + timeout ))
  max_attempts=$((timeout / poll + 1))
  while true; do
    id=$(resolve_container_id model-detector) || return 1
    status=$(run_post_stop_command docker inspect "$id" --format '{{.State.Health.Status}}') || return 1
    [[ "$status" == healthy ]] && return 0
    [[ "$status" != unhealthy ]] || return 1
    attempts=$((attempts + 1))
    [[ "$attempts" -lt "$max_attempts" ]] || return 1
    now=$(date -u +%s)
    [[ "$now" -lt "$deadline" ]] || return 1
    remaining=$((deadline - now))
    if [[ "$poll" -lt "$remaining" ]]; then run_post_stop_command sleep "$poll"; else run_post_stop_command sleep "$remaining"; fi
  done
}

wait_for_api_healthy() {
  local service=$1 timeout=${API_HEALTH_TIMEOUT_SECONDS:-90} poll=${API_HEALTH_POLL_SECONDS:-1}
  local deadline now remaining attempts=0 max_attempts api_status api_id
  [[ "$timeout" =~ ^[1-9][0-9]*$ && "$poll" =~ ^[1-9][0-9]*$ ]] || return 1
  deadline=$(( $(date -u +%s) + timeout ))
  max_attempts=$((timeout / poll + 1))
  while true; do
    api_id=$(resolve_container_id "$service") || return 1
    api_status=$(run_post_stop_command docker inspect "$api_id" --format '{{.State.Health.Status}}') || return 1
    [[ "$api_status" == healthy ]] && return 0
    attempts=$((attempts + 1))
    [[ "$attempts" -lt "$max_attempts" ]] || return 1
    now=$(date -u +%s)
    [[ "$now" =~ ^[0-9]+$ && "$now" -lt "$deadline" ]] || return 1
    remaining=$((deadline - now))
    if [[ "$poll" -lt "$remaining" ]]; then run_post_stop_command sleep "$poll"; else run_post_stop_command sleep "$remaining"; fi
  done
}

wait_for_candidate_healthy() {
	local service=$1 timeout=${CANDIDATE_HEALTH_TIMEOUT_SECONDS:-90} poll=${CANDIDATE_HEALTH_POLL_SECONDS:-1}
	local deadline now remaining attempts=0 max_attempts candidate_status candidate_id
	[[ "$timeout" =~ ^[1-9][0-9]*$ && "$poll" =~ ^[1-9][0-9]*$ ]] || return 1
	deadline=$(( $(date -u +%s) + timeout ))
	max_attempts=$((timeout / poll + 1))
	while true; do
		check_deadline
		candidate_id=$(resolve_container_id "$service") || return 1
    candidate_status=$(run_post_stop_command docker inspect "$candidate_id" --format '{{.State.Health.Status}}') || return 1
		[[ "$candidate_status" == healthy ]] && return 0
		[[ "$candidate_status" != unhealthy ]] || return 1
		attempts=$((attempts + 1))
		[[ "$attempts" -lt "$max_attempts" ]] || return 1
		now=$(date -u +%s)
		[[ "$now" =~ ^[0-9]+$ && "$now" -lt "$deadline" ]] || return 1
		remaining=$((deadline - now))
    if [[ "$poll" -lt "$remaining" ]]; then run_post_stop_command sleep "$poll"; else run_post_stop_command sleep "$remaining"; fi
	done
}

container_role() {
	local container_id=$1
	run_post_stop_operation '
		docker inspect "$1" --format "{{range .Config.Env}}{{println .}}{{end}}" | awk -F= '\''
			$1 == "SERVER_PROCESS_ROLE" { value=$2; count++ }
			END { if (count != 1) exit 1; print value }
		'\''
	' "$container_id"
}

live_caddy_upstream() {
  local jq_filter
  jq_filter='
		[.. | objects | .dial? // empty |
		 select(. == "sub2api-blue:8080" or . == "sub2api-green:8080")] |
		unique | if length == 1 then .[0] else error("active upstream is not unique") end
	'
  run_post_stop_operation '
    jq_filter=$1
    shift
    "$@" exec -T caddy wget -qO- http://127.0.0.1:2019/config/ | jq -er "$jq_filter"
  ' "$jq_filter" "${compose_current[@]}"
}

write_acceptance_headers() {
	[[ -n "$admin_header" && -n "$gateway_header" ]] && return 0
	admin_header="$record_root/.$attempt_id.admin.header"
	gateway_header="$record_root/.$attempt_id.gateway.header"
	run_post_stop_operation '
		printf "X-API-Key: %s\n" "$(tr -d "\r\n" <"$1")" >"$2" &&
		printf "Authorization: Bearer %s\n" "$(tr -d "\r\n" <"$3")" >"$4" &&
		chmod 0600 "$2" "$4"
	' "$admin_key_file" "$admin_header" "$gateway_key_file" "$gateway_header"
}

public_acceptance() {
	write_acceptance_headers || return 1
  run_post_stop_operation '
    curl -fsS --connect-timeout 5 --max-time 15 "$1/health" | jq -e '\''.status == "ok"'\'' >/dev/null
  ' "$base_url" || return 1
  run_post_stop_operation '
    curl -fsS --connect-timeout 5 --max-time 15 -H "@$2" "$1/api/v1/admin/system/version" |
      jq -e '\''(.data // .).version | type == "string" and length > 0'\'' >/dev/null
  ' "$base_url" "$admin_header" || return 1
  run_post_stop_operation '
    curl -fsS --connect-timeout 5 --max-time 15 -H "@$2" "$1/v1/models" |
      jq -e '\''.data | type == "array"'\'' >/dev/null
  ' "$base_url" "$gateway_header" || return 1
}

worker_logs_are_acceptable() {
  # Compose prefixes each line with the container name; avoid treating an
  # unrelated "Request failed" message as a worker startup failure.
  run_post_stop_operation '
    worker_logs=$("$@" logs --no-color --tail 200 sub2api-worker) || exit $?
    if printf "%s\n" "$worker_logs" | grep -Eiq '\''(^|[[:space:]])(panic:|fatal:|migration[^[:space:]]*[[:space:]]+failed|worker[[:space:]]+(startup|process|runtime)[^[:space:]]*[[:space:]]+failed)'\''; then
      exit 1
    fi
  ' "${compose_current[@]}"
}

restore_previous() {
  local rollback_ok=true current_blue current_green previous_previous
  rollback_in_progress=true
  if [[ "$cutover_attempted" == true ]]; then
    if validate_upstream "$previous_upstream"; then
      run_caddy_config_command "$previous_upstream" validate >/dev/null 2>&1 || rollback_ok=false
      run_caddy_config_command "$previous_upstream" reload >/dev/null 2>&1 || rollback_ok=false
    else
      rollback_ok=false
    fi
  fi
  if [[ "$rollback_ok" == true && ( "$maintenance_stopped" == true || "$worker_update_started" == true ) ]]; then
    rollback_env="$record_root/.$attempt_id.rollback.env"
    write_release_env_values_to "$rollback_env" "$rollback_blue_image" "$rollback_green_image" "$previous_worker_image" \
      "$previous_upstream" "$previous_slot" "$candidate_slot" || rollback_ok=false
    if [[ "$rollback_ok" == true ]]; then
      compose_rollback=(docker compose --project-name "$compose_project" --project-directory "$deploy_root"
        --env-file "$secret_env" --env-file "$rollback_env" -f "$base_compose")
      if [[ "$maintenance_stopped" == true ]]; then
        run_post_stop_command "${compose_rollback[@]}" up --no-deps -d "${compose_pull_args[@]+${compose_pull_args[@]}}" "sub2api-$previous_slot" >/dev/null 2>&1 || rollback_ok=false
      fi
      run_post_stop_command "${compose_rollback[@]}" up --no-deps -d "${compose_pull_args[@]+${compose_pull_args[@]}}" --force-recreate sub2api-worker >/dev/null 2>&1 || rollback_ok=false
    fi
  fi
	if [[ "$rollback_ok" == true ]]; then
		[[ "$rollback_ok" == false ]] || wait_for_api_healthy "sub2api-$previous_slot" || rollback_ok=false
		[[ "$rollback_ok" == false ]] || wait_for_worker_healthy || rollback_ok=false
		[[ "$rollback_ok" == false ]] || worker_logs_are_acceptable || rollback_ok=false
		[[ "$(live_caddy_upstream)" == "$previous_upstream" ]] || rollback_ok=false
		[[ "$rollback_ok" == false ]] || public_acceptance || rollback_ok=false
		if [[ "$rollback_ok" == true ]]; then
			rollback_active_id=$(resolve_container_id "sub2api-$previous_slot") || rollback_ok=false
			rollback_worker_id=$(resolve_container_id sub2api-worker) || rollback_ok=false
			[[ "$rollback_ok" == false || "$(run_post_stop_command docker inspect "$rollback_active_id" --format '{{.Image}}')" == "$rollback_active_image_id" ]] || rollback_ok=false
			[[ "$rollback_ok" == false || "$(run_post_stop_command docker inspect "$rollback_worker_id" --format '{{.Image}}')" == "$previous_worker_image_id" ]] || rollback_ok=false
		fi
	fi
	if [[ "$rollback_ok" == true ]]; then
		[[ "$(resolve_container_id postgres)" == "$rollback_postgres_id" ]] || rollback_ok=false
		[[ "$rollback_ok" == false || "$(resolve_container_id redis)" == "$rollback_redis_id" ]] || rollback_ok=false
		[[ "$rollback_ok" == false || "$(resolve_container_id caddy)" == "$rollback_caddy_id" ]] || rollback_ok=false
	fi
  if [[ "$rollback_ok" == true && ( "$persistence_started" == true || "$state_persisted" == true || "$worker_update_started" == true ) ]]; then
    current_blue=$rollback_blue_image
    current_green=$rollback_green_image
    previous_previous=$candidate_slot
    write_release_env_values "$current_blue" "$current_green" "$previous_worker_image" \
      "$previous_upstream" "$previous_slot" "$previous_previous" || rollback_ok=false
      write_state_values "$previous_slot" "$previous_upstream" "$current_blue" "$current_green" \
        "$previous_worker_image" "$rollback_source_commit" "$rollback_source_tree" "$rollback_migrations_hash" \
      "$rollback_postgres_id" "$rollback_redis_id" "$rollback_caddy_id" \
      "$rollback_blue_image_id" "$rollback_green_image_id" "$previous_worker_image_id" || rollback_ok=false
  fi
  [[ "$rollback_ok" == true ]] || return 1
  rollback_completed=true
  return 0
}

on_exit() {
  local status=$?
  trap - EXIT HUP INT TERM
  set +e
  run_post_stop_operation '
    for path in "$@"; do
      [[ -z "$path" ]] || rm -f -- "$path" || exit $?
    done
  ' "$candidate_env" "$rollback_env" "$admin_header" "$gateway_header" || true
  if [[ "$status" -ne 0 ]]; then
    restore_detector_topology || true
  fi
  if [[ "$status" -ne 0 && "$record_finalized" == false && -n "$partial_path" && -e "$partial_path" ]]; then
    if [[ "$maintenance_stopped" == true && -n "${maintenance_deadline_epoch:-}" ]]; then
      now=$(date -u +%s)
      rollback_remaining=$((maintenance_deadline_epoch - now))
      if [[ "${maintenance_started_millis:-}" =~ ^[0-9]+$ && "${maintenance_window_seconds:-}" =~ ^[1-9][0-9]*$ ]]; then
        elapsed_millis=$(( $(monotonic_millis) - maintenance_started_millis ))
        monotonic_rollback_remaining=$((maintenance_window_seconds - elapsed_millis / 1000))
        (( monotonic_rollback_remaining < rollback_remaining )) && rollback_remaining=$monotonic_rollback_remaining
      fi
      (( rollback_remaining > 0 )) && arm_deadline_watchdog "$rollback_remaining"
    fi
    if restore_previous; then
      finalization_in_progress=true
      write_final_record failed rolled_back "$failure_reason" || true
    else
      finalization_in_progress=true
      write_final_record failed rollback_failed "$failure_reason" || true
    fi
    if [[ "$rollback_completed" == true && "$record_finalized" == true ]]; then
      run_post_stop_command rm -f -- "$partial_path" || true
    fi
  fi
  if [[ "$status" -eq 0 && "$record_finalized" == true ]]; then
    rm -f -- "$detector_compose_backup" "$detector_secret_backup"
  elif [[ "$status" -ne 0 && "$record_finalized" == true && "$rollback_completed" == true ]]; then
    rm -f -- "$detector_compose_backup" "$detector_secret_backup"
  fi
  cleanup_lock
  exit "$status"
}
trap on_exit EXIT
trap 'failure_reason=interrupted; exit 130' HUP INT TERM

partial_record_is_valid() {
  local existing=$1
  [[ -f "$existing" && ! -L "$existing" && "$(mode_of "$existing")" == 600 ]] || return 1
  jq -e --arg mode "$mode" '
    def release_tag_matches_image_id($image; $image_id):
      if ($image | test(":release-[a-f0-9]{40}-[a-f0-9]{64}$")) then
        ($image | capture(":release-[a-f0-9]{40}-(?<suffix>[a-f0-9]{64})$").suffix) ==
          ($image_id | sub("^sha256:"; ""))
      else true end;
    type == "object" and
    (keys | sort) == ["attempt_id","candidate","cutover_applied","cutover_attempted","mode","phase","previous","schema_version","started_epoch","worker_updated"] and
    .schema_version == 1 and (.attempt_id | type == "string" and length > 0) and
    .mode == $mode and
    (.started_epoch | type == "number" and floor == .) and
    (.phase | type == "string" and length > 0) and
    (.cutover_attempted | type == "boolean") and (.cutover_applied | type == "boolean") and
    (.worker_updated | type == "boolean") and
    (.previous | type == "object" and
      ((keys | sort) == ["active_slot","active_upstream","blue_image","caddy_id","green_image","migrations_hash","postgres_id","redis_id","source_commit","source_tree","worker_image"] or
       ((keys | sort) == ["active_slot","active_upstream","blue_image","blue_image_id","caddy_id","green_image","green_image_id","migrations_hash","postgres_id","redis_id","source_commit","source_tree","worker_image","worker_image_id"] and
        ([.blue_image_id,.green_image_id,.worker_image_id] | all(type == "string" and test("^sha256:[a-f0-9]{64}$"))) and
        release_tag_matches_image_id(.blue_image; .blue_image_id) and
        release_tag_matches_image_id(.green_image; .green_image_id) and
        release_tag_matches_image_id(.worker_image; .worker_image_id))) and
      (.active_slot == "blue" or .active_slot == "green") and
      ((.active_slot == "blue" and .active_upstream == "sub2api-blue:8080") or
       (.active_slot == "green" and .active_upstream == "sub2api-green:8080")) and
      ([.blue_image,.green_image,.worker_image] | all(type == "string" and test("^[^[:space:]@]+(@sha256:[a-f0-9]{64}|:release-[a-f0-9]{40}(-[a-f0-9]{64})?)$"))) and
      (.source_commit | type == "string" and test("^[a-f0-9]{40}$")) and
      (.source_tree | type == "string" and test("^[a-f0-9]{40}$")) and
      (.migrations_hash | type == "string" and test("^[a-f0-9]{64}$")) and
      ([.postgres_id,.redis_id,.caddy_id] | all(type == "string" and length > 0))) and
    (.candidate | type == "object" and ((keys | sort) == ["image","slot","upstream"] or (keys | sort) == ["image","image_id","slot","upstream"]) and
      (.slot == "blue" or .slot == "green") and
      ((.slot == "blue" and .upstream == "sub2api-blue:8080") or
       (.slot == "green" and .upstream == "sub2api-green:8080")) and
      (.image | type == "string" and test("^[^[:space:]@]+(@sha256:[a-f0-9]{64}|:release-[a-f0-9]{40}(-[a-f0-9]{64})?)$") ) and
      ((.image_id // "") | type == "string" and (length == 0 or test("^sha256:[a-f0-9]{64}$"))) and
      (if has("image_id") then release_tag_matches_image_id(.image; .image_id) else true end)) and
    .previous.active_slot != .candidate.slot
  ' "$existing" >/dev/null 2>&1
}

partial_rollback_image_ids_match_local_images() {
  local existing=$1 previous_slot image expected actual
  previous_slot=$(jq -r '.previous.active_slot // empty' "$existing" 2>/dev/null) || return 1
  case "$previous_slot" in
    blue)
      image=$(jq -r '.previous.blue_image' "$existing" 2>/dev/null) || return 1
      expected=$(jq -r '.previous.blue_image_id // empty' "$existing" 2>/dev/null) || return 1
      ;;
    green)
      image=$(jq -r '.previous.green_image' "$existing" 2>/dev/null) || return 1
      expected=$(jq -r '.previous.green_image_id // empty' "$existing" 2>/dev/null) || return 1
      ;;
    *) return 1 ;;
  esac
  [[ -z "$expected" ]] || { actual=$(resolve_image_id "$image") && [[ "$actual" == "$expected" ]]; } || return 1
  image=$(jq -r '.previous.worker_image' "$existing" 2>/dev/null) || return 1
  expected=$(jq -r '.previous.worker_image_id // empty' "$existing" 2>/dev/null) || return 1
  [[ -z "$expected" ]] || { actual=$(resolve_image_id "$image") && [[ "$actual" == "$expected" ]]; } || return 1
}

recover_partial() {
  local existing=$1 now age recovery_cutover_attempted recovery_cutover recovery_worker
  partial_record_is_valid "$existing" || fail 'stale or invalid partial release record is present'
  partial_rollback_image_ids_match_local_images "$existing" \
    || fail 'partial release image ID does not match its local image reference'
  now=$(date -u +%s)
  age=$((now - $(jq -r '.started_epoch' "$existing")))
  [[ "$age" -ge 0 && "$age" -le 1800 ]] || fail 'stale partial release record is present'
  previous_slot=$(jq -r '.previous.active_slot' "$existing")
  previous_upstream=$(jq -r '.previous.active_upstream' "$existing")
  previous_worker_image=$(jq -r '.previous.worker_image' "$existing")
  previous_worker_image_id=$(jq -r '.previous.worker_image_id // empty' "$existing")
  rollback_blue_image=$(jq -r '.previous.blue_image' "$existing")
  rollback_green_image=$(jq -r '.previous.green_image' "$existing")
  rollback_blue_image_id=$(jq -r '.previous.blue_image_id // empty' "$existing")
  rollback_green_image_id=$(jq -r '.previous.green_image_id // empty' "$existing")
  if [[ "$previous_slot" == blue && -z "$rollback_blue_image_id" ]]; then
    rollback_blue_image_id=$(resolve_image_id "$rollback_blue_image") \
      || fail 'partial record previous active image ID could not be established'
  fi
  if [[ "$previous_slot" == green && -z "$rollback_green_image_id" ]]; then
    rollback_green_image_id=$(resolve_image_id "$rollback_green_image") \
      || fail 'partial record previous active image ID could not be established'
  fi
  if [[ -z "$previous_worker_image_id" ]]; then
    previous_worker_image_id=$(resolve_image_id "$previous_worker_image") \
      || fail 'partial record previous worker image ID could not be established'
  fi
  if [[ "$rollback_blue_image" == "$rollback_green_image" ]]; then
    [[ -n "$rollback_blue_image_id" ]] || rollback_blue_image_id=$rollback_green_image_id
    [[ -n "$rollback_green_image_id" ]] || rollback_green_image_id=$rollback_blue_image_id
  fi
  rollback_active_image_id=$rollback_blue_image_id
  [[ "$previous_slot" == green ]] && rollback_active_image_id=$rollback_green_image_id
  rollback_source_commit=$(jq -r '.previous.source_commit' "$existing")
  rollback_source_tree=$(jq -r '.previous.source_tree' "$existing")
  rollback_migrations_hash=$(jq -r '.previous.migrations_hash' "$existing")
  rollback_postgres_id=$(jq -r '.previous.postgres_id' "$existing")
  rollback_redis_id=$(jq -r '.previous.redis_id' "$existing")
  rollback_caddy_id=$(jq -r '.previous.caddy_id' "$existing")
  candidate_slot=$(jq -r '.candidate.slot' "$existing")
  candidate_upstream=$(jq -r '.candidate.upstream' "$existing")
  recovery_cutover_attempted=$(jq -r '.cutover_attempted' "$existing")
  recovery_cutover=$(jq -r '.cutover_applied' "$existing")
  recovery_worker=$(jq -r '.worker_updated' "$existing")
  validate_upstream "$previous_upstream" || fail 'partial record previous upstream is invalid'
  [[ "$previous_worker_image" =~ ^[^[:space:]@]+(@sha256:[a-f0-9]{64}|:release-[a-f0-9]{40}(-[a-f0-9]{64})?)$ ]] || fail 'partial record previous worker image is invalid'
  cutover_attempted=$recovery_cutover_attempted
  cutover_applied=$recovery_cutover
  state_persisted=$recovery_cutover
  persistence_started=$recovery_cutover
  worker_update_started=$recovery_worker
  failure_reason=interrupted_release_recovered
  partial_path=$existing
  if restore_previous; then
    write_final_record failed rolled_back "$failure_reason"
    rm -f -- "$existing"
  else
    write_final_record failed rollback_failed "$failure_reason"
  fi
  cleanup_lock
  exit 1
}

partial_is_committed_success() {
  local existing=$1 partial_attempt partial_slot partial_upstream partial_image success_record state_image
  partial_record_is_valid "$existing" || return 1
  partial_attempt=$(jq -r '.attempt_id // empty' "$existing" 2>/dev/null) || return 1
  partial_slot=$(jq -r '.candidate.slot // empty' "$existing" 2>/dev/null) || return 1
  partial_upstream=$(jq -r '.candidate.upstream // empty' "$existing" 2>/dev/null) || return 1
  partial_image=$(jq -r '.candidate.image // empty' "$existing" 2>/dev/null) || return 1
  [[ "$partial_attempt" =~ ^[A-Za-z0-9._-]+$ ]] || return 1
  case "$partial_slot:$partial_upstream" in
    blue:sub2api-blue:8080) state_image=$state_blue_image ;;
    green:sub2api-green:8080) state_image=$state_green_image ;;
    *) return 1 ;;
  esac
  [[ "$state_active_slot" == "$partial_slot" && "$state_active_upstream" == "$partial_upstream" && "$state_image" == "$partial_image" ]] \
    || return 1
  success_record="$record_root/$partial_attempt.json"
  [[ -f "$success_record" && ! -L "$success_record" && "$(mode_of "$success_record")" == 600 ]] || return 1
  jq -e --arg attempt_id "$partial_attempt" --arg mode "$mode" --arg image "$partial_image" '
    type == "object" and .schema_version == 1 and .attempt_id == $attempt_id and .mode == $mode and
    .result == "succeeded" and .state == "promoted" and (.requested | type == "object") and .requested.image == $image
  ' "$success_record" >/dev/null 2>&1
}

existing_partials=$(find "$record_root" -maxdepth 1 -type f -name '*.partial' -print)
partial_count=$(printf '%s\n' "$existing_partials" | awk 'NF { count++ } END { print count + 0 }')
[[ "$partial_count" -le 1 ]] || fail 'multiple partial release records are present'

if [[ ! -e "$release_state" ]]; then
  gate legacy_topology_bootstrap 'steady-state blue-green release state is absent; bootstrap requires maintenance authorization' 600
fi
[[ -f "$release_state" && ! -L "$release_state" ]] || fail 'RELEASE_STATE must be a regular non-symlink file'
[[ "$(mode_of "$release_state")" == 600 ]] || fail 'RELEASE_STATE mode must be 0600'

duplicate_state_keys=$(jq -r --stream 'select(length == 2 and (.[0] | length) == 1) | .[0][0]' "$release_state" 2>/dev/null | sort | uniq -d)
[[ -z "$duplicate_state_keys" ]] || fail 'RELEASE_STATE contains duplicate top-level keys'
jq -e '
  def release_tag_matches_image_id($image; $image_id):
    if ($image | test(":release-[a-f0-9]{40}-[a-f0-9]{64}$")) then
      ($image | capture(":release-[a-f0-9]{40}-(?<suffix>[a-f0-9]{64})$").suffix) ==
        ($image_id | sub("^sha256:"; ""))
    else true end;
  type == "object" and
  ((.schema_version == 1 and
    (keys | sort) == ["active_slot","active_upstream","blue_image","caddy_id","green_image","migrations_hash","postgres_id","redis_id","schema_version","source_commit","source_tree","worker_image"]) or
   (.schema_version == 2 and
    (keys | sort) == ["active_slot","active_upstream","blue_image","blue_image_id","caddy_id","green_image","green_image_id","migrations_hash","postgres_id","redis_id","schema_version","source_commit","source_tree","worker_image","worker_image_id"] and
    ([.blue_image_id,.green_image_id,.worker_image_id] | all(type == "string" and test("^sha256:[a-f0-9]{64}$"))) and
    release_tag_matches_image_id(.blue_image; .blue_image_id) and
    release_tag_matches_image_id(.green_image; .green_image_id) and
    release_tag_matches_image_id(.worker_image; .worker_image_id))) and
  (.active_slot == "blue" or .active_slot == "green") and
  (.active_upstream == "sub2api-blue:8080" or .active_upstream == "sub2api-green:8080") and
  ([.blue_image,.green_image,.worker_image] | all(type == "string" and test("^[^[:space:]@]+(@sha256:[a-f0-9]{64}|:release-[a-f0-9]{40}(-[a-f0-9]{64})?)$"))) and
  (.source_commit | type == "string" and test("^[a-f0-9]{40}$")) and
  (.source_tree | type == "string" and test("^[a-f0-9]{40}$")) and
  (.migrations_hash | type == "string" and test("^[a-f0-9]{64}$")) and
  ([.postgres_id,.redis_id,.caddy_id] | all(type == "string" and length > 0))
' "$release_state" >/dev/null 2>&1 || fail 'RELEASE_STATE schema is invalid'

state_active_slot=$(jq -r '.active_slot' "$release_state")
state_active_upstream=$(jq -r '.active_upstream' "$release_state")
state_blue_image=$(jq -r '.blue_image' "$release_state")
state_green_image=$(jq -r '.green_image' "$release_state")
state_worker_image=$(jq -r '.worker_image' "$release_state")
state_blue_image_id=$(jq -r '.blue_image_id // empty' "$release_state")
state_green_image_id=$(jq -r '.green_image_id // empty' "$release_state")
state_worker_image_id=$(jq -r '.worker_image_id // empty' "$release_state")
state_source_commit=$(jq -r '.source_commit' "$release_state")
state_source_tree=$(jq -r '.source_tree' "$release_state")
state_migrations_hash=$(jq -r '.migrations_hash' "$release_state")
state_postgres_id=$(jq -r '.postgres_id' "$release_state")
state_redis_id=$(jq -r '.redis_id' "$release_state")
state_caddy_id=$(jq -r '.caddy_id' "$release_state")

if [[ "$partial_count" == 1 ]]; then
  if partial_is_committed_success "$existing_partials"; then
    rm -f -- "$existing_partials"
    partial_count=0
  else
    recover_partial "$existing_partials"
  fi
fi

state_blue_actual_image_id=$(resolve_image_id "$state_blue_image") \
  || fail 'release state blue image ID could not be resolved locally'
state_green_actual_image_id=$(resolve_image_id "$state_green_image") \
  || fail 'release state green image ID could not be resolved locally'
state_worker_actual_image_id=$(resolve_image_id "$state_worker_image") \
  || fail 'release state worker image ID could not be resolved locally'
[[ -z "$state_blue_image_id" || "$state_blue_image_id" == "$state_blue_actual_image_id" ]] \
  || fail 'release state blue image ID does not match its local image reference'
[[ -z "$state_green_image_id" || "$state_green_image_id" == "$state_green_actual_image_id" ]] \
  || fail 'release state green image ID does not match its local image reference'
[[ -z "$state_worker_image_id" || "$state_worker_image_id" == "$state_worker_actual_image_id" ]] \
  || fail 'release state worker image ID does not match its local image reference'
state_blue_image_id=$state_blue_actual_image_id
state_green_image_id=$state_green_actual_image_id
state_worker_image_id=$state_worker_actual_image_id

# Recovery above depends only on protected checkpoint/state data. Confirm the
# preloaded probe image only after that recovery has either completed or found
# no interrupted release, and before the first candidate network probe runs.
if [[ "$preloaded_image" == true ]]; then
  docker image inspect "$network_curl_image" >/dev/null 2>&1 \
    || fail 'preloaded network probe image is not present locally'
fi

# Recovery above depends only on protected checkpoint/state data. New image
# availability and provenance must never block repairing an interrupted release.
if [[ "$preloaded_image" == true ]]; then
  secure_directory "$release_staging_root" RELEASE_STAGING_ROOT
  secure_file "$preloaded_archive" PRELOADED_ARCHIVE
  [[ "$(sha256_file "$preloaded_archive" 2>/dev/null)" == "$preloaded_archive_sha256" ]] \
    || fail 'preloaded image archive checksum mismatch'
  docker load --input "$preloaded_archive" >/dev/null \
    || fail 'preloaded image load failed'
  loaded_image_id=$(docker image inspect --format '{{.Id}}' "$requested_image" 2>/dev/null | tr -d '[:space:]') \
    || fail 'preloaded image ID inspection failed'
  [[ "$loaded_image_id" == "$preloaded_image_id" ]] \
    || fail 'preloaded image ID mismatch after load'
fi
image_json=$(docker image inspect "$requested_image") || fail 'could not inspect requested image'
requested_image_id=$(jq -r '.[0].Id // empty' <<<"$image_json")
[[ "$requested_image_id" =~ ^sha256:[a-f0-9]{64}$ ]] || fail 'requested image ID is invalid'
if [[ "$preloaded_image" == true && "$requested_image_id" != "$preloaded_image_id" ]]; then
  fail 'requested image ID differs from preloaded image ID'
fi
candidate_image_id=$requested_image_id
if [[ "$preloaded_image" == true ]]; then
  jq -e \
    --arg image_id "$preloaded_image_id" --arg source_commit "$source_commit" \
    --arg source_tree "$source_tree" --arg tested_tree "$tested_tree" \
    --arg migrations_hash "$migrations_hash" '
    length == 1 and
    .[0].Id == $image_id and
    .[0].Config.Labels["com.xingqiao.sub2api.qualified"] == "true" and
    .[0].Config.Labels["com.xingqiao.sub2api.source.commit"] == $source_commit and
    .[0].Config.Labels["com.xingqiao.sub2api.source.tree"] == $source_tree and
    .[0].Config.Labels["com.xingqiao.sub2api.tested.tree"] == $tested_tree and
    .[0].Config.Labels["com.xingqiao.sub2api.migrations.sha256"] == $migrations_hash
  ' <<<"$image_json" >/dev/null || fail 'preloaded image labels do not match qualified source/test evidence'
else
  jq -e \
    --arg image "$requested_image" --arg source_commit "$source_commit" \
    --arg source_tree "$source_tree" --arg tested_tree "$tested_tree" \
    --arg migrations_hash "$migrations_hash" '
    length == 1 and
    (.[0].RepoDigests | type == "array" and index($image) != null) and
    .[0].Config.Labels["com.xingqiao.sub2api.qualified"] == "true" and
    .[0].Config.Labels["com.xingqiao.sub2api.source.commit"] == $source_commit and
    .[0].Config.Labels["com.xingqiao.sub2api.source.tree"] == $source_tree and
    .[0].Config.Labels["com.xingqiao.sub2api.tested.tree"] == $tested_tree and
    .[0].Config.Labels["com.xingqiao.sub2api.migrations.sha256"] == $migrations_hash
  ' <<<"$image_json" >/dev/null || fail 'requested image labels do not match qualified source/test evidence'
fi

case "$state_active_slot:$state_active_upstream" in
  blue:sub2api-blue:8080) candidate_slot=green; candidate_upstream=sub2api-green:8080 ;;
  green:sub2api-green:8080) candidate_slot=blue; candidate_upstream=sub2api-blue:8080 ;;
  *) gate invalid_active_slot_upstream 'active slot and Caddy upstream are not an allowlisted matching pair' 300 ;;
esac
previous_slot=$state_active_slot
previous_upstream=$state_active_upstream
previous_worker_image=$state_worker_image
previous_worker_image_id=$state_worker_image_id
rollback_blue_image=$state_blue_image
rollback_green_image=$state_green_image
rollback_blue_image_id=$state_blue_image_id
rollback_green_image_id=$state_green_image_id
rollback_active_image_id=$state_blue_image_id
[[ "$state_active_slot" == green ]] && rollback_active_image_id=$state_green_image_id
rollback_source_commit=$state_source_commit
rollback_source_tree=$state_source_tree
rollback_migrations_hash=$state_migrations_hash
rollback_postgres_id=$state_postgres_id
rollback_redis_id=$state_redis_id
rollback_caddy_id=$state_caddy_id
validate_upstream "$candidate_upstream" || gate invalid_candidate_upstream 'candidate Caddy upstream is not allowlisted' 300

live_upstream=$(live_caddy_upstream) || fail 'live Caddy upstream is not uniquely resolvable'
[[ "$live_upstream" == "$state_active_upstream" ]] || fail 'live Caddy upstream does not match release state'

[[ "$(managed_env_value SUB2API_BLUE_IMAGE)" == "$state_blue_image" ]] || fail 'RELEASE_ENV blue image does not match state'
[[ "$(managed_env_value SUB2API_GREEN_IMAGE)" == "$state_green_image" ]] || fail 'RELEASE_ENV green image does not match state'
[[ "$(managed_env_value SUB2API_WORKER_IMAGE)" == "$state_worker_image" ]] || fail 'RELEASE_ENV worker image does not match state'
[[ "$(managed_env_value SUB2API_ACTIVE_UPSTREAM)" == "$state_active_upstream" ]] || fail 'RELEASE_ENV active upstream does not match state'
[[ "$(managed_env_value SUB2API_ACTIVE_SLOT)" == "$state_active_slot" ]] || fail 'RELEASE_ENV active slot does not match state'
state_previous_slot=green
[[ "$state_active_slot" == green ]] && state_previous_slot=blue
[[ "$(managed_env_value SUB2API_PREVIOUS_SLOT)" == "$state_previous_slot" ]] || fail 'RELEASE_ENV previous slot does not match state'

if [[ "$migrations_hash" != "$state_migrations_hash" ]]; then
  if [[ "$maintenance_authorized" == true \
      && "$maintenance_from_hash" == "$state_migrations_hash" ]] \
      && approved_maintenance_transition "$state_migrations_hash" "$migrations_hash"; then
    maintenance_transition=true
  else
    gate migration_set_changed 'candidate migration set differs from the active release' 300
  fi
elif [[ "$maintenance_authorized" == true \
    && "$maintenance_from_hash" == "$state_migrations_hash" ]]; then
  # A previously promoted release may have had its shared Caddy container
  # recreated independently of the release-state checkpoint. An explicitly
  # authorized maintenance run must be able to refresh that identity while
  # keeping the migration set unchanged; the identity block below still
  # verifies PostgreSQL/Redis continuity and the expected Caddy image.
  maintenance_transition=true
fi

postgres_id=$(resolve_container_id postgres) || gate legacy_topology_bootstrap 'PostgreSQL container identity is not uniquely resolvable' 600
redis_id=$(resolve_container_id redis) || gate legacy_topology_bootstrap 'Redis container identity is not uniquely resolvable' 600
caddy_id=$(resolve_container_id caddy) || gate legacy_topology_bootstrap 'Caddy container identity is not uniquely resolvable' 600
if [[ "$postgres_id" != "$state_postgres_id" || "$redis_id" != "$state_redis_id" || "$caddy_id" != "$state_caddy_id" ]]; then
  if [[ "$maintenance_transition" == true && "$postgres_id" == "$state_postgres_id" && "$redis_id" == "$state_redis_id" ]]; then
    expected_caddy_image=$("${compose_current[@]}" config --format json | jq -er '.services.caddy.image') \
      || gate shared_container_identity_changed 'current Caddy image could not be proved during authorized maintenance' 600
    [[ "$(docker inspect "$caddy_id" --format '{{.Config.Image}}')" == "$expected_caddy_image" ]] \
      || gate shared_container_identity_changed 'Caddy identity changed to an unexpected image' 600
    maintenance_identity_refresh=true
  else
    gate shared_container_identity_changed 'PostgreSQL, Redis, or Caddy identity differs from the active release state' 600
  fi
fi

active_service="sub2api-$state_active_slot"
active_image=$state_blue_image
[[ "$state_active_slot" == green ]] && active_image=$state_green_image
active_container_id=$(resolve_container_id "$active_service") \
	|| gate invalid_runtime_cardinality 'active API container identity is not uniquely resolvable' 600
worker_container_id=$(resolve_container_id sub2api-worker) \
	|| gate invalid_runtime_cardinality 'worker container identity is not uniquely resolvable' 600
active_runtime_image_id=$(docker inspect "$active_container_id" --format '{{.Image}}') \
  || gate active_runtime_drift 'active API image ID could not be inspected' 600
worker_runtime_image_id=$(docker inspect "$worker_container_id" --format '{{.Image}}') \
  || gate active_runtime_drift 'worker image ID could not be inspected' 600
[[ "$active_runtime_image_id" =~ ^sha256:[a-f0-9]{64}$ && "$worker_runtime_image_id" =~ ^sha256:[a-f0-9]{64}$ ]] \
  || gate active_runtime_drift 'runtime image ID is invalid' 600
[[ "$(docker inspect "$active_container_id" --format '{{.Config.Image}}')" == "$active_image" ]] \
	|| gate active_runtime_drift 'active API image differs from release state' 600
[[ "$(docker inspect "$worker_container_id" --format '{{.Config.Image}}')" == "$state_worker_image" ]] \
	|| gate active_runtime_drift 'worker image differs from release state' 600
if [[ "$state_active_slot" == blue && -z "$state_blue_image_id" ]]; then state_blue_image_id=$active_runtime_image_id; fi
if [[ "$state_active_slot" == green && -z "$state_green_image_id" ]]; then state_green_image_id=$active_runtime_image_id; fi
if [[ -z "$state_worker_image_id" ]]; then state_worker_image_id=$worker_runtime_image_id; fi
if [[ -z "$state_blue_image_id" || -z "$state_green_image_id" ]]; then
  # The inactive legacy slot has no running container; resolve its immutable ID
  # from the image already referenced by the protected release state.
  legacy_slot_image=$state_blue_image
  [[ -n "$state_blue_image_id" ]] || state_blue_image_id=$(docker image inspect --format '{{.Id}}' "$legacy_slot_image" 2>/dev/null | tr -d '[:space:]')
  legacy_slot_image=$state_green_image
  [[ -n "$state_green_image_id" ]] || state_green_image_id=$(docker image inspect --format '{{.Id}}' "$legacy_slot_image" 2>/dev/null | tr -d '[:space:]')
fi
[[ "$state_blue_image_id" =~ ^sha256:[a-f0-9]{64}$ && "$state_green_image_id" =~ ^sha256:[a-f0-9]{64}$ && "$state_worker_image_id" =~ ^sha256:[a-f0-9]{64}$ ]] \
  || gate active_runtime_drift 'release state image IDs could not be established' 600
[[ "$state_active_slot" == blue && "$state_blue_image_id" == "$active_runtime_image_id" ||
   "$state_active_slot" == green && "$state_green_image_id" == "$active_runtime_image_id" ]] \
  || gate active_runtime_drift 'active API image ID differs from release state' 600
[[ "$state_worker_image_id" == "$worker_runtime_image_id" ]] \
  || gate active_runtime_drift 'worker image ID differs from release state' 600
previous_worker_image_id=$state_worker_image_id
rollback_blue_image_id=$state_blue_image_id
rollback_green_image_id=$state_green_image_id
rollback_active_image_id=$rollback_blue_image_id
[[ "$previous_slot" == green ]] && rollback_active_image_id=$rollback_green_image_id
[[ "$(container_role "$active_container_id")" == api ]] \
	|| gate invalid_runtime_role 'active API runtime role is not api' 600
[[ "$(container_role "$worker_container_id")" == worker ]] \
	|| gate invalid_runtime_role 'worker runtime role is not worker' 600
legacy_all_ids=$(docker ps -q --filter "label=com.docker.compose.project=$compose_project" \
	--filter label=com.docker.compose.service=sub2api) || fail 'legacy runtime lookup failed'
[[ -z "$legacy_all_ids" ]] || gate invalid_runtime_cardinality 'legacy all-role runtime is still present' 600

available_kb=$(df -Pk "$deploy_root" | awk 'NR == 2 { print $4 }')
[[ "$available_kb" =~ ^[0-9]+$ && "$available_kb" -ge "${MIN_FREE_KB:-2097152}" ]] \
  || gate insufficient_disk_headroom 'less than 2 GiB disk headroom is available for parallel release operation' 300

meminfo_file=${MEMINFO_FILE:-/proc/meminfo}
[[ -f "$meminfo_file" && -r "$meminfo_file" && ! -L "$meminfo_file" ]] || fail 'memory headroom source is invalid'
memory_kb=$(awk '$1 == "MemAvailable:" { print $2; exit }' "$meminfo_file")
[[ "$memory_kb" =~ ^[0-9]+$ && "$memory_kb" -ge "${MIN_AVAILABLE_MEMORY_KB:-1048576}" ]] \
  || gate insufficient_memory_headroom 'less than 1 GiB available memory remains for the parallel API slot' 300

db_headroom=$("${compose_current[@]}" exec -T postgres sh -c \
  'exec psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "select current_setting('"'"'max_connections'"'"')::int - count(*)::int from pg_stat_activity"') \
  || gate insufficient_db_connection_headroom 'database connection headroom could not be proved' 300
db_headroom=$(printf '%s' "$db_headroom" | tr -d '[:space:]')
[[ "$db_headroom" =~ ^[0-9]+$ && "$db_headroom" -ge "${MIN_DB_CONNECTION_HEADROOM:-10}" ]] \
  || gate insufficient_db_connection_headroom 'fewer than 10 PostgreSQL connections remain available' 300

configure_detector_topology
candidate_env="$record_root/.$attempt_id.candidate.env"
cp "$release_env" "$candidate_env"
chmod 0600 "$candidate_env"
candidate_blue=$state_blue_image
candidate_green=$state_green_image
candidate_worker=$state_worker_image
candidate_blue_image_id=$state_blue_image_id
candidate_green_image_id=$state_green_image_id
candidate_worker_image_id=$requested_image_id
if [[ "$candidate_slot" == blue ]]; then candidate_blue=$requested_image; else candidate_green=$requested_image; fi
if [[ "$candidate_slot" == blue ]]; then candidate_blue_image_id=$requested_image_id; else candidate_green_image_id=$requested_image_id; fi
if [[ "$maintenance_transition" == true ]]; then candidate_worker=$requested_image; candidate_worker_image_id=$requested_image_id; fi
awk \
  -v blue="$candidate_blue" -v green="$candidate_green" -v worker="$candidate_worker" '
  /^SUB2API_BLUE_IMAGE=/ { print "SUB2API_BLUE_IMAGE=" blue; next }
  /^SUB2API_GREEN_IMAGE=/ { print "SUB2API_GREEN_IMAGE=" green; next }
  /^SUB2API_WORKER_IMAGE=/ { print "SUB2API_WORKER_IMAGE=" worker; next }
  { print }
' "$candidate_env" >"$candidate_env.tmp"
chmod 0600 "$candidate_env.tmp"
mv "$candidate_env.tmp" "$candidate_env"
compose_candidate=(docker compose --project-name "$compose_project" --project-directory "$deploy_root"
  --env-file "$secret_env" --env-file "$candidate_env" -f "$base_compose")
candidate_config=$("${compose_candidate[@]}" config --format json) || gate invalid_candidate_topology 'candidate Compose topology could not be rendered' 600
active_image=$state_blue_image
[[ "$state_active_slot" == green ]] && active_image=$state_green_image
jq -e --arg service "sub2api-$candidate_slot" --arg active_service "sub2api-$state_active_slot" \
  --arg image "$requested_image" --arg active_image "$active_image" --arg worker_image "$candidate_worker" '
  .services[$service].image == $image and
  .services[$active_service].image == $active_image and
  .services["sub2api-worker"].image == $worker_image
' <<<"$candidate_config" >/dev/null 2>&1 || gate invalid_candidate_topology 'candidate Compose image selection is not exact' 600
jq -e --arg service "sub2api-$candidate_slot" --arg active_service "sub2api-$state_active_slot" '
  .services[$service].environment.SERVER_PROCESS_ROLE == "api" and
  .services[$active_service].environment.SERVER_PROCESS_ROLE == "api" and
  .services["sub2api-worker"].environment.SERVER_PROCESS_ROLE == "worker"
' <<<"$candidate_config" >/dev/null 2>&1 || gate candidate_role_not_api 'inactive candidate slot is not configured with SERVER_PROCESS_ROLE=api' 600
partial_path="$record_root/$attempt_id.partial"
write_partial preflight_complete

failure_reason=candidate_pull_failed
if [[ "$maintenance_transition" == true ]]; then
  maintenance_requested_window_seconds=${MAINTENANCE_UNAVAILABLE_SECONDS:-300}
  [[ "$maintenance_requested_window_seconds" =~ ^[1-9][0-9]*$ && "$maintenance_requested_window_seconds" -le 300 ]] \
    || fail 'MAINTENANCE_UNAVAILABLE_SECONDS must be an integer between 1 and 300'
  maintenance_started_epoch=$(date -u +%s) || fail 'maintenance deadline clock failed'
  maintenance_end_to_end_remaining=$((deadline_epoch - maintenance_started_epoch))
  maintenance_end_to_end_elapsed=$(( ($(monotonic_millis) - release_deadline_started_millis) / 1000 ))
  maintenance_end_to_end_monotonic_remaining=$((release_deadline_window_seconds - maintenance_end_to_end_elapsed))
  if (( maintenance_end_to_end_monotonic_remaining < maintenance_end_to_end_remaining )); then
    maintenance_end_to_end_remaining=$maintenance_end_to_end_monotonic_remaining
  fi
  maintenance_window_seconds=$maintenance_requested_window_seconds
  if (( maintenance_end_to_end_remaining < maintenance_window_seconds )); then
    maintenance_window_seconds=$maintenance_end_to_end_remaining
  fi
  (( maintenance_window_seconds >= 5 )) \
    || fail 'maintenance deadline budget is too small for bounded recovery'

  # Reserve 20% of the hard unavailable-window budget for rollback, capped at
  # 60 seconds.  The last portion of that reserve (up to five seconds) is kept
  # exclusively for a truthful final record and checkpoint/lock cleanup.
  maintenance_recovery_reserve_seconds=$(((maintenance_window_seconds + 4) / 5))
  if (( maintenance_window_seconds < 60 )); then
    maintenance_scaled_recovery_seconds=$(((maintenance_window_seconds * 2 + 2) / 3))
    (( maintenance_recovery_reserve_seconds < maintenance_scaled_recovery_seconds )) \
      && maintenance_recovery_reserve_seconds=$maintenance_scaled_recovery_seconds
  elif (( maintenance_recovery_reserve_seconds < 4 )); then
    maintenance_recovery_reserve_seconds=4
  fi
  (( maintenance_recovery_reserve_seconds > 60 )) && maintenance_recovery_reserve_seconds=60
  maintenance_finalization_reserve_seconds=$(((maintenance_recovery_reserve_seconds + 11) / 12))
  # A one-second finalization budget races the hard watchdog after a bounded
  # rollback command is terminated. Keep two seconds for the final record and
  # checkpoint cleanup, still within the same hard unavailable-window limit.
  if (( maintenance_window_seconds >= 20 && maintenance_window_seconds < 60 \
      && maintenance_finalization_reserve_seconds < 10 )); then
    maintenance_finalization_reserve_seconds=10
  elif (( maintenance_window_seconds >= 12 && maintenance_finalization_reserve_seconds < 4 )); then
    maintenance_finalization_reserve_seconds=4
  elif (( maintenance_finalization_reserve_seconds < 2 )); then
    maintenance_finalization_reserve_seconds=2
  fi
  if (( maintenance_window_seconds < 60 )); then
    (( maintenance_finalization_reserve_seconds > 10 )) && maintenance_finalization_reserve_seconds=10
  else
    (( maintenance_finalization_reserve_seconds > 5 )) && maintenance_finalization_reserve_seconds=5
  fi

  maintenance_forward_window_seconds=$((maintenance_window_seconds - maintenance_recovery_reserve_seconds))
  maintenance_rollback_window_seconds=$((maintenance_window_seconds - maintenance_finalization_reserve_seconds))
  (( maintenance_forward_window_seconds > 0 \
      && maintenance_rollback_window_seconds > maintenance_forward_window_seconds )) \
    || fail 'maintenance deadline budget cannot be partitioned safely'
  maintenance_deadline_epoch=$((maintenance_started_epoch + maintenance_window_seconds))
  maintenance_forward_deadline_epoch=$((maintenance_started_epoch + maintenance_forward_window_seconds))
  maintenance_rollback_deadline_epoch=$((maintenance_started_epoch + maintenance_rollback_window_seconds))
  maintenance_started_millis=$(monotonic_millis)
  # Stop forward work at its partitioned deadline so EXIT can still spend the
  # reserved rollback/finalization budget before the overall hard deadline.
  arm_deadline_watchdog "$maintenance_forward_window_seconds"
  trace_event 'maintenance stop api-worker'
  maintenance_stopped=true
  run_post_stop_command "${compose_current[@]}" stop sub2api-blue sub2api-green sub2api-worker >/dev/null
  # Rebase the process watchdog after the stop command so its duration tracks
  # the remaining absolute forward budget rather than double-counting setup.
  now=$(date -u +%s)
  forward_remaining=$((maintenance_forward_deadline_epoch - now))
  (( forward_remaining > 0 )) || fail 'maintenance forward budget expired while stopping API and worker'
  arm_deadline_watchdog "$forward_remaining"
  [[ "$(date -u +%s)" -lt "$maintenance_deadline_epoch" ]] || fail 'maintenance unavailable window expired after stopping API and worker'
  trace_event 'maintenance start worker for migrations'
  if [[ "$preloaded_image" == false ]]; then
    run_post_stop_command "${compose_candidate[@]}" pull sub2api-worker >/dev/null
  fi
  run_post_stop_command "${compose_candidate[@]}" up --no-deps -d "${compose_pull_args[@]+${compose_pull_args[@]}}" --force-recreate sub2api-worker >/dev/null
  worker_update_started=true
  wait_for_worker_healthy || fail 'maintenance worker did not become healthy before timeout'
  [[ "$(run_post_stop_command docker inspect "$(resolve_container_id sub2api-worker)" --format '{{.Image}}')" == "$candidate_worker_image_id" ]] \
    || fail 'maintenance worker image ID differs from candidate'
  [[ "$(date -u +%s)" -lt "$maintenance_deadline_epoch" ]] || fail 'maintenance unavailable window expired while applying migrations'
fi
candidate_env="$record_root/.$attempt_id.candidate.env"
candidate_env_awk='
  /^SUB2API_BLUE_IMAGE=/ { print "SUB2API_BLUE_IMAGE=" blue; next }
  /^SUB2API_GREEN_IMAGE=/ { print "SUB2API_GREEN_IMAGE=" green; next }
  /^SUB2API_WORKER_IMAGE=/ { print "SUB2API_WORKER_IMAGE=" worker; next }
  { print }
'
run_post_stop_operation '
  release_env=$1 candidate_env=$2 blue=$3 green=$4 worker=$5 awk_program=$6
  cp "$release_env" "$candidate_env" && chmod 0600 "$candidate_env" &&
    awk -v blue="$blue" -v green="$green" -v worker="$worker" "$awk_program" "$candidate_env" >"$candidate_env.tmp" &&
    chmod 0600 "$candidate_env.tmp" && mv "$candidate_env.tmp" "$candidate_env"
' "$release_env" "$candidate_env" "$candidate_blue" "$candidate_green" "$candidate_worker" "$candidate_env_awk"
compose_candidate=(docker compose --project-name "$compose_project" --project-directory "$deploy_root"
  --env-file "$secret_env" --env-file "$candidate_env" -f "$base_compose")
if [[ "$preloaded_image" == false ]]; then
  run_post_stop_command "${compose_candidate[@]}" pull "sub2api-$candidate_slot" >/dev/null
fi

failure_reason=candidate_start_failed
check_maintenance_deadline
if [[ "$detector_enabled" == true ]]; then
  run_post_stop_command "${compose_candidate[@]}" up -d --force-recreate model-detector >/dev/null
  wait_for_detector_healthy || fail 'model detector did not become healthy before timeout'
fi
run_post_stop_command "${compose_candidate[@]}" up --no-deps -d "${compose_pull_args[@]+${compose_pull_args[@]}}" "sub2api-$candidate_slot" >/dev/null
write_partial candidate_started
	wait_for_candidate_healthy "sub2api-$candidate_slot" || fail 'candidate did not become healthy before timeout'
candidate_container_id=$(resolve_container_id "sub2api-$candidate_slot") || fail 'candidate container identity is not uniquely resolvable'
candidate_runtime_image_id=$(run_post_stop_command docker inspect "$candidate_container_id" --format '{{.Image}}') || fail 'candidate image ID could not be inspected'
[[ "$candidate_runtime_image_id" == "$requested_image_id" ]] || fail 'candidate image ID differs from requested image'
check_maintenance_deadline

write_acceptance_headers
network_name="${compose_project}_default"
candidate_url="http://sub2api-$candidate_slot:8080"
failure_reason=candidate_acceptance_failed
run_post_stop_operation '
  url=$1
  shift
  docker run "$@" "$url" | jq -e '\''.status == "ok"'\'' >/dev/null
' "$candidate_url/health" "${network_probe_pull_args[@]+${network_probe_pull_args[@]}}" --rm --network "$network_name" \
  "$network_curl_image" -fsS --connect-timeout 5 --max-time 15
run_post_stop_operation '
  url=$1
  shift
  docker run "$@" "$url" | jq -e '\''(.data // .).version | type == "string" and length > 0'\'' >/dev/null
' "$candidate_url/api/v1/admin/system/version" "${network_probe_pull_args[@]+${network_probe_pull_args[@]}}" --rm --network "$network_name" \
  --user 0:0 -v "$admin_header:/run/key:ro" "$network_curl_image" -fsS --connect-timeout 5 --max-time 15 -H @/run/key
run_post_stop_operation '
  url=$1
  shift
  docker run "$@" "$url" | jq -e '\''type == "object"'\'' >/dev/null
' "$candidate_url/api/v1/settings/public" "${network_probe_pull_args[@]+${network_probe_pull_args[@]}}" --rm --network "$network_name" \
  "$network_curl_image" -fsS --connect-timeout 5 --max-time 15
run_post_stop_operation '
  url=$1
  shift
  docker run "$@" "$url" | jq -e '\''.data | type == "array"'\'' >/dev/null
' "$candidate_url/v1/models" "${network_probe_pull_args[@]+${network_probe_pull_args[@]}}" --rm --network "$network_name" \
  --user 0:0 -v "$gateway_header:/run/key:ro" "$network_curl_image" -fsS --connect-timeout 5 --max-time 15 -H @/run/key
write_partial candidate_accepted
check_maintenance_deadline

failure_reason=caddy_validate_failed
run_caddy_config_command "$candidate_upstream" validate >/dev/null
write_partial caddy_validated

failure_reason=caddy_reload_failed
cutover_attempted=true
write_partial cutover_attempted
run_caddy_config_command "$candidate_upstream" reload >/dev/null
cutover_applied=true
write_partial cutover_applied

failure_reason=public_acceptance_failed
public_acceptance
write_partial public_accepted
check_maintenance_deadline

failure_reason=state_persist_failed
persistence_started=true
write_partial state_persisting
write_release_env_values "$candidate_blue" "$candidate_green" "$requested_image" \
  "$candidate_upstream" "$candidate_slot" "$previous_slot"
trace_event 'persist release-env'
write_state_values "$candidate_slot" "$candidate_upstream" "$candidate_blue" "$candidate_green" \
  "$requested_image" "$source_commit" "$source_tree" "$migrations_hash" \
  "$postgres_id" "$redis_id" "$caddy_id" \
  "$candidate_blue_image_id" "$candidate_green_image_id" "$candidate_worker_image_id"
trace_event 'persist release-state'
state_persisted=true
write_partial state_persisted
[[ "$(live_caddy_upstream)" == "$candidate_upstream" ]] || fail 'persisted route does not match live Caddy upstream'

failure_reason=worker_update_failed
worker_update_started=true
write_partial worker_updating
run_post_stop_command "${compose_current[@]}" up --no-deps -d "${compose_pull_args[@]+${compose_pull_args[@]}}" --force-recreate sub2api-worker >/dev/null
wait_for_worker_healthy || fail 'worker did not become healthy before timeout'
worker_logs_are_acceptable || fail 'worker logs contain a startup failure'
worker_runtime_image_id=$(run_post_stop_command docker inspect "$(resolve_container_id sub2api-worker)" --format '{{.Image}}') || fail 'updated worker image ID could not be inspected'
[[ "$worker_runtime_image_id" == "$candidate_worker_image_id" ]] || fail 'updated worker image ID differs from candidate'
write_partial worker_accepted

failure_reason=final_identity_check_failed
[[ "$(resolve_container_id postgres)" == "$postgres_id" ]] || fail 'PostgreSQL identity changed during release'
[[ "$(resolve_container_id redis)" == "$redis_id" ]] || fail 'Redis identity changed during release'
[[ "$(resolve_container_id caddy)" == "$caddy_id" ]] || fail 'Caddy identity changed during release'

write_final_record succeeded promoted ''
trace_event 'persist success-record'
record_finalized=true
run_post_stop_command rm -f -- "$partial_path" "$candidate_env" "$admin_header" "$gateway_header"
partial_path=''
candidate_env=''
cleanup_lock
printf '{"schema_version":1,"downtime_required":false,"result":"succeeded","active_slot":"%s","active_upstream":"%s","image":"%s"}\n' \
  "$candidate_slot" "$candidate_upstream" "$requested_image"
