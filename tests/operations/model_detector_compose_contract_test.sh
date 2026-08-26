#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
COMPOSE="$ROOT/infra/compose.yaml"
DOCKERFILE="$ROOT/upstream/sub2api/Dockerfile"
fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

[[ -f "$COMPOSE" ]] || fail "compose file missing"
[[ -f "$DOCKERFILE" ]] || fail "Dockerfile missing"
grep -Fq 'SUB2API_MODEL_DETECTOR_URL:' "$COMPOSE" || fail "detector URL is not wired"
grep -Fq 'SUB2API_MODEL_DETECTOR_TOKEN:' "$COMPOSE" || fail "detector token is not wired"
grep -Fq 'model-detector:' "$COMPOSE" || fail "detector service is not declared"
grep -Fq '/app/model-detector' "$COMPOSE" || fail "detector service command is not pinned"
grep -Fq 'http://model-detector:8090' "$COMPOSE" || fail "detector private URL is not wired"
grep -Fq 'MODEL_DETECTOR_VERSION: ${MODEL_DETECTOR_VERSION:-4.1.1}' "$COMPOSE" || fail "detector version must default to licensed v4.1.1"
grep -Fq 'GPT56_V411_RELEASE_SHA256=70c0c2f092e66cd219f2384e08872e5bedb4559e427c2e320d0070186376f865' "$DOCKERFILE" || fail "pinned v4.1.1 artifact checksum is missing"
grep -Fq 'model-detector-v411-adapter.py' "$DOCKERFILE" || fail "v4.1.1 adapter is not installed"
grep -Fq 'python3' "$DOCKERFILE" || fail "v4.1.1 Python runtime is missing"
grep -Fq 'nodejs' "$DOCKERFILE" || fail "v4.1.1 native transport runtime is missing"
[[ $(grep -Fc '<<: *sub2api-environment' "$COMPOSE") -eq 3 ]] || fail "blue, green, and worker do not share detector environment"
if grep -Eiq 'ports:.*detector|detector.*ports:' "$COMPOSE"; then
  fail "detector port must not be published by Sub compose"
fi
if grep -Fq 'MODEL_DETECTOR_TOKEN: ' "$COMPOSE" && grep -Eq 'MODEL_DETECTOR_TOKEN: [^$]' "$COMPOSE"; then
  fail "detector token must come from operator environment"
fi
[[ $(grep -Fc -- '--mount=type=cache,id=sub2api-gomod,target=/go/pkg/mod' "$DOCKERFILE") -eq 3 ]] \
  || fail "module download and both backend binaries must use the explicit Go module cache target"
[[ $(grep -Fc -- '--mount=type=cache,id=sub2api-gobuild,target=/root/.cache/go-build' "$DOCKERFILE") -eq 2 ]] \
  || fail "both backend binaries must use the explicit Go build cache target"
printf 'model detector compose contract passed\n'
