#!/usr/bin/env bash
set -euo pipefail

mode=check
case "${1:-}" in
  "") ;;
  --apply) mode=apply ;;
  *) printf 'Usage: %s [--apply]\n' "${0##*/}" >&2; exit 2 ;;
esac

: "${SUB2API_COMPOSE_PROJECT:?Set SUB2API_COMPOSE_PROJECT}"
: "${SUB2API_PROJECT_DIRECTORY:?Set SUB2API_PROJECT_DIRECTORY}"
: "${SUB2API_SECRET_ENV_FILE:?Set SUB2API_SECRET_ENV_FILE}"
: "${SUB2API_RELEASE_ENV_FILE:?Set SUB2API_RELEASE_ENV_FILE}"
: "${SUB2API_COMPOSE_FILE:?Set SUB2API_COMPOSE_FILE}"
: "${SUB2API_IMAGE_OVERLAY:?Set SUB2API_IMAGE_OVERLAY}"

case "$SUB2API_COMPOSE_PROJECT" in
  sub2api-deploy|sub2api-official-rehearsal) ;;
  *) printf 'Unsupported Compose project identity\n' >&2; exit 1 ;;
esac
for path in "$SUB2API_SECRET_ENV_FILE" "$SUB2API_RELEASE_ENV_FILE" "$SUB2API_COMPOSE_FILE" "$SUB2API_IMAGE_OVERLAY"; do
  [[ -f "$path" && -r "$path" && ! -L "$path" ]] || { printf 'Invalid deployment file: %s\n' "$path" >&2; exit 1; }
done
[[ -d "$SUB2API_PROJECT_DIRECTORY" && ! -L "$SUB2API_PROJECT_DIRECTORY" ]] || { printf 'Invalid project directory\n' >&2; exit 1; }
command -v docker >/dev/null || { printf 'docker is required\n' >&2; exit 1; }

compose=(docker compose
  --project-name "$SUB2API_COMPOSE_PROJECT"
  --project-directory "$SUB2API_PROJECT_DIRECTORY"
  --env-file "$SUB2API_SECRET_ENV_FILE"
  --env-file "$SUB2API_RELEASE_ENV_FILE"
  -f "$SUB2API_COMPOSE_FILE"
  -f "$SUB2API_IMAGE_OVERLAY")

run_psql() {
  "${compose[@]}" exec -T postgres \
    sh -c 'exec psql -v ON_ERROR_STOP=1 -v VERBOSITY=verbose -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
}

if [[ "$mode" == apply ]]; then
  run_psql <<'SQL'
\set VERBOSITY verbose
BEGIN TRANSACTION ISOLATION LEVEL SERIALIZABLE;
SET LOCAL application_name = 'configure-image-account-costs';
LOCK TABLE groups, channels, channel_groups, channel_model_pricing,
  channel_account_stats_pricing_rules, channel_account_stats_model_pricing,
  channel_account_stats_pricing_intervals IN SHARE ROW EXCLUSIVE MODE;

DO $guard$
DECLARE
  target_group_id bigint;
  target_channel_id bigint;
  target_rule_id bigint;
  target_pricing_id bigint;
BEGIN
  IF (SELECT count(*) FROM groups WHERE name = '生图') <> 1 THEN
    RAISE EXCEPTION 'expected exactly one group named 生图';
  END IF;
  SELECT id INTO target_group_id FROM groups WHERE name = '生图';
  SELECT id INTO target_channel_id FROM channels WHERE name = '生图固定上游成本';

  IF target_channel_id IS NOT NULL THEN
    IF EXISTS (SELECT 1 FROM channels WHERE id = target_channel_id AND (status <> 'active' OR apply_pricing_to_account_stats)) THEN
      RAISE EXCEPTION 'incompatible fixed image cost channel';
    END IF;
    IF EXISTS (SELECT 1 FROM channel_model_pricing WHERE channel_id = target_channel_id) THEN
      RAISE EXCEPTION 'fixed image cost channel must not contain customer model pricing';
    END IF;
    IF EXISTS (SELECT 1 FROM channel_groups WHERE channel_id = target_channel_id AND group_id <> target_group_id) THEN
      RAISE EXCEPTION 'fixed image cost channel has an unexpected group binding';
    END IF;
  END IF;
  IF EXISTS (
    SELECT 1 FROM channel_groups cg
    WHERE cg.group_id = target_group_id
      AND (target_channel_id IS NULL OR cg.channel_id <> target_channel_id)
  ) THEN
    RAISE EXCEPTION '生图 is already bound to a different channel';
  END IF;

  IF target_channel_id IS NOT NULL THEN
    IF (SELECT count(*) FROM channel_account_stats_pricing_rules WHERE channel_id = target_channel_id AND name = '生图分组图片固定成本') > 1 THEN
      RAISE EXCEPTION 'duplicate fixed image cost rules';
    END IF;
    SELECT id INTO target_rule_id FROM channel_account_stats_pricing_rules
      WHERE channel_id = target_channel_id AND name = '生图分组图片固定成本';
    IF target_rule_id IS NOT NULL AND EXISTS (
      SELECT 1 FROM channel_account_stats_pricing_rules
      WHERE id = target_rule_id AND (group_ids <> ARRAY[target_group_id]::bigint[] OR account_ids <> '{}'::bigint[] OR sort_order <> 0)
    ) THEN
      RAISE EXCEPTION 'incompatible fixed image cost rule';
    END IF;
    IF target_rule_id IS NOT NULL THEN
      IF (SELECT count(*) FROM channel_account_stats_model_pricing WHERE rule_id = target_rule_id) NOT IN (0, 1) THEN
        RAISE EXCEPTION 'incompatible fixed image cost pricing count';
      END IF;
      SELECT id INTO target_pricing_id FROM channel_account_stats_model_pricing WHERE rule_id = target_rule_id;
      IF target_pricing_id IS NOT NULL AND EXISTS (
        SELECT 1 FROM channel_account_stats_model_pricing
        WHERE id = target_pricing_id AND (
          platform <> 'openai' OR models <> '["gpt-image-*"]'::jsonb OR billing_mode <> 'image'
          OR input_price IS NOT NULL OR output_price IS NOT NULL OR cache_write_price IS NOT NULL
          OR cache_read_price IS NOT NULL OR image_output_price IS NOT NULL OR per_request_price IS NOT NULL
        )
      ) THEN
        RAISE EXCEPTION 'incompatible fixed image model pricing';
      END IF;
      IF target_pricing_id IS NOT NULL AND (SELECT count(*) FROM channel_account_stats_pricing_intervals WHERE pricing_id = target_pricing_id) NOT IN (0, 3) THEN
        RAISE EXCEPTION 'incompatible fixed image interval count';
      END IF;
      IF target_pricing_id IS NOT NULL AND EXISTS (SELECT 1 FROM channel_account_stats_pricing_intervals WHERE pricing_id = target_pricing_id) AND NOT (
        (SELECT count(*) FROM channel_account_stats_pricing_intervals WHERE pricing_id = target_pricing_id AND tier_label = '1K' AND per_request_price = 0.06) = 1 AND
        (SELECT count(*) FROM channel_account_stats_pricing_intervals WHERE pricing_id = target_pricing_id AND tier_label = '2K' AND per_request_price = 0.08) = 1 AND
        (SELECT count(*) FROM channel_account_stats_pricing_intervals WHERE pricing_id = target_pricing_id AND tier_label = '4K' AND per_request_price = 0.10) = 1
      ) THEN
        RAISE EXCEPTION 'incompatible fixed image interval prices';
      END IF;
    END IF;
  END IF;
END
$guard$;

INSERT INTO channels (name, description, status, apply_pricing_to_account_stats)
VALUES ('生图固定上游成本', '生图分组固定上游账号成本，仅用于账号统计', 'active', false)
ON CONFLICT (name) DO NOTHING;

INSERT INTO channel_groups (channel_id, group_id)
SELECT c.id, g.id FROM channels c CROSS JOIN groups g
WHERE c.name = '生图固定上游成本' AND g.name = '生图'
ON CONFLICT (group_id) DO NOTHING;

INSERT INTO channel_account_stats_pricing_rules (channel_id, name, group_ids, account_ids, sort_order)
SELECT c.id, '生图分组图片固定成本', ARRAY[g.id]::bigint[], '{}'::bigint[], 0
FROM channels c CROSS JOIN groups g
WHERE c.name = '生图固定上游成本' AND g.name = '生图'
  AND NOT EXISTS (SELECT 1 FROM channel_account_stats_pricing_rules r WHERE r.channel_id = c.id AND r.name = '生图分组图片固定成本');

INSERT INTO channel_account_stats_model_pricing (rule_id, platform, models, billing_mode)
SELECT r.id, 'openai', '["gpt-image-*"]'::jsonb, 'image'
FROM channel_account_stats_pricing_rules r JOIN channels c ON c.id = r.channel_id
WHERE c.name = '生图固定上游成本' AND r.name = '生图分组图片固定成本'
  AND NOT EXISTS (SELECT 1 FROM channel_account_stats_model_pricing p WHERE p.rule_id = r.id);

INSERT INTO channel_account_stats_pricing_intervals (pricing_id, tier_label, per_request_price, sort_order)
SELECT p.id, v.tier_label, v.price, v.sort_order
FROM channel_account_stats_model_pricing p
JOIN channel_account_stats_pricing_rules r ON r.id = p.rule_id
JOIN channels c ON c.id = r.channel_id
CROSS JOIN (VALUES ('1K', 0.06::numeric, 0), ('2K', 0.08::numeric, 1), ('4K', 0.10::numeric, 2)) v(tier_label, price, sort_order)
WHERE c.name = '生图固定上游成本' AND r.name = '生图分组图片固定成本'
  AND NOT EXISTS (SELECT 1 FROM channel_account_stats_pricing_intervals i WHERE i.pricing_id = p.id);
COMMIT;
SQL
else
  run_psql <<'SQL'
\set VERBOSITY verbose
BEGIN READ ONLY;
DO $check$
BEGIN
  IF (SELECT count(*) FROM groups WHERE name = '生图') <> 1 THEN
    RAISE EXCEPTION 'expected exactly one group named 生图';
  END IF;
END
$check$;
SELECT
  (SELECT count(*) FROM groups WHERE name = '生图') AS target_groups,
  (SELECT count(*) FROM channels WHERE name = '生图固定上游成本') AS fixed_channels,
  (SELECT count(*) FROM channel_account_stats_pricing_rules WHERE name = '生图分组图片固定成本') AS fixed_rules;
ROLLBACK;
SQL
fi

printf 'Image account cost configuration %s completed\n' "$mode"
