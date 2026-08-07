#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
EXECUTOR=${EXECUTOR_UNDER_TEST:-"$ROOT/ops/deploy-sub2api-blue-green-host.sh"}
FIXTURE=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-blue-green-host.XXXXXX")
FIXTURE=$(cd "$FIXTURE" && pwd -P)
trap 'rm -rf -- "$FIXTURE"' EXIT

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

[[ -f "$EXECUTOR" ]] || fail "host executor does not exist: $EXECUTOR"

assert_rehearsal_topology_ready() {
  local topology image
  image="example.invalid/sub2api@sha256:$(printf '9%.0s' {1..64})"
  topology=$(env \
    POSTGRES_USER=sub2api POSTGRES_PASSWORD=rehearsal-postgres POSTGRES_DB=sub2api \
    REDIS_PASSWORD=rehearsal-redis ADMIN_EMAIL=admin@rehearsal.test ADMIN_PASSWORD=rehearsal-admin \
    JWT_SECRET=rehearsal-jwt TOTP_ENCRYPTION_KEY=rehearsal-totp \
    REHEARSAL_SUB2API_DATA_DIR="$FIXTURE/app-data" \
    REHEARSAL_POSTGRES_DATA_DIR="$FIXTURE/postgres-data" \
    REHEARSAL_REDIS_DATA_DIR="$FIXTURE/redis-data" \
    REHEARSAL_ROLLBACK_IMAGE="$image" SUB2API_BLUE_IMAGE="$image" \
    SUB2API_GREEN_IMAGE="$image" SUB2API_WORKER_IMAGE="$image" \
		SUB2API_RELEASE_ENV_FILE="$FIXTURE/rehearsal-release.env" \
    SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 REHEARSAL_FAIL_PUBLIC_ACCEPTANCE=false \
    docker compose -f "$ROOT/infra/compose.sub2api-rehearsal.yaml" config --format json) \
    || fail 'rehearsal Compose topology did not render'
  ruby -rjson -e '
    value = JSON.parse(STDIN.read)
    services = value.fetch("services")
    abort unless value["name"] == "sub2api-blue-green-rehearsal"
    abort unless %w[sub2api-blue sub2api-green sub2api-worker].all? { |name| services.key?(name) }
  ' <<<"$topology" 2>/dev/null || fail 'isolated two-slot rehearsal topology is not ready'
}

REAL_JQ=$(command -v jq)
IMAGE="example.invalid/sub2api@sha256:$(printf 'a%.0s' {1..64})"
SOURCE_COMMIT=$(printf 'b%.0s' {1..40})
SOURCE_TREE=$(printf 'c%.0s' {1..40})
TESTED_TREE=$SOURCE_TREE
MIGRATIONS_HASH=$(printf 'd%.0s' {1..64})
PREVIOUS_IMAGE="example.invalid/sub2api@sha256:$(printf 'e%.0s' {1..64})"
NETWORK_CURL_IMAGE="example.invalid/network-curl@sha256:$(printf 'f%.0s' {1..64})"
NETWORK_CURL_IMAGE_ALLOWLIST="$NETWORK_CURL_IMAGE"

setup_case() {
  CASE_DIR="$FIXTURE/$1"
  rm -rf -- "$CASE_DIR"
  mkdir -p "$CASE_DIR/bin" "$CASE_DIR/deploy" "$CASE_DIR/records"
  EVENT_LOG="$CASE_DIR/events.log"
  : >"$EVENT_LOG"
  printf 'secret=value\n' >"$CASE_DIR/secret.env"
  printf 'admin-test-key\n' >"$CASE_DIR/admin.key"
  printf 'gateway-test-key\n' >"$CASE_DIR/gateway.key"
  printf 'services: {}\n' >"$CASE_DIR/compose.yaml"
  cat >"$CASE_DIR/release.env" <<EOF
UNRELATED_SETTING=preserved
SUB2API_BLUE_IMAGE=$PREVIOUS_IMAGE
SUB2API_GREEN_IMAGE=$PREVIOUS_IMAGE
SUB2API_WORKER_IMAGE=$PREVIOUS_IMAGE
SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080
SUB2API_ACTIVE_SLOT=blue
SUB2API_PREVIOUS_SLOT=green
EOF
  "$REAL_JQ" -n \
    --arg image "$PREVIOUS_IMAGE" \
    --arg source_commit "$(printf 'f%.0s' {1..40})" \
    --arg source_tree "$(printf '1%.0s' {1..40})" \
    --arg migrations_hash "$MIGRATIONS_HASH" '
      {
        schema_version: 1,
        active_slot: "blue",
        active_upstream: "sub2api-blue:8080",
        blue_image: $image,
        green_image: $image,
        worker_image: $image,
        source_commit: $source_commit,
        source_tree: $source_tree,
        migrations_hash: $migrations_hash,
        postgres_id: "postgres-id",
        redis_id: "redis-id",
        caddy_id: "caddy-id"
      }
    ' >"$CASE_DIR/state.json"
  chmod 0600 "$CASE_DIR/state.json" "$CASE_DIR/release.env" "$CASE_DIR/secret.env" \
    "$CASE_DIR/admin.key" "$CASE_DIR/gateway.key"

  cat >"$CASE_DIR/bin/jq" <<EOF
#!/usr/bin/env bash
printf 'jq %s\n' "\$*" >>"\${FAKE_EVENT_LOG:?}"
exec "$REAL_JQ" "\$@"
EOF
  cat >"$CASE_DIR/bin/uname" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "${FAKE_UNAME:-Linux}"
EOF
  cat >"$CASE_DIR/bin/date" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *+%s*)
    sequence=${FAKE_EPOCH_SEQUENCE:-${FAKE_EPOCH:-1785513600}}
    count_file="${FAKE_EVENT_LOG:?}.date-count"
    count=0
    [[ -f "$count_file" ]] && count=$(cat "$count_file")
    count=$((count + 1))
    printf '%s\n' "$count" >"$count_file"
    printf '%s\n' "$sequence" | awk -F ',' -v position="$count" '{ if (position <= NF) print $position; else print $NF }'
    ;;
  *) printf '%s\n' "${FAKE_DATE:-20260731T160000Z}" ;;
esac
EOF
  cat >"$CASE_DIR/bin/sleep" <<'EOF'
#!/usr/bin/env bash
printf 'sleep %s\n' "$*" >>"${FAKE_EVENT_LOG:?}"
EOF
  cat >"$CASE_DIR/bin/stat" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
target="${@: -1}"
if [[ -n "${FAKE_ROOT_ONLY_STAGING:-}" && "$target" == "${FAKE_ROOT_ONLY_STAGING}"* ]]; then
  case "$2" in
    %u) printf '0\n' ;;
    %a) printf '600\n' ;;
    *) exit 1 ;;
  esac
else
  exec /usr/bin/stat "$@"
fi
EOF
  cat >"$CASE_DIR/bin/mkdir" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
/bin/mkdir "$@"
if [[ "${FAKE_PAUSE_AFTER_LOCK_MKDIR:-}" == 1 && "$1" == "${FAKE_LOCK_DIR:-}" ]]; then
  : >"${FAKE_LOCK_CREATED_FILE:?}"
  while [[ ! -e "${FAKE_LOCK_RELEASE_FILE:?}" ]]; do
    /bin/sleep 1
  done
fi
EOF
  cat >"$CASE_DIR/kill-hook.bash" <<'EOF'
kill() {
  if [[ "${FAKE_PAUSE_AFTER_DEAD_PID_KILL:-}" == 1 \
      && "$#" -eq 2 \
      && "$1" == -0 \
      && "$2" == "${FAKE_DEAD_PID:?}" ]]; then
    if builtin kill "$@"; then
      return 0
    fi
    printf '%s\n' "$2" >"${FAKE_DEAD_PID_READY_FILE:?}"
    while [[ ! -e "${FAKE_DEAD_PID_RELEASE_FILE:?}" ]]; do
      /bin/sleep 1
    done
    return 1
  fi
  builtin kill "$@"
}
EOF
  cat >"$CASE_DIR/bin/df" <<'EOF'
#!/usr/bin/env bash
printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n'
printf '/dev/fake 99999999 1 %s 1%% /\n' "${FAKE_DISK_KB:-4194304}"
EOF
  cat >"$CASE_DIR/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'curl %s\n' "$*" >>"${FAKE_EVENT_LOG:?}"
case "${FAKE_SCENARIO:-success}:$*" in
	public_failure:*example.invalid*) exit 22 ;;
	rollback_shared_id_drift:*example.invalid*) [[ -e "${FAKE_EVENT_LOG}.live-route-green" ]] && exit 22 ;;
	caddy_rollback_failure:*example.invalid*) exit 22 ;;
esac
case "$*" in
  *'/health'*) printf '{"status":"ok"}\n' ;;
  *'/api/v1/admin/system/version'*) printf '{"data":{"version":"1.2.3"}}\n' ;;
  *'/v1/models'*) printf '{"data":[]}\n' ;;
  *) printf '{}\n' ;;
esac
EOF
  cat >"$CASE_DIR/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >>"${FAKE_EVENT_LOG:?}"
scenario=${FAKE_SCENARIO:-success}
case "$*" in
  'context show') printf '%s\n' "${FAKE_DOCKER_CONTEXT:-default}" ;;
  load\ --input\ *)
    [[ "$scenario" != preload_load_failure ]] || exit 1
    ;;
	  image\ inspect\ --format\ \{\{.Id\}\}\ *)
	    image_ref="${*: -1}"
	    [[ "$scenario" != recovery_unused_image_missing || "$image_ref" != "${EXPECTED_IMAGE:?}" ]] || exit 1
	    if [[ "$image_ref" == "${PREVIOUS_IMAGE_FOR_FAKE:-}" ]]; then
	      printf '%s\n' "${PREVIOUS_IMAGE_ID_FOR_FAKE:?}"
	    else
	      printf '%s\n' "${EXPECTED_IMAGE_ID:-sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}"
	    fi
	    ;;
	  image\ inspect*)
	    [[ "${FAKE_NETWORK_PROBE_IMAGE_MISSING:-false}" != true || "${*: -1}" != "${NETWORK_CURL_IMAGE:-}" ]] || exit 1
		[[ "$scenario" != image_unavailable ]] || exit 1
    qualified=true
    source_commit=${EXPECTED_SOURCE_COMMIT:?}
    source_tree=${EXPECTED_SOURCE_TREE:?}
    tested_tree=${EXPECTED_TESTED_TREE:?}
    migrations=${EXPECTED_MIGRATIONS_HASH:?}
    [[ "$scenario" == label_mismatch ]] && source_tree=$(printf '9%.0s' {1..40})
    cat <<JSON
[{"Id":"${EXPECTED_IMAGE_ID:-sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}","RepoDigests":["${EXPECTED_IMAGE:?}"],"Config":{"Labels":{"com.xingqiao.sub2api.qualified":"$qualified","com.xingqiao.sub2api.source.commit":"$source_commit","com.xingqiao.sub2api.source.tree":"$source_tree","com.xingqiao.sub2api.tested.tree":"$tested_tree","com.xingqiao.sub2api.migrations.sha256":"$migrations"}}}]
JSON
    ;;
  *' config --format json')
    role=api
    [[ "$scenario" == candidate_role ]] && role=worker
		if [[ "${FAKE_CANDIDATE_SLOT:-green}" == blue ]]; then
		  worker_image=${EXPECTED_WORKER_IMAGE:-${PREVIOUS_IMAGE_FOR_FAKE:?}}
		  printf '{"services":{"sub2api-green":{"image":"%s","environment":{"SERVER_PROCESS_ROLE":"api"}},"sub2api-blue":{"image":"%s","environment":{"SERVER_PROCESS_ROLE":"%s"}},"sub2api-worker":{"image":"%s","environment":{"SERVER_PROCESS_ROLE":"worker"}}}}\n' \
		    "${PREVIOUS_IMAGE_FOR_FAKE:?}" "${EXPECTED_IMAGE:?}" "$role" "$worker_image"
		else
		  worker_image=${EXPECTED_WORKER_IMAGE:-${PREVIOUS_IMAGE_FOR_FAKE:?}}
		  printf '{"services":{"sub2api-green":{"image":"%s","environment":{"SERVER_PROCESS_ROLE":"%s"}},"sub2api-blue":{"image":"%s","environment":{"SERVER_PROCESS_ROLE":"api"}},"sub2api-worker":{"image":"%s","environment":{"SERVER_PROCESS_ROLE":"worker"}}}}\n' \
		    "${EXPECTED_IMAGE:?}" "$role" "${PREVIOUS_IMAGE_FOR_FAKE:?}" "$worker_image"
    fi
    ;;
  *' ps -q postgres') printf 'postgres-id\n' ;;
  *' ps -q redis') printf 'redis-id\n' ;;
  *' ps -q caddy')
    [[ "$scenario" != rollback_shared_id_drift || ! -e "${FAKE_EVENT_LOG}.cutover-seen" ]] \
      || { printf 'changed-caddy-id\n'; exit 0; }
    printf 'caddy-id\n'
    ;;
  *' ps -q sub2api-blue') printf 'blue-id\n' ;;
  *' ps -q sub2api-green') printf 'green-id\n' ;;
	*' ps -q sub2api-worker')
		if [[ "$scenario" == multiple_workers ]]; then printf 'worker-id\nworker-id-2\n'; else printf 'worker-id\n'; fi
		;;
	'ps -q --filter label=com.docker.compose.project=sub2api --filter label=com.docker.compose.service=sub2api')
		[[ "$scenario" != legacy_all_role ]] || printf 'legacy-id\n'
		;;
	'inspect blue-id --format {{.Config.Image}}')
		if [[ "$scenario" == active_image_drift ]]; then printf '%s\n' "${EXPECTED_IMAGE:?}"; else printf '%s\n' "${PREVIOUS_IMAGE_FOR_FAKE:?}"; fi
		;;
	'inspect green-id --format {{.Config.Image}}')
		if [[ -e "${FAKE_EVENT_LOG}.live-route-green" ]]; then
			printf '%s\n' "${PREVIOUS_IMAGE_FOR_FAKE:?}"
		else
			printf '%s\n' "${EXPECTED_IMAGE:?}"
		fi
		;;
	'inspect blue-id --format {{.Image}}')
		if [[ -e "${FAKE_EVENT_LOG}.live-route-green" ]]; then printf '%s\n' "${EXPECTED_IMAGE_ID:?}"; else printf '%s\n' "${PREVIOUS_IMAGE_ID_FOR_FAKE:?}"; fi
		;;
	'inspect green-id --format {{.Image}}')
		if [[ -e "${FAKE_EVENT_LOG}.live-route-green" ]]; then printf '%s\n' "${PREVIOUS_IMAGE_ID_FOR_FAKE:?}"; else printf '%s\n' "${EXPECTED_IMAGE_ID:?}"; fi
		;;
	'inspect worker-id --format {{.Image}}')
			worker_image_id_file="${FAKE_EVENT_LOG}.worker-image-id"
			if [[ -f "$worker_image_id_file" ]]; then cat "$worker_image_id_file"; else printf '%s\n' "${PREVIOUS_IMAGE_ID_FOR_FAKE:?}"; fi
			;;
	'inspect worker-id --format {{.Config.Image}}') printf '%s\n' "${PREVIOUS_IMAGE_FOR_FAKE:?}" ;;
	'inspect blue-id --format {{range .Config.Env}}{{println .}}{{end}}')
		if [[ "$scenario" == active_role_all ]]; then printf 'SERVER_PROCESS_ROLE=all\n'; else printf 'SERVER_PROCESS_ROLE=api\n'; fi
		;;
	'inspect green-id --format {{range .Config.Env}}{{println .}}{{end}}') printf 'SERVER_PROCESS_ROLE=api\n' ;;
	'inspect worker-id --format {{range .Config.Env}}{{println .}}{{end}}')
		if [[ "$scenario" == worker_role_all ]]; then printf 'SERVER_PROCESS_ROLE=all\n'; else printf 'SERVER_PROCESS_ROLE=worker\n'; fi
		;;
	'inspect legacy-id --format {{range .Config.Env}}{{println .}}{{end}}') printf 'SERVER_PROCESS_ROLE=all\n' ;;
  *'exec -T postgres '*'psql'*) printf '%s\n' "${FAKE_DB_HEADROOM:-30}" ;;
  *'pull sub2api-green') : ;;
  *'up --no-deps -d sub2api-green')
    [[ "$scenario" != candidate_up_failure ]] || exit 1
    ;;
  *'run --rm --network '*'/health'|*'run --pull never --rm --network '*'/health'*)
    [[ "$scenario" != candidate_health_failure ]] || exit 1
    printf '{"status":"ok"}\n'
    ;;
  *'run --rm --network '*'/api/v1/admin/system/version'|*'run --pull never --rm --network '*'/api/v1/admin/system/version'*) printf '{"data":{"version":"1.2.3"}}\n' ;;
  *'run --rm --network '*'/api/v1/settings/public'|*'run --pull never --rm --network '*'/api/v1/settings/public'*) printf '{"data":{}}\n' ;;
  *'run --rm --network '*'/v1/models'|*'run --pull never --rm --network '*'/v1/models'*) printf '{"data":[]}\n' ;;
  *'exec -T -e SUB2API_ACTIVE_UPSTREAM='*' caddy caddy validate'*)
    [[ "$scenario" != caddy_validate_failure ]] || exit 1
    ;;
	*'exec -T -e SUB2API_ACTIVE_UPSTREAM='*' caddy caddy reload'*)
    if [[ "$scenario" == reload_failure && "$*" == *sub2api-green:8080* ]]; then exit 1; fi
    if [[ "$scenario" == caddy_rollback_failure && "$*" == *sub2api-blue:8080* ]]; then exit 1; fi
		if [[ "$*" == *sub2api-green:8080* ]]; then
			: >"${FAKE_EVENT_LOG}.cutover-seen"
			: >"${FAKE_EVENT_LOG}.live-route-green"
		else
			rm -f -- "${FAKE_EVENT_LOG}.live-route-green"
		fi
    ;;
	*'up --no-deps -d --force-recreate sub2api-worker'|*'up --no-deps -d --pull never --force-recreate sub2api-worker')
			: >"${FAKE_EVENT_LOG}.worker-recreated"
			count_file="${FAKE_EVENT_LOG}.worker-up-count"
			count=0
			[[ -f "$count_file" ]] && count=$(cat "$count_file")
			count=$((count + 1))
			printf '%s\n' "$count" >"$count_file"
			if [[ "$*" == *'.rollback.env'* ]]; then
				printf '%s\n' "${PREVIOUS_IMAGE_ID_FOR_FAKE:?}" >"${FAKE_EVENT_LOG}.worker-image-id"
			else
				printf '%s\n' "${EXPECTED_IMAGE_ID:?}" >"${FAKE_EVENT_LOG}.worker-image-id"
			fi
	    if [[ "$scenario" == worker_update_failure && ! -e "${FAKE_EVENT_LOG}.worker-failed" ]]; then
	      : >"${FAKE_EVENT_LOG}.worker-failed"
      exit 1
    fi
    if [[ "$scenario" == worker_rollback_failure ]]; then
      count_file="${FAKE_EVENT_LOG}.worker-up-count"
      count=0
      [[ -f "$count_file" ]] && count=$(cat "$count_file")
      count=$((count + 1))
      printf '%s\n' "$count" >"$count_file"
      [[ "$count" -lt 2 ]] || exit 1
    fi
    ;;
  'inspect worker-id --format {{.State.Health.Status}}')
    if [[ "$scenario" == worker_starting_then_healthy ]]; then
      count_file="${FAKE_EVENT_LOG}.worker-health-count"
      count=0
      [[ -f "$count_file" ]] && count=$(cat "$count_file")
      count=$((count + 1))
      printf '%s\n' "$count" >"$count_file"
      [[ "$count" -lt 2 ]] && { printf 'starting\n'; exit 0; }
    fi
    [[ "$scenario" != worker_health_failure && "$scenario" != worker_health_timeout && "$scenario" != worker_rollback_failure ]] || { printf 'unhealthy\n'; exit 0; }
    printf 'healthy\n'
    ;;
	'inspect green-id --format {{.State.Health.Status}}')
		if [[ "$scenario" == candidate_starting_then_healthy ]]; then
			count_file="${FAKE_EVENT_LOG}.candidate-health-count"
			count=0
			[[ -f "$count_file" ]] && count=$(cat "$count_file")
			count=$((count + 1))
			printf '%s\n' "$count" >"$count_file"
			[[ "$count" -lt 2 ]] && { printf 'starting\n'; exit 0; }
		fi
		[[ "$scenario" != candidate_unhealthy ]] || { printf 'unhealthy\n'; exit 0; }
		[[ "$scenario" != candidate_health_timeout ]] || { printf 'starting\n'; exit 0; }
		printf 'healthy\n'
		;;
	'inspect blue-id --format {{.State.Health.Status}}') printf 'healthy\n' ;;
	*'exec -T caddy wget -qO- http://127.0.0.1:2019/config/'*)
		upstream=sub2api-blue:8080
		[[ "$scenario" == live_route_green || -e "${FAKE_EVENT_LOG}.live-route-green" ]] && upstream=sub2api-green:8080
		printf '{"apps":{"http":{"servers":{"srv0":{"routes":[{"handle":[{"upstreams":[{"dial":"%s"}]}]}]}}}}}\n' "$upstream"
		;;
  *'logs --no-color --tail 200 sub2api-worker')
    [[ "$scenario" != worker_request_failure_log ]] || { printf 'sub2api-worker-1  | Request failed: upstream timeout\n'; exit 0; }
    [[ "$scenario" != worker_log_failure ]] || { printf 'panic: worker failed\n'; exit 0; }
    printf 'worker ready\n'
    ;;
  *) : ;;
esac
EOF
  chmod +x "$CASE_DIR/bin/"*
}

run_executor() {
	local executor_mode=${EXECUTOR_MODE:-production}
  local expected_worker_image=$PREVIOUS_IMAGE
  local requested_image=$IMAGE
  if [[ "${PRELOADED_MODE:-false}" == true ]]; then
    requested_image=${PRELOADED_REQUESTED_IMAGE:?}
  fi
  local executor_args=(
    --mode "$executor_mode"
    --image "$requested_image"
    --source-commit "$SOURCE_COMMIT"
    --source-tree "$SOURCE_TREE"
    --tested-tree "$TESTED_TREE"
    --migrations-hash "$MIGRATIONS_HASH"
    --deadline-epoch "${RELEASE_DEADLINE_EPOCH:-1785515400}"
  )
  if [[ "${PRELOADED_MODE:-false}" == true ]]; then
    executor_args+=(
      --preloaded-archive "${PRELOADED_ARCHIVE:?}"
      --preloaded-archive-sha256 "${PRELOADED_ARCHIVE_SHA256:?}"
      --preloaded-image-id "${PRELOADED_IMAGE_ID:?}"
    )
  fi
  if [[ "${MAINTENANCE_MODE:-false}" == true ]]; then
    expected_worker_image=$IMAGE
    executor_args+=(--maintenance-authorized --maintenance-from-hash "${MAINTENANCE_FROM_HASH:?}")
  fi
  env \
    PATH="$CASE_DIR/bin:$PATH" \
    BASH_ENV="$CASE_DIR/kill-hook.bash" \
    FAKE_EVENT_LOG="$EVENT_LOG" \
    RELEASE_EVENT_LOG="$EVENT_LOG" \
    EXPECTED_IMAGE="$requested_image" \
    EXPECTED_IMAGE_ID="${PRELOADED_IMAGE_ID:-sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa}" \
    EXPECTED_SOURCE_COMMIT="$SOURCE_COMMIT" \
    EXPECTED_SOURCE_TREE="$SOURCE_TREE" \
    EXPECTED_TESTED_TREE="$TESTED_TREE" \
    EXPECTED_MIGRATIONS_HASH="$MIGRATIONS_HASH" \
    EXPECTED_WORKER_IMAGE="${EXPECTED_WORKER_IMAGE_OVERRIDE:-$expected_worker_image}" \
    PREVIOUS_IMAGE_FOR_FAKE="${PREVIOUS_IMAGE_FOR_FAKE:-$PREVIOUS_IMAGE}" \
    PREVIOUS_IMAGE_ID_FOR_FAKE="sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" \
    DEPLOY_ROOT="$CASE_DIR/deploy" \
    BASE_COMPOSE="$CASE_DIR/compose.yaml" \
    SECRET_ENV="$CASE_DIR/secret.env" \
    RELEASE_ENV="$CASE_DIR/release.env" \
    RELEASE_STATE="$CASE_DIR/state.json" \
    RELEASE_RECORD_ROOT="$CASE_DIR/records" \
    BASE_URL="https://example.invalid" \
    ADMIN_API_KEY_FILE="$CASE_DIR/admin.key" \
    GATEWAY_API_KEY_FILE="$CASE_DIR/gateway.key" \
    NETWORK_CURL_IMAGE="$NETWORK_CURL_IMAGE" \
    NETWORK_CURL_IMAGE_ALLOWLIST="$NETWORK_CURL_IMAGE_ALLOWLIST" \
    MEMINFO_FILE="$CASE_DIR/meminfo" \
    RELEASE_PRELOADED_IMAGE="${PRELOADED_MODE:-false}" \
    WORKER_HEALTH_TIMEOUT_SECONDS=2 \
    WORKER_HEALTH_POLL_SECONDS=1 \
    CANDIDATE_HEALTH_TIMEOUT_SECONDS=2 \
    CANDIDATE_HEALTH_POLL_SECONDS=1 \
    COMPOSE_PROJECT_NAME=sub2api \
    RELEASE_STAGING_ROOT="${PRELOADED_STAGING_ROOT:-}" \
    "$@" bash "$EXECUTOR" \
    "${executor_args[@]}"
}

run_rehearsal_executor() {
	EXECUTOR_MODE=rehearsal run_executor "$@"
}

test_final_review_rehearsal_isolation() {
	setup_case rehearsal_production_scope
	write_meminfo
	expect_failure rehearsal_production_scope run_rehearsal_executor
	assert_no_mutation rehearsal_production_scope
}

test_final_review_candidate_readiness() {
	setup_case candidate_starting_then_healthy
	write_meminfo
	run_executor FAKE_SCENARIO=candidate_starting_then_healthy >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" \
		|| fail "delayed candidate readiness should succeed: $(cat "$CASE_DIR/stderr")"
	[[ "$(grep -c 'inspect green-id --format {{.State.Health.Status}}' "$EVENT_LOG" || true)" -ge 2 ]] \
		|| fail 'candidate starting state was not polled'
	grep -q '^sleep 1$' "$EVENT_LOG" || fail 'candidate readiness did not wait before retrying'

	setup_case candidate_unhealthy
	write_meminfo
	expect_failure candidate_unhealthy run_executor FAKE_SCENARIO=candidate_unhealthy
	! grep -q 'caddy caddy reload' "$EVENT_LOG" || fail 'unhealthy candidate reached Caddy mutation'

	setup_case candidate_timeout
	write_meminfo
	expect_failure candidate_timeout run_executor FAKE_SCENARIO=candidate_health_timeout \
		FAKE_EPOCH_SEQUENCE=1785513600,1785513600,1785513602 CANDIDATE_HEALTH_TIMEOUT_SECONDS=1
	grep -q 'candidate did not become healthy before timeout' "$CASE_DIR/stderr" \
		|| fail 'candidate readiness timeout was not bounded'
}

test_final_review_runtime_singletons() {
	setup_case worker_role_all
	write_meminfo
	expect_failure worker_role_all run_executor FAKE_SCENARIO=worker_role_all
	assert_no_mutation worker_role_all

	setup_case legacy_all_role
	write_meminfo
	expect_failure legacy_all_role run_executor FAKE_SCENARIO=legacy_all_role
	assert_no_mutation legacy_all_role

	for scenario in multiple_workers active_image_drift active_role_all; do
		setup_case "$scenario"
		write_meminfo
		expect_failure "$scenario" run_executor FAKE_SCENARIO="$scenario"
		assert_no_mutation "$scenario"
	done
}

test_final_review_recovery_precedes_new_image() {
  setup_case recovery_before_image
  write_meminfo
  write_review_partial "$CASE_DIR/records/recovery-before-image.partial" recovery-before-image true true true green sub2api-green:8080 "$IMAGE"
  expect_failure recovery_before_image run_executor FAKE_SCENARIO=image_unavailable
  grep -q 'SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 caddy caddy reload' "$EVENT_LOG" \
    || fail 'partial recovery did not run before rejecting the unavailable next image'
}

test_preloaded_partial_recovery_precedes_probe_image_check() {
  setup_case preloaded_partial_probe_missing
  write_meminfo
  local staging="$CASE_DIR/staging" archive="$CASE_DIR/staging/sub2api.tar" image_id=sha256:1111111111111111111111111111111111111111111111111111111111111111 archive_sha
  mkdir -p "$staging"
  printf 'preloaded image archive\n' >"$archive"
  archive_sha=$(sha256sum "$archive" | awk '{print $1}')
  write_review_partial "$CASE_DIR/records/preloaded-partial" preloaded-partial true true true green sub2api-green:8080 "example.invalid/sub2api:release-$SOURCE_COMMIT"
  mv "$CASE_DIR/records/preloaded-partial" "$CASE_DIR/records/preloaded-partial.partial"
  if PRELOADED_MODE=true \
    PRELOADED_REQUESTED_IMAGE="example.invalid/sub2api:release-$SOURCE_COMMIT-${image_id#sha256:}" \
    PRELOADED_ARCHIVE="$archive" PRELOADED_ARCHIVE_SHA256="$archive_sha" PRELOADED_IMAGE_ID="$image_id" \
    PRELOADED_STAGING_ROOT="$staging" FAKE_ROOT_ONLY_STAGING="$staging" FAKE_NETWORK_PROBE_IMAGE_MISSING=true \
    run_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'preloaded partial recovery unexpectedly succeeded'
  fi
  grep -q 'SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 caddy caddy reload' "$EVENT_LOG" \
    || fail 'preloaded partial recovery did not restore the previous upstream before probe image validation'
  ! grep -q "docker image inspect $NETWORK_CURL_IMAGE" "$EVENT_LOG" \
    || fail 'preloaded partial recovery checked the next probe image before rollback'
}

test_release_tag_partial_recovery() {
  setup_case release_tag_partial
  write_meminfo
  local image_id=1111111111111111111111111111111111111111111111111111111111111111
  local release_tag="example.invalid/sub2api:release-$SOURCE_COMMIT-$image_id"
  write_review_partial "$CASE_DIR/records/release-tag.partial" release-tag true true true green sub2api-green:8080 "$release_tag"
  "$REAL_JQ" --arg image "$release_tag" '
    .previous.blue_image=$image |
    .previous.green_image=$image |
    .previous.worker_image=$image
  ' "$CASE_DIR/records/release-tag.partial" >"$CASE_DIR/records/release-tag.partial.tmp"
  mv "$CASE_DIR/records/release-tag.partial.tmp" "$CASE_DIR/records/release-tag.partial"
  chmod 0600 "$CASE_DIR/records/release-tag.partial"
  expect_failure release_tag_partial run_executor FAKE_SCENARIO=image_unavailable
  grep -q 'SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 caddy caddy reload' "$EVENT_LOG" \
    || fail 'ID-bound release-tag partial recovery did not restore the previous upstream'
}

test_schema_v1_interrupted_partial_recovers_with_full_image_ids() {
  setup_case schema_v1_interrupted_partial
  write_meminfo
  expect_failure schema_v1_interrupted_partial run_executor FAKE_SCENARIO=worker_rollback_failure

  partial=$(find "$CASE_DIR/records" -maxdepth 1 -name '*.partial' -print -quit)
  [[ -n "$partial" ]] || fail 'v1 interruption did not retain a recovery checkpoint'
  "$REAL_JQ" -e \
    '[.previous.blue_image_id, .previous.green_image_id, .previous.worker_image_id, .candidate.image_id] |
     all(type == "string" and test("^sha256:[a-f0-9]{64}$"))' \
    "$partial" >/dev/null || fail 'v1 interruption checkpoint omitted immutable rollback image IDs'

  expect_failure schema_v1_interrupted_recovery run_executor
  "$REAL_JQ" -e \
    '.schema_version == 2 and
     ([.blue_image_id, .green_image_id, .worker_image_id] |
      all(type == "string" and test("^sha256:[a-f0-9]{64}$")))' \
    "$CASE_DIR/state.json" >/dev/null || fail 'v1 interruption recovery did not persist complete schema v2 image IDs'
}

test_legacy_partial_recovery_writes_complete_schema_v2_image_ids() {
  setup_case legacy_partial_recovery
  write_meminfo
  write_review_partial "$CASE_DIR/records/legacy.partial" legacy true true true green sub2api-green:8080 "$IMAGE"

  expect_failure legacy_partial_recovery run_executor
  "$REAL_JQ" -e \
    '.schema_version == 2 and
     ([.blue_image_id, .green_image_id, .worker_image_id] |
      all(type == "string" and test("^sha256:[a-f0-9]{64}$")))' \
    "$CASE_DIR/state.json" >/dev/null \
    || fail 'legacy partial recovery did not persist complete schema v2 image IDs'
}

test_release_tag_image_ids_must_match_tags() {
  local expected_id=1111111111111111111111111111111111111111111111111111111111111111
  local mismatched_id=2222222222222222222222222222222222222222222222222222222222222222
  local release_tag="example.invalid/sub2api:release-$SOURCE_COMMIT-$expected_id"

  setup_case release_tag_state_id_mismatch
  write_meminfo
  "$REAL_JQ" --arg image "$release_tag" --arg id "sha256:$mismatched_id" '
    .schema_version=2 |
    .blue_image=$image | .green_image=$image | .worker_image=$image |
    .blue_image_id=$id | .green_image_id=$id | .worker_image_id=$id
  ' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"
  chmod 0600 "$CASE_DIR/state.json"
  expect_failure release_tag_state_id_mismatch run_executor
  assert_no_mutation release_tag_state_id_mismatch

  setup_case release_tag_partial_id_mismatch
  write_meminfo
  write_review_partial "$CASE_DIR/records/release-tag.partial" release-tag true true true green sub2api-green:8080 "$release_tag"
  "$REAL_JQ" --arg image "$release_tag" --arg id "sha256:$mismatched_id" '
    .previous.blue_image=$image | .previous.green_image=$image | .previous.worker_image=$image |
    .previous.blue_image_id=$id | .previous.green_image_id=$id | .previous.worker_image_id=$id |
    .candidate.image=$image | .candidate.image_id=$id
  ' "$CASE_DIR/records/release-tag.partial" >"$CASE_DIR/records/release-tag.partial.tmp"
  mv "$CASE_DIR/records/release-tag.partial.tmp" "$CASE_DIR/records/release-tag.partial"
  chmod 0600 "$CASE_DIR/records/release-tag.partial"
  expect_failure release_tag_partial_id_mismatch run_executor
  ! grep -q 'caddy caddy reload' "$EVENT_LOG" || fail 'mismatched release-tag partial reached rollback mutation'
}

test_digest_image_ids_must_match_local_images() {
  local previous_id='sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee'
  local mismatched_id='sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff'

  setup_case digest_state_id_mismatch
  write_meminfo
  "$REAL_JQ" --arg previous_id "$previous_id" --arg mismatched_id "$mismatched_id" '
    .schema_version=2 |
    .blue_image_id=$previous_id | .green_image_id=$mismatched_id | .worker_image_id=$previous_id
  ' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"
  chmod 0600 "$CASE_DIR/state.json"
  expect_failure digest_state_id_mismatch run_executor
  assert_no_mutation digest_state_id_mismatch

  setup_case digest_partial_previous_id_mismatch
  write_meminfo
  write_review_partial "$CASE_DIR/records/digest.partial" digest true true true green sub2api-green:8080 "$IMAGE"
  "$REAL_JQ" --arg previous_id "$previous_id" --arg mismatched_id "$mismatched_id" '
    .previous.blue_image_id=$mismatched_id |
    .previous.green_image_id=$previous_id |
    .previous.worker_image_id=$previous_id
  ' "$CASE_DIR/records/digest.partial" >"$CASE_DIR/records/digest.partial.tmp"
  mv "$CASE_DIR/records/digest.partial.tmp" "$CASE_DIR/records/digest.partial"
  chmod 0600 "$CASE_DIR/records/digest.partial"
  expect_failure digest_partial_previous_id_mismatch run_executor
  ! grep -q 'caddy caddy reload' "$EVENT_LOG" || fail 'mismatched digest partial previous image ID reached rollback mutation'

  setup_case partial_recovery_ignores_unneeded_candidate_image
  write_meminfo
  "$REAL_JQ" --arg image "$IMAGE" --arg previous_id "$previous_id" --arg candidate_id 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' '
    .schema_version=2 |
    .green_image=$image |
    .blue_image_id=$previous_id | .green_image_id=$candidate_id | .worker_image_id=$previous_id
  ' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"
  chmod 0600 "$CASE_DIR/state.json"
  sed -i.bak -e "s|^SUB2API_GREEN_IMAGE=.*|SUB2API_GREEN_IMAGE=$IMAGE|" "$CASE_DIR/release.env"
  rm -f -- "$CASE_DIR/release.env.bak"
  write_review_partial "$CASE_DIR/records/digest.partial" digest true true true green sub2api-green:8080 "$IMAGE"
  "$REAL_JQ" --arg previous_id "$previous_id" --arg candidate_id 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' '
    .previous.blue_image_id=$previous_id |
    .previous.green_image_id=$previous_id |
    .previous.worker_image_id=$previous_id |
    .candidate.image_id=$candidate_id
  ' "$CASE_DIR/records/digest.partial" >"$CASE_DIR/records/digest.partial.tmp"
  mv "$CASE_DIR/records/digest.partial.tmp" "$CASE_DIR/records/digest.partial"
  chmod 0600 "$CASE_DIR/records/digest.partial"
  expect_failure partial_recovery_ignores_unneeded_candidate_image run_executor FAKE_SCENARIO=recovery_unused_image_missing
  grep -q 'SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 caddy caddy reload' "$EVENT_LOG" \
    || fail 'partial recovery did not restore the prior route when candidate and inactive images were unavailable'
}

test_final_review_rollback_proof() {
	setup_case rollback_public_unhealthy
	write_meminfo
	expect_failure rollback_public_unhealthy run_executor FAKE_SCENARIO=public_failure
	record=$(find "$CASE_DIR/records" -maxdepth 1 -type f -name '*.json' -print -quit)
	"$REAL_JQ" -e '.state == "rollback_failed" and .rolled_back == false' "$record" >/dev/null \
		|| fail 'unhealthy previous public route was finalized as a completed rollback'
	[[ -n "$(find "$CASE_DIR/records" -maxdepth 1 -name '*.partial' -print -quit)" ]] \
		|| fail 'failed rollback proof discarded the recovery checkpoint'

	setup_case rollback_shared_id_drift
	write_meminfo
	expect_failure rollback_shared_id_drift run_executor FAKE_SCENARIO=rollback_shared_id_drift
	record=$(find "$CASE_DIR/records" -maxdepth 1 -type f -name '*.json' -print -quit)
	"$REAL_JQ" -e '.state == "rollback_failed" and .rolled_back == false' "$record" >/dev/null \
		|| fail 'shared-ID drift was finalized as a completed rollback'
	[[ -n "$(find "$CASE_DIR/records" -maxdepth 1 -name '*.partial' -print -quit)" ]] \
		|| fail 'shared-ID rollback failure discarded the recovery checkpoint'
}

test_final_review_live_route_mismatch() {
	setup_case live_route_mismatch
	write_meminfo
	expect_failure live_route_mismatch run_executor FAKE_SCENARIO=live_route_green
	assert_no_mutation live_route_mismatch
}

test_final_review_restart_stable_route() {
	setup_case restart_stable_route
	write_meminfo
	run_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" \
		|| fail "restart-stable route release failed: $(cat "$CASE_DIR/stderr")"
	grep -q 'exec -T caddy wget -qO- http://127.0.0.1:2019/config/' "$EVENT_LOG" \
		|| fail 'live Caddy route was not proved after persistence'
	grep -q '^SUB2API_ACTIVE_UPSTREAM=sub2api-green:8080$' "$CASE_DIR/release.env" \
		|| fail 'restart source did not persist the promoted route'
}

test_final_review_host_deadline() {
	setup_case expired_deadline
	write_meminfo
	expect_failure expired_deadline run_executor FAKE_EPOCH=1785515401
	assert_no_mutation expired_deadline
}

write_meminfo() {
  printf 'MemAvailable: %s kB\n' "${1:-2097152}" >"$CASE_DIR/meminfo"
}

set_active_green() {
  "$REAL_JQ" '
    .active_slot="green" |
    .active_upstream="sub2api-green:8080"
  ' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"
  chmod 0600 "$CASE_DIR/state.json"
  sed -i.bak \
    -e 's|^SUB2API_ACTIVE_UPSTREAM=.*|SUB2API_ACTIVE_UPSTREAM=sub2api-green:8080|' \
    -e 's|^SUB2API_ACTIVE_SLOT=.*|SUB2API_ACTIVE_SLOT=green|' \
    -e 's|^SUB2API_PREVIOUS_SLOT=.*|SUB2API_PREVIOUS_SLOT=blue|' "$CASE_DIR/release.env"
  rm -f -- "$CASE_DIR/release.env.bak"
  : >"${EVENT_LOG}.live-route-green"
}

assert_no_mutation() {
  ! grep -Eq 'docker .*compose .* up |caddy caddy reload' "$EVENT_LOG" || fail "$1 mutated Docker/Caddy"
  grep -q '^SUB2API_ACTIVE_SLOT=blue$' "$CASE_DIR/release.env" || fail "$1 rewrote release env"
}

expect_failure() {
  local label=$1
  shift
  if "$@" >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail "$label unexpectedly succeeded"
  fi
}

test_validation_failures() {
  setup_case validation
  write_meminfo
  local saved_image=$IMAGE
  IMAGE=bad
  expect_failure malformed run_executor
  IMAGE=$saved_image
  assert_no_mutation malformed

  setup_case label
  write_meminfo
  expect_failure label_mismatch run_executor FAKE_SCENARIO=label_mismatch
  assert_no_mutation label_mismatch

  setup_case trees
  write_meminfo
  TESTED_TREE=$(printf '2%.0s' {1..40})
  expect_failure tree_mismatch run_executor
  TESTED_TREE=$SOURCE_TREE
  assert_no_mutation tree_mismatch

  setup_case nonlinux
  write_meminfo
  expect_failure non_linux run_executor FAKE_UNAME=Darwin
  assert_no_mutation non_linux

  setup_case context
  write_meminfo
  expect_failure context run_executor FAKE_DOCKER_CONTEXT=remote
  assert_no_mutation context

  setup_case symlink
  write_meminfo
  mv "$CASE_DIR/state.json" "$CASE_DIR/real-state.json"
  ln -s "$CASE_DIR/real-state.json" "$CASE_DIR/state.json"
  expect_failure symlink run_executor
  assert_no_mutation symlink

  setup_case duplicate
  write_meminfo
  sed 's/"active_slot": "blue"/"active_slot": "blue", "active_slot": "green"/' "$CASE_DIR/state.json" >"$CASE_DIR/duplicate.json"
  mv "$CASE_DIR/duplicate.json" "$CASE_DIR/state.json"
  chmod 0600 "$CASE_DIR/state.json"
  expect_failure duplicate run_executor
  assert_no_mutation duplicate

  setup_case invalid_key
  write_meminfo
  "$REAL_JQ" '.unexpected=true' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"
  chmod 0600 "$CASE_DIR/state.json"
  expect_failure invalid_key run_executor
  assert_no_mutation invalid_key

  setup_case stale_partial
  write_meminfo
  printf '{"schema_version":1,"started_epoch":1}\n' >"$CASE_DIR/records/stale.partial"
  chmod 0600 "$CASE_DIR/records/stale.partial"
  expect_failure stale_partial run_executor
  assert_no_mutation stale_partial

  setup_case lock
  write_meminfo
  mkdir "$CASE_DIR/records/.blue-green.lock"
  expect_failure concurrent_lock run_executor
  assert_no_mutation concurrent_lock

  setup_case malformed_lock
  write_meminfo
  mkdir "$CASE_DIR/records/.blue-green.lock"
  printf 'not-a-pid\n' >"$CASE_DIR/records/.blue-green.lock/owner.pid"
  chmod 0600 "$CASE_DIR/records/.blue-green.lock/owner.pid"
  expect_failure malformed_lock run_executor
  assert_no_mutation malformed_lock

  setup_case live_lock
  write_meminfo
  mkdir "$CASE_DIR/records/.blue-green.lock"
  printf '%s\n' "$$" >"$CASE_DIR/records/.blue-green.lock/owner.pid"
  chmod 0600 "$CASE_DIR/records/.blue-green.lock/owner.pid"
  expect_failure live_lock run_executor
  assert_no_mutation live_lock
}

test_preloaded_transport_loads_archive_without_pull() {
  setup_case preloaded
  write_meminfo
  local staging="$CASE_DIR/staging" archive="$CASE_DIR/staging/sub2api.tar" image_id=sha256:1111111111111111111111111111111111111111111111111111111111111111 archive_sha
  mkdir -p "$staging"
  printf 'preloaded image archive\n' >"$archive"
  archive_sha=$(sha256sum "$archive" | awk '{print $1}')
  PRELOADED_MODE=true \
  PRELOADED_REQUESTED_IMAGE="example.invalid/sub2api:release-$SOURCE_COMMIT-${image_id#sha256:}" \
  PRELOADED_ARCHIVE="$archive" \
  PRELOADED_ARCHIVE_SHA256="$archive_sha" \
  PRELOADED_IMAGE_ID="$image_id" \
  PRELOADED_STAGING_ROOT="$staging" \
  FAKE_ROOT_ONLY_STAGING="$staging" \
    run_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" \
    || fail "preloaded release failed: $(cat "$CASE_DIR/stderr")"
  grep -q '^docker load --input ' "$EVENT_LOG" || fail 'preloaded release did not load the staged archive'
  ! grep -q 'pull sub2api-green' "$EVENT_LOG" || fail 'preloaded release attempted to pull the candidate image'
  [[ "$(grep -c '^docker run --pull never --rm ' "$EVENT_LOG" || true)" == 4 ]] \
    || fail 'preloaded network probes did not disable image pulls'
  probe_image_line=$(grep -n "^docker image inspect $NETWORK_CURL_IMAGE$" "$EVENT_LOG" | cut -d: -f1 | head -n1)
  first_probe_line=$(grep -n '^docker run --pull never --rm ' "$EVENT_LOG" | cut -d: -f1 | head -n1)
  [[ -n "$probe_image_line" && -n "$first_probe_line" && "$probe_image_line" -lt "$first_probe_line" ]] \
    || fail 'preloaded release did not verify the local network probe image before use'
  grep -q 'release-' "$CASE_DIR/release.env" || fail 'preloaded release did not persist the release tag'

  setup_case preloaded-mismatched-reference
  write_meminfo
  staging="$CASE_DIR/staging"
  archive="$CASE_DIR/staging/sub2api.tar"
  mkdir -p "$staging"
  printf 'preloaded image archive\n' >"$archive"
  archive_sha=$(sha256sum "$archive" | awk '{print $1}')
  if PRELOADED_MODE=true \
    PRELOADED_REQUESTED_IMAGE="example.invalid/sub2api:release-$SOURCE_COMMIT-$(printf '2%.0s' {1..64})" \
    PRELOADED_ARCHIVE="$archive" \
    PRELOADED_ARCHIVE_SHA256="$archive_sha" \
    PRELOADED_IMAGE_ID="$image_id" \
    PRELOADED_STAGING_ROOT="$staging" \
    FAKE_ROOT_ONLY_STAGING="$staging" \
    run_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'preloaded host accepted a tag bound to a different image ID'
  fi
  ! grep -q '^docker load --input ' "$EVENT_LOG" \
    || fail 'preloaded host loaded an archive before rejecting a mismatched image-ID-bound tag'
}

test_downtime_gates() {
  local scenario reason
  for scenario in migration legacy disk memory db active_pair identity candidate_role; do
    setup_case "gate-$scenario"
    write_meminfo
    reason=$scenario
    case "$scenario" in
      migration)
        MIGRATIONS_HASH=$(printf '7%.0s' {1..64})
        ;;
      legacy) rm -f "$CASE_DIR/state.json" ;;
      disk) export FAKE_DISK_KB=1024 ;;
      memory) write_meminfo 1024 ;;
      db) export FAKE_DB_HEADROOM=1 ;;
      active_pair)
        "$REAL_JQ" '.active_upstream="sub2api-green:8080"' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
        mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"; chmod 0600 "$CASE_DIR/state.json"
        ;;
      identity)
        "$REAL_JQ" '.postgres_id="different"' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
        mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"; chmod 0600 "$CASE_DIR/state.json"
        ;;
    esac
    if [[ "$scenario" == candidate_role ]]; then
      expect_failure "$scenario" run_executor FAKE_SCENARIO=candidate_role
    else
      expect_failure "$scenario" run_executor
    fi
    grep -q '"downtime_required"' "$CASE_DIR/stdout" || fail "$scenario did not print a JSON gate: $(cat "$CASE_DIR/stderr")"
    grep -q 'true' "$CASE_DIR/stdout" || fail "$scenario gate was not true"
    assert_no_mutation "$scenario gate"
    MIGRATIONS_HASH=$(printf 'd%.0s' {1..64})
    unset FAKE_DISK_KB FAKE_DB_HEADROOM || true
  done
}

test_authorized_maintenance_transition() {
  local old_hash=ac8b0b33d7ea31a1a4f0117716ba56efec4bd66be9c38267a88d4c512d01bf39
  local new_hash=0204f39423f3218ffa0c8d4e3d665f7113c4990610e0dd22e9f5910c4d578c6d
  local legacy_old_hash=c618fc284897bb24c662297ba6cb263064a1e04a024e5432f50f082ac7317408
  local legacy_new_hash=e95b3512ccfc5b5103b4547857c437338921fd6bb463b7f2078c9ee24da4f0fc

  setup_case maintenance_unauthorized
  write_meminfo
  MIGRATIONS_HASH=$new_hash
  "$REAL_JQ" --arg hash "$old_hash" '.migrations_hash=$hash' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"; chmod 0600 "$CASE_DIR/state.json"
  expect_failure maintenance_unauthorized run_executor
  grep -q 'migration_set_changed' "$CASE_DIR/stdout" || fail 'unauthorized migration transition was not gated'
  assert_no_mutation maintenance_unauthorized

  setup_case maintenance_illegal_set
  write_meminfo
  MIGRATIONS_HASH=$(printf '8%.0s' {1..64})
  "$REAL_JQ" --arg hash "$old_hash" '.migrations_hash=$hash' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"; chmod 0600 "$CASE_DIR/state.json"
  MAINTENANCE_MODE=true MAINTENANCE_FROM_HASH=$old_hash expect_failure maintenance_illegal_set run_executor
  grep -q 'migration_set_changed' "$CASE_DIR/stdout" || fail 'illegal migration set was not gated'
  assert_no_mutation maintenance_illegal_set

  setup_case maintenance_legacy_transition
  write_meminfo
  MIGRATIONS_HASH=$legacy_new_hash
  "$REAL_JQ" --arg hash "$legacy_old_hash" '.migrations_hash=$hash' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"; chmod 0600 "$CASE_DIR/state.json"
  MAINTENANCE_MODE=true MAINTENANCE_FROM_HASH=$legacy_old_hash expect_failure maintenance_legacy_transition run_executor
  grep -q 'approved active migration hash' "$CASE_DIR/stderr" || fail 'retired migration transition was not rejected'
  assert_no_mutation maintenance_legacy_transition

  setup_case maintenance_success
  write_meminfo
  MIGRATIONS_HASH=$new_hash
  "$REAL_JQ" --arg hash "$old_hash" '.migrations_hash=$hash' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"; chmod 0600 "$CASE_DIR/state.json"
  MAINTENANCE_MODE=true MAINTENANCE_FROM_HASH=$old_hash run_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" \
    || fail "authorized maintenance transition failed: $(cat "$CASE_DIR/stderr")"
  grep -q 'maintenance stop api-worker' "$EVENT_LOG" || fail 'maintenance path did not stop API and worker'
  ! grep -Eq 'compose .* stop .*postgres|compose .* stop .*redis|compose .* stop .*caddy' "$EVENT_LOG" \
    || fail 'maintenance path stopped a shared service'
  ! grep -Eq 'compose .* (up|pull|rm|restart|recreate).*postgres|compose .* (up|pull|rm|restart|recreate).*redis|compose .* (up|pull|rm|restart|recreate).*caddy' "$EVENT_LOG" \
    || fail 'maintenance path rebuilt a shared service'

  setup_case maintenance_rollback
  write_meminfo
  MIGRATIONS_HASH=$new_hash
  "$REAL_JQ" --arg hash "$old_hash" '.migrations_hash=$hash' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"; chmod 0600 "$CASE_DIR/state.json"
  MAINTENANCE_MODE=true MAINTENANCE_FROM_HASH=$old_hash expect_failure maintenance_rollback run_executor \
    FAKE_SCENARIO=candidate_health_failure
  grep -q 'maintenance stop api-worker' "$EVENT_LOG" || fail 'maintenance rollback did not enter maintenance path'
  grep -q 'up --no-deps -d sub2api-blue' "$EVENT_LOG" || fail 'maintenance rollback did not restore active API'
  grep -Eq 'up --no-deps .*--force-recreate sub2api-worker' "$EVENT_LOG" || fail 'maintenance rollback did not restore worker'
  ! grep -Eq 'compose .* (stop|up|pull|rm|restart|recreate).*postgres|compose .* (stop|up|pull|rm|restart|recreate).*redis|compose .* (stop|up|pull|rm|restart|recreate).*caddy' "$EVENT_LOG" \
    || fail 'maintenance rollback touched a shared service'
}

test_caddy_reconciliation_route() {
  sed -n '/handle @relay_ops_reconciliation {/,/^\t}/p' "$ROOT/infra/Caddyfile" | grep -q 'reverse_proxy relay-ops:8100' \
    || fail 'Caddy does not preserve reconciliation proxy ordering ahead of the retired page response'
  grep -q '/relay-ops/api/reconciliation/\*' "$ROOT/infra/Caddyfile" \
    || fail 'Caddy does not route reconciliation API to relay-ops'
  ! sed -n '/@relay_ops_public {/,/^\t}/p' "$ROOT/infra/Caddyfile" | grep -q '/relay-ops/\*' \
    || fail 'Caddy exposes retired relay-ops pages through the public matcher'
  grep -q '@retired_relay_ops_pages {' "$ROOT/infra/Caddyfile" \
    || fail 'Caddy does not explicitly retire relay-ops pages'
  sed -n '/@retired_relay_ops_pages {/,/^\t}/p' "$ROOT/infra/Caddyfile" | grep -q 'not path /relay-ops/api/reconciliation/\*' \
    || fail 'retired relay-ops matcher intercepts the reconciliation API before reverse_proxy'
}

test_success_order_and_atomic_records() {
  setup_case success
  write_meminfo
  run_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail "success path failed: $(cat "$CASE_DIR/stderr")"
  local previous=0 line current pattern
  for pattern in \
    'docker image inspect' \
    'docker compose .* ps -q postgres' \
    'docker compose .* pull sub2api-green' \
    'docker compose .* up --no-deps -d sub2api-green' \
    'docker run --rm --network sub2api_default .*health' \
    'caddy caddy validate' \
    'caddy caddy reload' \
    'curl .*https://example.invalid/health' \
    'persist release-env' \
    'persist release-state' \
    'docker compose .* up --no-deps -d --force-recreate sub2api-worker' \
    'docker inspect worker-id' \
    'docker compose .* ps -q postgres' \
    'persist success-record'; do
    line=$(awk -v pattern="$pattern" -v after="$previous" 'NR > after && $0 ~ pattern { print NR; exit }' "$EVENT_LOG")
    [[ -n "$line" && "$line" -gt "$previous" ]] || fail "successful order missing/out of order: $pattern"
    previous=$line
  done
  ! grep -Eq 'compose .* down( |$)|volume (rm|prune)|compose .* (up|rm|stop) .*postgres|compose .* (up|rm|stop) .*redis|compose .* (up|rm|stop) .*caddy|compose .*stop sub2api-blue|database restore' "$EVENT_LOG" \
    || fail 'success path used a prohibited destructive operation'
  grep -q '^UNRELATED_SETTING=preserved$' "$CASE_DIR/release.env" || fail 'release env lost unrelated settings'
  grep -q '^SUB2API_ACTIVE_SLOT=green$' "$CASE_DIR/release.env" || fail 'release env did not persist green'
  "$REAL_JQ" -e '.active_slot == "green" and .active_upstream == "sub2api-green:8080" and .worker_image == $image' \
    --arg image "$IMAGE" "$CASE_DIR/state.json" >/dev/null || fail 'state was not promoted atomically'
  "$REAL_JQ" -e '.schema_version == 2 and .blue_image_id == $previous_id and .green_image_id == $requested_id and .worker_image_id == $requested_id' \
    --arg previous_id "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" \
    --arg requested_id "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" \
    "$CASE_DIR/state.json" >/dev/null || fail 'state did not persist immutable runtime image IDs'
  [[ "$(stat -f '%Lp' "$CASE_DIR/state.json" 2>/dev/null || stat -c '%a' "$CASE_DIR/state.json")" == 600 ]] || fail 'state mode is not 0600'
  record=$(find "$CASE_DIR/records" -maxdepth 1 -type f -name '*.json' -print -quit)
  [[ -n "$record" ]] || fail 'success record missing'
  [[ "$(stat -f '%Lp' "$record" 2>/dev/null || stat -c '%a' "$record")" == 600 ]] || fail 'record mode is not 0600'
  "$REAL_JQ" -e '.result == "succeeded" and .state == "promoted"' "$record" >/dev/null || fail 'success record invalid'
  [[ -z "$(find "$CASE_DIR/records" -maxdepth 1 -name '*.partial' -print -quit)" ]] || fail 'partial record remained after success'
}

test_worker_request_failure_log_does_not_trigger_startup_failure() {
  setup_case worker_request_failure_log
  write_meminfo
  run_executor FAKE_SCENARIO=worker_request_failure_log >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" \
    || fail "business request failure in worker logs was misclassified as startup failure: $(cat "$CASE_DIR/stderr")"
  "$REAL_JQ" -e '.result == "succeeded" and .state == "promoted"' \
    "$(find "$CASE_DIR/records" -maxdepth 1 -type f -name '*.json' -print -quit)" >/dev/null \
    || fail 'business request failure log prevented a successful release record'
}

test_two_slot_rehearsal_cycles() {
  local started elapsed shared count scenario
  started=$SECONDS

  setup_case cycle_blue_to_green
  write_meminfo
  run_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail "blue-to-green rehearsal failed: $(cat "$CASE_DIR/stderr")"
  grep -q '^SUB2API_ACTIVE_SLOT=green$' "$CASE_DIR/release.env" || fail 'blue-to-green did not promote green'
  ! grep -Eq 'compose .* (stop|rm).*sub2api-blue|compose .* down' "$EVENT_LOG" || fail 'blue-to-green did not retain the prior blue slot'
  [[ "$(grep -c 'up --no-deps -d --force-recreate sub2api-worker' "$EVENT_LOG")" == 1 ]] || fail 'blue-to-green did not update exactly one worker'
  for shared in postgres redis caddy; do
    count=$(grep -c "ps -q $shared" "$EVENT_LOG")
    [[ "$count" -ge 2 ]] || fail "blue-to-green did not preserve and recheck $shared identity"
  done

  setup_case cycle_green_to_blue
  write_meminfo
  set_active_green
  [[ -e "${EVENT_LOG}.live-route-green" ]] || fail 'green-to-blue fixture did not mark the active green Caddy route'
	run_executor FAKE_CANDIDATE_SLOT=blue >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail "green-to-blue rehearsal failed: $(cat "$CASE_DIR/stderr")"
	grep -q '^SUB2API_ACTIVE_SLOT=blue$' "$CASE_DIR/release.env" || fail 'green-to-blue did not promote blue'
	"$REAL_JQ" -e '.schema_version == 2 and .blue_image_id == $requested_id and .green_image_id == $previous_id and .worker_image_id == $requested_id' \
		--arg previous_id "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" \
		--arg requested_id "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" \
		"$CASE_DIR/state.json" >/dev/null || fail 'green-to-blue did not persist the expected schema v2 runtime image IDs'
	! grep -Eq 'compose .* (stop|rm).*sub2api-green|compose .* down' "$EVENT_LOG" || fail 'green-to-blue did not retain the prior green slot'
  [[ "$(grep -c 'up --no-deps -d --force-recreate sub2api-worker' "$EVENT_LOG")" == 1 ]] || fail 'green-to-blue did not update exactly one worker'

  for scenario in candidate_health_failure reload_failure public_failure; do
    setup_case "cycle-$scenario"
    write_meminfo
    expect_failure "$scenario" run_executor FAKE_SCENARIO="$scenario"
    grep -q '^SUB2API_ACTIVE_SLOT=blue$' "$CASE_DIR/release.env" || fail "$scenario did not leave blue active"
    ! grep -Eq 'compose .* (stop|rm).*sub2api-blue|compose .* down' "$EVENT_LOG" || fail "$scenario stopped the active blue slot"
    if [[ "$scenario" == reload_failure || "$scenario" == public_failure ]]; then
      grep -q 'SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 caddy caddy reload' "$EVENT_LOG" \
        || fail "$scenario did not restore the previous Caddy upstream"
    fi
  done

  elapsed=$((SECONDS - started))
  (( elapsed < 1800 )) || fail "rehearsal fixture exceeded 1800 seconds: $elapsed"
  printf 'PASS: two-slot rehearsal cycles (%ss)\n' "$elapsed"
}

test_failures_and_recovery() {
  local scenario
  for scenario in candidate_health_failure caddy_validate_failure reload_failure public_failure worker_update_failure; do
    setup_case "$scenario"
    write_meminfo
    expect_failure "$scenario" run_executor FAKE_SCENARIO="$scenario"
    if [[ "$scenario" == public_failure || "$scenario" == worker_update_failure ]]; then
      grep -q 'SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 caddy caddy reload' "$EVENT_LOG" \
        || fail "$scenario did not reload the previous upstream"
      if [[ "$scenario" == worker_update_failure ]]; then
        count=$(grep -c 'up --no-deps -d --force-recreate sub2api-worker' "$EVENT_LOG")
        [[ "$count" -ge 2 ]] || fail 'worker failure did not restore the prior worker digest'
      fi
  grep -q '^SUB2API_ACTIVE_SLOT=blue$' "$CASE_DIR/release.env" || fail "$scenario did not restore release env"
		if [[ "$scenario" == worker_update_failure ]]; then
			record=$(find "$CASE_DIR/records" -maxdepth 1 -type f -name '*.json' -print -quit)
			"$REAL_JQ" -e '.state == "rolled_back" and .rolled_back == true' "$record" >/dev/null \
				|| fail 'worker rollback did not prove restoration of the old worker image ID'
			grep -q 'inspect worker-id --format {{.Image}}' "$EVENT_LOG" \
				|| fail 'worker rollback did not inspect the restored worker image ID'
		fi
    fi
    record=$(find "$CASE_DIR/records" -maxdepth 1 -type f -name '*.json' -print -quit)
    [[ -n "$record" ]] || fail "$scenario failure record missing"
    "$REAL_JQ" -e '.result == "failed"' "$record" >/dev/null || fail "$scenario failure record invalid"
  done

  setup_case restart_recovery
  write_meminfo
  cat >"$CASE_DIR/records/restart.partial" <<EOF
{"schema_version":1,"attempt_id":"restart","mode":"production","started_epoch":1785513590,"phase":"worker_update","cutover_attempted":true,"cutover_applied":true,"worker_updated":true,"previous":{"active_slot":"blue","active_upstream":"sub2api-blue:8080","blue_image":"$PREVIOUS_IMAGE","green_image":"$PREVIOUS_IMAGE","worker_image":"$PREVIOUS_IMAGE","source_commit":"$(printf 'f%.0s' {1..40})","source_tree":"$(printf '1%.0s' {1..40})","migrations_hash":"$MIGRATIONS_HASH","postgres_id":"postgres-id","redis_id":"redis-id","caddy_id":"caddy-id"},"candidate":{"slot":"green","upstream":"sub2api-green:8080","image":"$IMAGE"}}
EOF
  chmod 0600 "$CASE_DIR/records/restart.partial"
  if run_executor >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    fail 'restart recovery should finish the interrupted attempt as failed'
  fi
  grep -q 'SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 caddy caddy reload' "$EVENT_LOG" || fail 'restart did not restore previous upstream'
  grep -q 'up --no-deps -d --force-recreate sub2api-worker' "$EVENT_LOG" || fail 'restart did not restore previous worker'
  [[ ! -e "$CASE_DIR/records/restart.partial" ]] || fail 'recovered partial remains'
  [[ ! -e "$CASE_DIR/records/.blue-green.lock" ]] || fail 'recovered stale lock remains'
}

test_review_network_probe_image_policy() {
  setup_case network_probe_empty
  write_meminfo
  expect_failure network_probe_empty run_executor NETWORK_CURL_IMAGE=
  assert_no_mutation network_probe_empty

  setup_case network_probe_tag
  write_meminfo
  expect_failure network_probe_tag run_executor NETWORK_CURL_IMAGE=curlimages/curl:8.12.1
  assert_no_mutation network_probe_tag

  setup_case network_probe_unapproved
  write_meminfo
  expect_failure network_probe_unapproved run_executor \
    "NETWORK_CURL_IMAGE=example.invalid/unapproved@sha256:$(printf '0%.0s' {1..64})"
  assert_no_mutation network_probe_unapproved
}

test_review_worker_health_wait() {
  setup_case worker_starting_then_healthy
  write_meminfo
  run_executor FAKE_SCENARIO=worker_starting_then_healthy FAKE_EPOCH_SEQUENCE=1785513600,1785513601 \
    >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail "worker transition should succeed: $(cat "$CASE_DIR/stderr")"
  [[ "$(grep -c 'inspect worker-id' "$EVENT_LOG")" -ge 2 ]] || fail 'worker starting state was not polled'
  grep -q '^sleep 1$' "$EVENT_LOG" || fail 'worker starting state did not wait before retrying'

  setup_case worker_health_timeout
  write_meminfo
  expect_failure worker_health_timeout run_executor FAKE_SCENARIO=worker_health_timeout \
    FAKE_EPOCH_SEQUENCE=1785513600,1785513600,1785513602 WORKER_HEALTH_TIMEOUT_SECONDS=1
  grep -q 'worker did not become healthy before timeout' "$CASE_DIR/stderr" \
    || fail 'worker health timeout was not bounded'
}

write_review_partial() {
  local path=$1 attempt_id=$2 cutover_attempted=$3 cutover_applied=$4 worker_updated=$5 candidate_slot=$6 candidate_upstream=$7 candidate_image=$8
  cat >"$path" <<EOF
{"schema_version":1,"attempt_id":"$attempt_id","mode":"production","started_epoch":1785513590,"phase":"review","cutover_attempted":$cutover_attempted,"cutover_applied":$cutover_applied,"worker_updated":$worker_updated,"previous":{"active_slot":"blue","active_upstream":"sub2api-blue:8080","blue_image":"$PREVIOUS_IMAGE","green_image":"$PREVIOUS_IMAGE","worker_image":"$PREVIOUS_IMAGE","source_commit":"$(printf 'f%.0s' {1..40})","source_tree":"$(printf '1%.0s' {1..40})","migrations_hash":"$MIGRATIONS_HASH","postgres_id":"postgres-id","redis_id":"redis-id","caddy_id":"caddy-id"},"candidate":{"slot":"$candidate_slot","upstream":"$candidate_upstream","image":"$candidate_image"}}
EOF
  chmod 0600 "$path"
}

test_review_recovery_and_cleanup() {
  setup_case crash_after_reload
  write_meminfo
  write_review_partial "$CASE_DIR/records/reload.partial" reload true false false green sub2api-green:8080 "$IMAGE"
  expect_failure crash_after_reload run_executor
  grep -q 'SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 caddy caddy reload' "$EVENT_LOG" \
    || fail 'uncertain reload recovery did not restore previous upstream'

  setup_case credential_cleanup
  write_meminfo
  expect_failure credential_cleanup run_executor FAKE_SCENARIO=candidate_health_failure
  [[ -z "$(find "$CASE_DIR/records" -maxdepth 1 \( -name '*.admin.header' -o -name '*.gateway.header' \) -print -quit)" ]] \
    || fail 'credential header survived failed candidate acceptance'

  setup_case caddy_rollback_failure
  write_meminfo
  expect_failure caddy_rollback_failure run_executor FAKE_SCENARIO=caddy_rollback_failure
  [[ -n "$(find "$CASE_DIR/records" -maxdepth 1 -name '*.partial' -print -quit)" ]] \
    || fail 'failed Caddy rollback discarded recovery checkpoint'
  pulls_before=$(grep -c 'pull sub2api-green' "$EVENT_LOG" || true)
  expect_failure caddy_rollback_retry run_executor FAKE_SCENARIO=caddy_rollback_failure
  [[ "$(grep -c 'pull sub2api-green' "$EVENT_LOG" || true)" == "$pulls_before" ]] \
    || fail 'ordinary release continued after Caddy rollback failure'
  [[ -n "$(find "$CASE_DIR/records" -maxdepth 1 -name '*.partial' -print -quit)" ]] \
    || fail 'Caddy rollback failure checkpoint was discarded during recovery retry'
  pulls_before=$(grep -c 'pull sub2api-green' "$EVENT_LOG" || true)
  expect_failure caddy_rollback_blocked run_executor FAKE_SCENARIO=caddy_rollback_failure
  [[ "$(grep -c 'pull sub2api-green' "$EVENT_LOG" || true)" == "$pulls_before" ]] \
    || fail 'ordinary release was not blocked by retained rollback checkpoint'

  setup_case worker_rollback_failure
  write_meminfo
  expect_failure worker_rollback_failure run_executor FAKE_SCENARIO=worker_rollback_failure
  [[ -n "$(find "$CASE_DIR/records" -maxdepth 1 -name '*.partial' -print -quit)" ]] \
    || fail 'failed worker rollback discarded recovery checkpoint'
  grep -q '^SUB2API_ACTIVE_SLOT=green$' "$CASE_DIR/release.env" \
    || fail 'old release state was persisted before worker rollback verification'

  setup_case committed_success_partial
  write_meminfo
  "$REAL_JQ" --arg image "$IMAGE" '
    .active_slot="green" | .active_upstream="sub2api-green:8080" |
    .green_image=$image | .worker_image=$image
  ' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"
  chmod 0600 "$CASE_DIR/state.json"
  sed -i.bak \
    -e "s|^SUB2API_GREEN_IMAGE=.*|SUB2API_GREEN_IMAGE=$IMAGE|" \
    -e "s|^SUB2API_WORKER_IMAGE=.*|SUB2API_WORKER_IMAGE=$IMAGE|" \
    -e 's|^SUB2API_ACTIVE_UPSTREAM=.*|SUB2API_ACTIVE_UPSTREAM=sub2api-green:8080|' \
    -e 's|^SUB2API_ACTIVE_SLOT=.*|SUB2API_ACTIVE_SLOT=green|' \
    -e 's|^SUB2API_PREVIOUS_SLOT=.*|SUB2API_PREVIOUS_SLOT=blue|' "$CASE_DIR/release.env"
  rm -f -- "$CASE_DIR/release.env.bak"
  : >"${EVENT_LOG}.live-route-green"
  write_review_partial "$CASE_DIR/records/committed.partial" committed true true true green sub2api-green:8080 "$IMAGE"
  "$REAL_JQ" -n --arg image "$IMAGE" '
    {schema_version:1, attempt_id:"committed", mode:"production",
     requested:{image:$image}, result:"succeeded", state:"promoted", reason:"", rolled_back:false}
  ' >"$CASE_DIR/records/committed.json"
  chmod 0600 "$CASE_DIR/records/committed.json"
  EXPECTED_WORKER_IMAGE_OVERRIDE="$IMAGE" run_executor FAKE_CANDIDATE_SLOT=blue PREVIOUS_IMAGE_FOR_FAKE="$IMAGE" >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr" || fail "committed success cleanup should continue with the requested release: $(cat "$CASE_DIR/stderr")"
  [[ -z "$(find "$CASE_DIR/records" -maxdepth 1 -name 'committed.partial' -print -quit)" ]] \
    || fail 'committed success partial was not cleaned up'
  first_blue_reload=$(grep -n 'SUB2API_ACTIVE_UPSTREAM=sub2api-blue:8080 caddy caddy reload' "$EVENT_LOG" | head -n 1 | cut -d: -f1)
  first_blue_pull=$(grep -n 'pull sub2api-blue' "$EVENT_LOG" | head -n 1 | cut -d: -f1)
  [[ -n "$first_blue_reload" && -n "$first_blue_pull" && "$first_blue_pull" -lt "$first_blue_reload" ]] \
    || fail 'committed success was recovered as an incomplete release'

  setup_case malformed_committed_success_partial
  write_meminfo
  "$REAL_JQ" --arg image "$IMAGE" '
    .active_slot="green" | .active_upstream="sub2api-green:8080" |
    .green_image=$image | .worker_image=$image
  ' "$CASE_DIR/state.json" >"$CASE_DIR/state.tmp"
  mv "$CASE_DIR/state.tmp" "$CASE_DIR/state.json"
  chmod 0600 "$CASE_DIR/state.json"
  sed -i.bak \
    -e "s|^SUB2API_GREEN_IMAGE=.*|SUB2API_GREEN_IMAGE=$IMAGE|" \
    -e "s|^SUB2API_WORKER_IMAGE=.*|SUB2API_WORKER_IMAGE=$IMAGE|" \
    -e 's|^SUB2API_ACTIVE_UPSTREAM=.*|SUB2API_ACTIVE_UPSTREAM=sub2api-green:8080|' \
    -e 's|^SUB2API_ACTIVE_SLOT=.*|SUB2API_ACTIVE_SLOT=green|' \
    -e 's|^SUB2API_PREVIOUS_SLOT=.*|SUB2API_PREVIOUS_SLOT=blue|' "$CASE_DIR/release.env"
  rm -f -- "$CASE_DIR/release.env.bak"
  : >"${EVENT_LOG}.live-route-green"
  printf '{"attempt_id":"malformed-committed","candidate":{"slot":"green","upstream":"sub2api-green:8080","image":"%s"}}\n' "$IMAGE" \
    >"$CASE_DIR/records/malformed-committed.partial"
  chmod 0600 "$CASE_DIR/records/malformed-committed.partial"
  "$REAL_JQ" -n --arg image "$IMAGE" '
    {schema_version:1, attempt_id:"malformed-committed", mode:"production",
     requested:{image:$image}, result:"succeeded", state:"promoted", reason:"", rolled_back:false}
  ' >"$CASE_DIR/records/malformed-committed.json"
  chmod 0600 "$CASE_DIR/records/malformed-committed.json"
  expect_failure malformed_committed_success run_executor
  [[ -e "$CASE_DIR/records/malformed-committed.partial" ]] \
    || fail 'malformed committed-success partial was deleted'
  ! grep -q 'pull sub2api-blue' "$EVENT_LOG" \
    || fail 'malformed committed-success partial permitted a new release'
}

test_review_paused_lock_creator_is_never_reclaimed() {
  local first_pid second_status attempts=0
  setup_case paused_lock_creator
  write_meminfo
  (
    run_executor \
      FAKE_PAUSE_AFTER_LOCK_MKDIR=1 \
      FAKE_LOCK_DIR="$CASE_DIR/records/.blue-green.lock" \
      FAKE_LOCK_CREATED_FILE="$CASE_DIR/lock-created" \
      FAKE_LOCK_RELEASE_FILE="$CASE_DIR/lock-release"
  ) >"$CASE_DIR/first.stdout" 2>"$CASE_DIR/first.stderr" &
  first_pid=$!
  while [[ ! -e "$CASE_DIR/lock-created" && "$attempts" -lt 10 ]]; do
    /bin/sleep 1
    attempts=$((attempts + 1))
  done
  [[ -e "$CASE_DIR/lock-created" ]] || fail 'first deployer did not pause after acquiring lock directory'

  if run_executor LOCK_OWNER_GRACE_SECONDS=0 FAKE_EPOCH=1785539999 >"$CASE_DIR/stdout" 2>"$CASE_DIR/stderr"; then
    second_status=0
  else
    second_status=$?
  fi
  [[ "$second_status" -ne 0 ]] || {
    : >"$CASE_DIR/lock-release"
    wait "$first_pid" || true
    fail 'second deployer reclaimed a live ownerless lock'
  }
  ! grep -Eq 'docker .*compose .* up |caddy caddy reload' "$EVENT_LOG" \
    || fail 'blocked second deployer mutated Docker or Caddy'

  : >"$CASE_DIR/lock-release"
  wait "$first_pid" || fail "first deployer did not retain its lock: $(cat "$CASE_DIR/first.stderr")"
}

test_review_concurrent_dead_pid_observers_fail_closed() {
  local stale_pid=2147483647 first_pid second_pid first_status second_status attempts=0
  setup_case dead_pid_lock_race
  write_meminfo
  mkdir "$CASE_DIR/records/.blue-green.lock"
  printf '%s\n' "$stale_pid" >"$CASE_DIR/records/.blue-green.lock/owner.pid"
  chmod 0600 "$CASE_DIR/records/.blue-green.lock/owner.pid"

  (
    run_executor \
      FAKE_PAUSE_AFTER_DEAD_PID_KILL=1 \
      FAKE_DEAD_PID="$stale_pid" \
      FAKE_DEAD_PID_READY_FILE="$CASE_DIR/first-kill-ready" \
      FAKE_DEAD_PID_RELEASE_FILE="$CASE_DIR/first-kill-release"
  ) >"$CASE_DIR/first.stdout" 2>"$CASE_DIR/first.stderr" &
  first_pid=$!

  (
    run_executor \
      FAKE_PAUSE_AFTER_DEAD_PID_KILL=1 \
      FAKE_DEAD_PID="$stale_pid" \
      FAKE_DEAD_PID_READY_FILE="$CASE_DIR/second-kill-ready" \
      FAKE_DEAD_PID_RELEASE_FILE="$CASE_DIR/second-kill-release"
  ) >"$CASE_DIR/second.stdout" 2>"$CASE_DIR/second.stderr" &
  second_pid=$!

  while [[ (! -e "$CASE_DIR/first-kill-ready" || ! -e "$CASE_DIR/second-kill-ready") && "$attempts" -lt 10 ]]; do
    /bin/sleep 1
    attempts=$((attempts + 1))
  done
  if [[ ! -e "$CASE_DIR/first-kill-ready" || ! -e "$CASE_DIR/second-kill-ready" ]]; then
    : >"$CASE_DIR/first-kill-release"
    : >"$CASE_DIR/second-kill-release"
    wait "$first_pid" || true
    wait "$second_pid" || true
    fail 'both contenders did not pause after observing the same dead PID owner'
  fi
  [[ "$(cat "$CASE_DIR/first-kill-ready")" == "$stale_pid" \
      && "$(cat "$CASE_DIR/second-kill-ready")" == "$stale_pid" ]] \
    || fail 'dead PID observation markers did not identify the stale owner'
  kill -0 "$first_pid" 2>/dev/null && kill -0 "$second_pid" 2>/dev/null \
    || fail 'both contenders were not simultaneously paused after the dead PID observation'

  : >"$CASE_DIR/first-kill-release"
  : >"$CASE_DIR/second-kill-release"

  if wait "$first_pid"; then first_status=0; else first_status=$?; fi
  if wait "$second_pid"; then second_status=0; else second_status=$?; fi
  [[ "$first_status" -ne 0 && "$second_status" -ne 0 ]] \
    || fail 'dead PID lock reclaim allowed a concurrent deployer to continue'
  [[ -d "$CASE_DIR/records/.blue-green.lock" \
      && -f "$CASE_DIR/records/.blue-green.lock/owner.pid" \
      && "$(cat "$CASE_DIR/records/.blue-green.lock/owner.pid")" == "$stale_pid" ]] \
    || fail 'concurrent dead PID observers changed the protected stale lock'
  ! grep -Eq 'docker .*compose .* up |caddy caddy reload' "$EVENT_LOG" \
    || fail 'dead PID lock reclaimer mutated Docker or Caddy'
}

case "${ONLY_TEST:-all}" in
  all)
    assert_rehearsal_topology_ready
    test_validation_failures
    printf 'PASS: fail-closed validation harness\n'
    test_preloaded_transport_loads_archive_without_pull
    printf 'PASS: preloaded archive transport\n'
    test_downtime_gates
    printf 'PASS: downtime gates precede mutation\n'
    test_authorized_maintenance_transition
    test_caddy_reconciliation_route
    printf 'PASS: authorized maintenance transition and Caddy reconciliation route\n'
    test_success_order_and_atomic_records
    test_worker_request_failure_log_does_not_trigger_startup_failure
    printf 'PASS: successful blue-green command order\n'
    test_two_slot_rehearsal_cycles
    test_failures_and_recovery
    printf 'PASS: rollback and interruption recovery\n'
    test_review_network_probe_image_policy
    printf 'PASS: immutable network probe image policy\n'
    test_review_worker_health_wait
    printf 'PASS: worker health wait\n'
    test_review_recovery_and_cleanup
    printf 'PASS: recovery checkpoints and credential cleanup\n'
    test_review_paused_lock_creator_is_never_reclaimed
    printf 'PASS: ownerless lock fail-closed concurrency\n'
    test_review_concurrent_dead_pid_observers_fail_closed
    printf 'PASS: stale PID lock fail-closed concurrency\n'
		test_final_review_rehearsal_isolation
		test_final_review_candidate_readiness
		test_final_review_runtime_singletons
		test_final_review_recovery_precedes_new_image
		test_preloaded_partial_recovery_precedes_probe_image_check
		test_final_review_rollback_proof
		test_final_review_live_route_mismatch
		test_final_review_restart_stable_route
			test_final_review_host_deadline
			printf 'PASS: final-review host safety regressions\n'
			test_schema_v1_interrupted_partial_recovers_with_full_image_ids
			test_legacy_partial_recovery_writes_complete_schema_v2_image_ids
			test_release_tag_image_ids_must_match_tags
			test_digest_image_ids_must_match_local_images
			printf 'PASS: immutable release-tag state and recovery IDs\n'
    ;;
  network) test_review_network_probe_image_policy ;;
  worker) test_review_worker_health_wait ;;
  recovery) test_review_recovery_and_cleanup ;;
  lock)
    test_review_paused_lock_creator_is_never_reclaimed
    test_review_concurrent_dead_pid_observers_fail_closed
    ;;
	final-review)
		test_final_review_rehearsal_isolation
		test_final_review_candidate_readiness
		test_final_review_runtime_singletons
		test_final_review_recovery_precedes_new_image
		test_preloaded_partial_recovery_precedes_probe_image_check
		test_final_review_rollback_proof
		test_final_review_live_route_mismatch
		test_final_review_restart_stable_route
			test_final_review_host_deadline
			test_release_tag_partial_recovery
			test_schema_v1_interrupted_partial_recovers_with_full_image_ids
			test_legacy_partial_recovery_writes_complete_schema_v2_image_ids
			test_release_tag_image_ids_must_match_tags
			test_digest_image_ids_must_match_local_images
			;;
	  partial)
			test_release_tag_partial_recovery
			test_schema_v1_interrupted_partial_recovers_with_full_image_ids
			test_legacy_partial_recovery_writes_complete_schema_v2_image_ids
			test_release_tag_image_ids_must_match_tags
			test_digest_image_ids_must_match_local_images
			;;
  preloaded-partial) test_preloaded_partial_recovery_precedes_probe_image_check ;;
  success) test_success_order_and_atomic_records ;;
  worker-request-log) test_worker_request_failure_log_does_not_trigger_startup_failure ;;
  maintenance)
		test_authorized_maintenance_transition
		test_caddy_reconciliation_route
		;;
	preloaded) test_preloaded_transport_loads_archive_without_pull ;;
  *) fail "unknown ONLY_TEST: ${ONLY_TEST}" ;;
esac
