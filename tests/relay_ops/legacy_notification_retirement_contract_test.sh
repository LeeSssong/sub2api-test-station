#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
RELAY_ROOT="$ROOT/relay-ops-service"

dependencies=$(
  cd "$RELAY_ROOT"
  go list -deps ./cmd/relay-ops ./cmd/provision-billing-source
)

if rg -Fxq 'example.invalid/relay-ops-service/internal/legacyretirement' <<<"$dependencies"; then
  printf 'cleanup package is reachable from relay-ops runtime or provision command\n' >&2
  exit 1
fi

retired_packages=(
  acceptance
  agent
  alerting
  dailyreport
  feishuapi
  groupimpact
  incidents
  nativealerts
  notificationpolicy
  notify
  opsmonitor
  pricingevents
)

for package in "${retired_packages[@]}"; do
  if rg -Fxq "example.invalid/relay-ops-service/internal/$package" <<<"$dependencies"; then
    printf 'retired notification package remains reachable: %s\n' "$package" >&2
    exit 1
  fi
done

[[ -f "$RELAY_ROOT/cmd/retire-legacy-notifications/main.go" ]] || {
  printf 'controlled retirement command is missing\n' >&2
  exit 1
}

if [[ -d "$RELAY_ROOT/cmd/release-prep-notify" ]]; then
  printf 'retired release notification command remains present\n' >&2
  exit 1
fi

for compose_file in infra/compose.yaml; do
  if rg -q 'RELAY_OPS_FEISHU_|RELAY_OPS_NOTIFICATION_POLICY_|/run/secrets/feishu-|notification-policy.json' "$ROOT/$compose_file"; then
    printf 'relay-ops still receives retired notification configuration in %s\n' "$compose_file" >&2
    exit 1
  fi
done
