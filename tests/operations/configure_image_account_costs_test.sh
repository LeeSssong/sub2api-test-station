#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT INT TERM
mkdir -p "$fixture/bin" "$fixture/project"
touch "$fixture/secret.env" "$fixture/release.env" "$fixture/compose.yaml" "$fixture/overlay.yaml" "$fixture/invocations"

cat > "$fixture/bin/docker" <<'SH'
#!/usr/bin/env bash
set -euo pipefail
fixture=${IMAGE_COST_TEST_FIXTURE:?}
printf '%s\n' "$*" >> "$fixture/invocations"
count=$(wc -l < "$fixture/invocations" | tr -d ' ')
cat > "$fixture/sql.$count"
SH
chmod +x "$fixture/bin/docker"

context=(
  "PATH=$fixture/bin:$PATH"
  "IMAGE_COST_TEST_FIXTURE=$fixture"
  SUB2API_COMPOSE_PROJECT=sub2api-deploy
  "SUB2API_PROJECT_DIRECTORY=$fixture/project"
  "SUB2API_SECRET_ENV_FILE=$fixture/secret.env"
  "SUB2API_RELEASE_ENV_FILE=$fixture/release.env"
  "SUB2API_COMPOSE_FILE=$fixture/compose.yaml"
  "SUB2API_IMAGE_OVERLAY=$fixture/overlay.yaml"
)

run_script() {
  env "${context[@]}" bash "$root/ops/configure-image-account-costs.sh" "$@"
}

run_production_project_script() {
  env "${context[@]}" SUB2API_COMPOSE_PROJECT=sub2api bash "$root/ops/configure-image-account-costs.sh" "$@"
}

check_output=$(run_script)
rg -Fq 'BEGIN READ ONLY;' "$fixture/sql.1"
rg -Fq "FROM groups WHERE name = '生图'" "$fixture/sql.1"
! rg -n 'INSERT INTO|UPDATE |DELETE FROM|SERIALIZABLE' "$fixture/sql.1"

apply_output=$(run_script --apply)
run_script --apply >/dev/null
rg -Fq 'BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;' "$fixture/sql.2"
rg -Fq 'LOCK TABLE groups, channels, channel_groups, channel_model_pricing' "$fixture/sql.2"
rg -Fq "'生图固定上游成本'" "$fixture/sql.2"
rg -Fq "'生图分组图片固定成本'" "$fixture/sql.2"
rg -Fq "'[\"gpt-image-*\"]'::jsonb" "$fixture/sql.2"
rg -Fq "('1K', 0.06::numeric, 0)" "$fixture/sql.2"
rg -Fq "('2K', 0.08::numeric, 1)" "$fixture/sql.2"
rg -Fq "('4K', 0.10::numeric, 2)" "$fixture/sql.2"
rg -Fq 'apply_pricing_to_account_stats' "$fixture/sql.2"
rg -Fq 'channel_model_pricing' "$fixture/sql.2"
cmp "$fixture/sql.2" "$fixture/sql.3"

run_production_project_script >/dev/null
rg -Fq 'BEGIN READ ONLY;' "$fixture/sql.4"

! rg -n -i 'UPDATE[[:space:]]+usage_logs|DELETE[[:space:]]+FROM[[:space:]]+usage_logs|rate_multiplier[[:space:]]*=' "$fixture/sql.2"
! rg -n '\.github/workflows|workflow_dispatch|schedule:' "$root/ops/configure-image-account-costs.sh"
! rg -n -i 'password|secret.env' <<< "$check_output$apply_output"

if run_script --unknown >/dev/null 2>&1; then
  printf 'FAIL: unknown mode accepted\n' >&2
  exit 1
fi

printf 'configure_image_account_costs_test: PASS\n'
