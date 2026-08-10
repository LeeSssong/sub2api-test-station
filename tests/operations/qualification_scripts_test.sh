#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

state="$TMP/qualification.json"
reporter="$TMP/reporter.sh"
executor="$TMP/executor.sh"
cat >"$reporter" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
cat <<JSON
{"tag":"$1","commit":"abc123","asset":"sub2api.tar","checksum":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","adapter_version":"adapter-1","contract_version":1,"migration_class":"expand_only","tests":["contract"],"data_diff":"zero","passed":true}
JSON
EOF
cat >"$executor" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'executor %s\n' "$*" >"${EXECUTOR_LOG:?}"
EOF
chmod 700 "$reporter" "$executor"

if RELEASE_STATE_FILE="$state" QUALIFICATION_COMMAND="$reporter" "$ROOT/ops/qualify-sub2api-official-release.sh" v0.1.173 >/dev/null; then
  test "$(jq -r .stage "$state")" = ready
else
  echo 'qualification should accept the complete fixture' >&2
  exit 1
fi

if RELEASE_STATE_FILE="$state" SUB2API_HOST_EXECUTOR="$executor" EXECUTOR_LOG="$TMP/executor.log" "$ROOT/ops/promote-sub2api-qualified-release.sh" >/dev/null; then
  grep -q -- '--qualification-state' "$TMP/executor.log"
else
  echo 'promotion should invoke the reviewed executor' >&2
  exit 1
fi

rm -f "$state"
if RELEASE_STATE_FILE="$state" env -u QUALIFICATION_COMMAND "$ROOT/ops/qualify-sub2api-official-release.sh" v0.1.173 >/dev/null 2>&1; then
  echo 'missing qualification evidence must fail closed' >&2
  exit 1
fi

echo 'PASS: qualification script contracts'
