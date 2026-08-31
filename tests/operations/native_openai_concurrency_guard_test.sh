#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
GUARD="$ROOT/ops/assert-native-openai-concurrency-only.sh"
FIXTURE=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-native-concurrency-guard.XXXXXX")
trap 'rm -rf -- "$FIXTURE"' EXIT

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }

mkdir -p "$FIXTURE/upstream/sub2api/backend/internal/service" "$FIXTURE/upstream/sub2api/backend/internal/handler"
printf 'module example.invalid/sub2api\n' >"$FIXTURE/upstream/sub2api/backend/go.mod"
cp "$ROOT/upstream/sub2api/backend/internal/service/openai_shared_health.go" \
  "$FIXTURE/upstream/sub2api/backend/internal/service/openai_shared_health.go"
cp "$ROOT/upstream/sub2api/backend/internal/handler/openai_chat_completions.go" \
  "$FIXTURE/upstream/sub2api/backend/internal/handler/openai_chat_completions.go"
cp "$ROOT/upstream/sub2api/backend/internal/handler/openai_gateway_handler.go" \
  "$FIXTURE/upstream/sub2api/backend/internal/handler/openai_gateway_handler.go"

"$GUARD" --worktree "$FIXTURE" >/dev/null || fail 'current native-only source was rejected'

sed -i.bak 's/h\.acquireResponsesAccountSlot(/h.service.AcquireOpenAIAdmission(/' \
  "$FIXTURE/upstream/sub2api/backend/internal/handler/openai_chat_completions.go"
rm -f "$FIXTURE/upstream/sub2api/backend/internal/handler/openai_chat_completions.go.bak"
if "$GUARD" --worktree "$FIXTURE" >/dev/null 2>&1; then
  fail 'custom admission handler call was accepted'
fi

cp "$ROOT/upstream/sub2api/backend/internal/handler/openai_chat_completions.go" \
  "$FIXTURE/upstream/sub2api/backend/internal/handler/openai_chat_completions.go"
sed -i.bak 's/return func() {}, OpenAISharedAdmissionDecision{Allowed: true, Reason: "disabled"}/return func() {}, OpenAISharedAdmissionDecision{Allowed: false, Reason: "custom"}/' \
  "$FIXTURE/upstream/sub2api/backend/internal/service/openai_shared_health.go"
rm -f "$FIXTURE/upstream/sub2api/backend/internal/service/openai_shared_health.go.bak"
if "$GUARD" --worktree "$FIXTURE" >/dev/null 2>&1; then
  fail 'restored admission rejection was accepted'
fi

printf 'PASS: native OpenAI account concurrency guard\n'
