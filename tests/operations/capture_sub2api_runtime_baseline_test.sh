#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
SCRIPT="$ROOT/ops/capture-sub2api-runtime-baseline.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

new_fixture() {
  FIXTURE_ROOT=$(mktemp -d)
  FIXTURE_BIN="$FIXTURE_ROOT/bin"
  FIXTURE_DEPLOY="$FIXTURE_ROOT/deploy"
  mkdir -p "$FIXTURE_BIN" "$FIXTURE_DEPLOY/data" "$FIXTURE_DEPLOY/postgres_data" "$FIXTURE_DEPLOY/redis_data"

  cat >"$FIXTURE_BIN/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail

[[ "${1:-}" == inspect ]] || exit 64

case "${2:-}" in
  sub2api)
    cat <<JSON
[{"Config":{"Image":"xingqiao-sub2api:v0.1.164-contact-v1","Env":["POSTGRES_PASSWORD=do-not-print","JWT_SECRET=do-not-print"],"Labels":{"com.docker.compose.project":"${FAKE_PROJECT:-sub2api-deploy}","com.docker.compose.project.config_files":"$FIXTURE_DEPLOY/docker-compose.yml","com.docker.compose.project.working_dir":"$FIXTURE_DEPLOY"}},"Image":"${FAKE_IMAGE_ID:-sha256:939e6f88068e82fd65f212bcc7b28b9ef2a9af27b8cce64e0b819a8b65fc3220}","Mounts":[{"Type":"bind","Source":"${FAKE_SUB2API_SOURCE:-$FIXTURE_DEPLOY/data}","Destination":"/app/data","RW":${FAKE_SUB2API_RW:-true}}],"NetworkSettings":{"Networks":{"sub2api-deploy_sub2api-network":{}}}}]
JSON
    ;;
  sub2api-postgres)
    cat <<JSON
[{"Config":{"Labels":{"com.docker.compose.project":"${FAKE_PROJECT:-sub2api-deploy}"}},"Mounts":[{"Type":"volume","Source":"anonymous-postgres-volume","Destination":"/var/lib/postgresql","RW":true},{"Type":"bind","Source":"${FAKE_POSTGRES_SOURCE:-$FIXTURE_DEPLOY/postgres_data}","Destination":"/var/lib/postgresql/data","RW":${FAKE_POSTGRES_RW:-true}}],"NetworkSettings":{"Networks":{"sub2api-deploy_sub2api-network":{}}}}]
JSON
    ;;
  sub2api-redis)
    cat <<JSON
[{"Config":{"Labels":{"com.docker.compose.project":"${FAKE_PROJECT:-sub2api-deploy}"}},"Mounts":[{"Type":"bind","Source":"${FAKE_REDIS_SOURCE:-$FIXTURE_DEPLOY/redis_data}","Destination":"/data","RW":${FAKE_REDIS_RW:-true}}],"NetworkSettings":{"Networks":{"sub2api-deploy_sub2api-network":{}}}}]
JSON
    ;;
  *) exit 65 ;;
esac
SH
  chmod 0755 "$FIXTURE_BIN/docker"
}

cleanup_fixture() {
  rm -rf -- "$FIXTURE_ROOT"
}

run_collector() {
  env \
    PATH="$FIXTURE_BIN:$PATH" \
    FIXTURE_DEPLOY="$FIXTURE_DEPLOY" \
    EXPECTED_PROJECT=sub2api-deploy \
    EXPECTED_SUB2API_DATA="${EXPECTED_SUB2API_DATA:-$FIXTURE_DEPLOY/data}" \
    EXPECTED_POSTGRES_DATA="$FIXTURE_DEPLOY/postgres_data" \
    EXPECTED_REDIS_DATA="$FIXTURE_DEPLOY/redis_data" \
    "$SCRIPT"
}

test_accepts_authoritative_production_binds_only() {
  new_fixture
  run_collector >"$FIXTURE_ROOT/baseline.json"

  jq -e '
    .project == "sub2api-deploy" and
    .image == "xingqiao-sub2api:v0.1.164-contact-v1" and
    .mounts.sub2api.destination == "/app/data" and
    .mounts.postgres.destination == "/var/lib/postgresql/data" and
    .mounts.redis.destination == "/data" and
    .network_names == ["sub2api-deploy_sub2api-network"] and
    (.captured_at | type == "string")
  ' "$FIXTURE_ROOT/baseline.json" >/dev/null || fail 'baseline JSON omitted an approved runtime fact'
  ! rg -n 'do-not-print|POSTGRES_PASSWORD|JWT_SECRET' "$FIXTURE_ROOT/baseline.json" \
    || fail 'baseline JSON exposed environment data'

  mkdir "$FIXTURE_DEPLOY/wrong"
  if EXPECTED_SUB2API_DATA="$FIXTURE_DEPLOY/wrong" run_collector \
    >/dev/null 2>"$FIXTURE_ROOT/wrong-bind.stderr"; then
    fail 'wrong application-data bind was accepted'
  fi
  rg -Fq 'unexpected source at /app/data' "$FIXTURE_ROOT/wrong-bind.stderr" \
    || fail 'wrong application-data bind did not reach source mismatch validation'
  cleanup_fixture
}

test_rejects_wrong_project_image_or_read_only_data() {
  new_fixture

  if FAKE_PROJECT=other-project run_collector >/dev/null 2>&1; then
    fail 'wrong Compose project was accepted'
  fi
  if FAKE_IMAGE_ID=sha256:not-the-approved-image run_collector >/dev/null 2>&1; then
    fail 'wrong Sub2API image identity was accepted'
  fi
  if FAKE_POSTGRES_RW=false run_collector >/dev/null 2>&1; then
    fail 'read-only PostgreSQL data bind was accepted'
  fi
  cleanup_fixture
}

test_accepts_authoritative_production_binds_only
test_rejects_wrong_project_image_or_read_only_data

printf 'PASS: Sub2API runtime baseline capture contracts\n'
