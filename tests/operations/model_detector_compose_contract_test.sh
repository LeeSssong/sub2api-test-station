#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
COMPOSE="$ROOT/infra/compose.yaml"
fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

[[ -f "$COMPOSE" ]] || fail "compose file missing"
grep -Fq 'SUB2API_MODEL_DETECTOR_URL:' "$COMPOSE" || fail "detector URL is not wired"
grep -Fq 'SUB2API_MODEL_DETECTOR_TOKEN:' "$COMPOSE" || fail "detector token is not wired"
grep -Fq 'model-detector:' "$COMPOSE" || fail "detector service is not declared"
grep -Fq '/app/model-detector' "$COMPOSE" || fail "detector service command is not pinned"
grep -Fq 'http://model-detector:8090' "$COMPOSE" || fail "detector private URL is not wired"
[[ $(grep -Fc '<<: *sub2api-environment' "$COMPOSE") -eq 3 ]] || fail "blue, green, and worker do not share detector environment"
if grep -Eiq 'ports:.*detector|detector.*ports:' "$COMPOSE"; then
  fail "detector port must not be published by Sub compose"
fi
if grep -Fq 'MODEL_DETECTOR_TOKEN: ' "$COMPOSE" && grep -Eq 'MODEL_DETECTOR_TOKEN: [^$]' "$COMPOSE"; then
  fail "detector token must come from operator environment"
fi
printf 'model detector compose contract passed\n'
