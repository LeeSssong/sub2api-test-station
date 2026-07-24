#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
fixture=$(mktemp -d)

cleanup() {
  rm -rf "$fixture"
}
trap cleanup EXIT INT TERM

mkdir -p "$fixture/bin" "$fixture/project"
: > "$fixture/compose.yaml"
: > "$fixture/compose.image.yaml"
printf '%s\n' 'POSTGRES_PASSWORD=test-postgres-password' > "$fixture/secret.env"
: > "$fixture/release.env"
: > "$fixture/docker-invocations"
printf '%s\n' '{"settings":{"payment_enabled":true,"registration_enabled":false,"affiliate_enabled":true,"custom_menu_items":"preserve"}}' > "$fixture/db.json"

cat > "$fixture/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

fixture=${PUBLIC_LINKS_TEST_FIXTURE:?}
project=${SUB2API_COMPOSE_PROJECT:?}
[[ "$project" == 'sub2api' || "$project" == 'sub2api-deploy' || "$project" == 'sub2api-official-rehearsal' ]] || exit 89
expected="compose --project-name $project --project-directory $fixture/project --env-file $fixture/secret.env --env-file $fixture/release.env -f $fixture/compose.yaml -f $fixture/compose.image.yaml exec -T postgres sh -c exec psql -v ON_ERROR_STOP=1 -v VERBOSITY=verbose -U \"\$POSTGRES_USER\" -d \"\$POSTGRES_DB\""
[[ "$*" == "$expected" ]] || {
  printf 'unexpected Compose identity: %s\n' "$*" >&2
  exit 90
}
printf '%s\n' "$*" >> "$fixture/docker-invocations"
invocation=$(wc -l < "$fixture/docker-invocations" | tr -d ' ')
cat > "$fixture/sql.$invocation"
printf '%s\n' 'BEGIN' 'SET' ' pg_advisory_xact_lock' 'INSERT 0 2' 'COMMIT'
jq '.settings.doc_url = "https://api.xingqiaolab.top/docs/" | .settings.balance_low_notify_recharge_url = "https://api.xingqiaolab.top/custom/xingqiao-storefront"' "$fixture/db.json" > "$fixture/db.next.json"
mv "$fixture/db.next.json" "$fixture/db.json"
EOF
chmod +x "$fixture/bin/docker"

public_links_context=(
  "PATH=$fixture/bin:$PATH"
  "PUBLIC_LINKS_TEST_FIXTURE=$fixture"
  'SUB2API_COMPOSE_PROJECT=sub2api-deploy'
  "SUB2API_PROJECT_DIRECTORY=$fixture/project"
  "SUB2API_SECRET_ENV_FILE=$fixture/secret.env"
  "SUB2API_RELEASE_ENV_FILE=$fixture/release.env"
  "SUB2API_COMPOSE_FILE=$fixture/compose.yaml"
  "SUB2API_IMAGE_OVERLAY=$fixture/compose.image.yaml"
)

run_configure() {
  env "${public_links_context[@]}" bash "$ROOT/ops/configure-sub2api-public-links.sh"
}

output=$(run_configure)
[[ "$output" == 'Configured Sub2API public links' ]]
[[ $(wc -l < "$fixture/docker-invocations" | tr -d ' ') == 1 ]]
rg -Fq 'BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;' "$fixture/sql.1"
rg -Fq '\set VERBOSITY verbose' "$fixture/sql.1"
rg -Fq "SET LOCAL application_name = 'configure-sub2api-public-links';" "$fixture/sql.1"
rg -Fq "pg_advisory_xact_lock(hashtext('sub2api:settings:public-links'))" "$fixture/sql.1"
rg -Fq "'doc_url', 'https://api.xingqiaolab.top/docs/'" "$fixture/sql.1"
rg -Fq "'balance_low_notify_recharge_url', 'https://api.xingqiaolab.top/custom/xingqiao-storefront'" "$fixture/sql.1"
rg -Fq 'ON CONFLICT (key) DO UPDATE' "$fixture/sql.1"
! rg -n "custom_menu_items|payment_enabled|registration_enabled|affiliate_enabled" "$fixture/sql.1"
! rg -n -i 'test-postgres-password|settings.*response' <<< "$output"
jq -e '.settings.doc_url == "https://api.xingqiaolab.top/docs/"' "$fixture/db.json" >/dev/null
jq -e '.settings.balance_low_notify_recharge_url == "https://api.xingqiaolab.top/custom/xingqiao-storefront"' "$fixture/db.json" >/dev/null
jq -e '.settings.payment_enabled == true and .settings.registration_enabled == false and .settings.affiliate_enabled == true and .settings.custom_menu_items == "preserve"' "$fixture/db.json" >/dev/null

run_configure >/dev/null
cmp "$fixture/sql.1" "$fixture/sql.2"

env "${public_links_context[@]}" SUB2API_COMPOSE_PROJECT=sub2api-official-rehearsal \
  bash "$ROOT/ops/configure-sub2api-public-links.sh" >/dev/null
rg -Fq -- 'compose --project-name sub2api-official-rehearsal' "$fixture/docker-invocations"

required_context=(
  SUB2API_COMPOSE_PROJECT
  SUB2API_PROJECT_DIRECTORY
  SUB2API_SECRET_ENV_FILE
  SUB2API_RELEASE_ENV_FILE
  SUB2API_COMPOSE_FILE
  SUB2API_IMAGE_OVERLAY
)
for missing in "${required_context[@]}"; do
  reduced_context=()
  for assignment in "${public_links_context[@]}"; do
    [[ "$assignment" == "$missing="* ]] || reduced_context+=("$assignment")
  done
  before=$(wc -l < "$fixture/docker-invocations" | tr -d ' ')
  if env -u "$missing" "${reduced_context[@]}" bash "$ROOT/ops/configure-sub2api-public-links.sh" >/dev/null 2>&1; then
    printf 'FAIL: accepted missing deployment context: %s\n' "$missing" >&2
    exit 1
  fi
  after=$(wc -l < "$fixture/docker-invocations" | tr -d ' ')
  [[ "$before" == "$after" ]]
done

if env "${public_links_context[@]}" SUB2API_COMPOSE_PROJECT=not-production bash "$ROOT/ops/configure-sub2api-public-links.sh" >/dev/null 2>&1; then
  printf 'FAIL: accepted changed Compose project identity\n' >&2
  exit 1
fi

assert_context_path_rejected() {
  local variable=$1 value=$2 label=$3 before after
  before=$(wc -l < "$fixture/docker-invocations" | tr -d ' ')
  if env "${public_links_context[@]}" "$variable=$value" bash "$ROOT/ops/configure-sub2api-public-links.sh" >/dev/null 2>&1; then
    printf 'FAIL: accepted invalid deployment context path: %s\n' "$label" >&2
    exit 1
  fi
  after=$(wc -l < "$fixture/docker-invocations" | tr -d ' ')
  [[ "$before" == "$after" ]]
}

ln -s "$fixture/secret.env" "$fixture/symlink-secret.env"
ln -s "$fixture/release.env" "$fixture/symlink-release.env"
ln -s "$fixture/compose.yaml" "$fixture/symlink-compose.yaml"
ln -s "$fixture/compose.image.yaml" "$fixture/symlink-overlay.yaml"
ln -s "$fixture/project" "$fixture/symlink-project"
assert_context_path_rejected SUB2API_SECRET_ENV_FILE "$fixture/symlink-secret.env" 'secret env symlink'
assert_context_path_rejected SUB2API_RELEASE_ENV_FILE "$fixture/symlink-release.env" 'release env symlink'
assert_context_path_rejected SUB2API_COMPOSE_FILE "$fixture/symlink-compose.yaml" 'base Compose symlink'
assert_context_path_rejected SUB2API_IMAGE_OVERLAY "$fixture/symlink-overlay.yaml" 'image overlay symlink'
assert_context_path_rejected SUB2API_PROJECT_DIRECTORY "$fixture/symlink-project" 'project directory symlink'

: > "$fixture/changed-release.env"
assert_context_path_rejected SUB2API_RELEASE_ENV_FILE "$fixture/changed-release.env" 'changed release identity'

run_retry_exhaustion_test() {
  local sqlstate=$1
  mkdir -p "$fixture/retry-bin-$sqlstate"
  cat > "$fixture/retry-bin-$sqlstate/docker" <<EOF
#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
printf 'attempt\\n' >> "$fixture/retry-attempts-$sqlstate"
printf 'ERROR:  $sqlstate: retryable database failure\\n' >&2
exit 3
EOF
  chmod +x "$fixture/retry-bin-$sqlstate/docker"
  : > "$fixture/retry-attempts-$sqlstate"
  if env "${public_links_context[@]}" PATH="$fixture/retry-bin-$sqlstate:$PATH" \
      bash "$ROOT/ops/configure-sub2api-public-links.sh" > "$fixture/retry-output-$sqlstate" 2>&1; then
    printf 'FAIL: %s retry exhaustion reported success\n' "$sqlstate" >&2
    exit 1
  fi
  [[ $(wc -l < "$fixture/retry-attempts-$sqlstate" | tr -d ' ') == 5 ]]
  ! rg -n -i 'test-postgres-password|settings.*response|Configured Sub2API public links' "$fixture/retry-output-$sqlstate"
}

run_retry_exhaustion_test 40001
run_retry_exhaustion_test 40P01

printf 'PASS: public links transaction and deployment identity contracts\n'
