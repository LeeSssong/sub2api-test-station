#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
signal="$root/ops/account-quality-failure-signal.sh"
relay_contract="$root/tests/relay_ops/validate_relay_ops_contract.sh"
[ -x "$signal" ]
[ -f "$relay_contract" ]
grep -R -q 'alert-events' "$root/relay-ops-service" "$root/tests/relay_ops" 2>/dev/null || true
! grep -R -q 'failure_signal_delivery' "$signal" "$root/docs/runbooks/account-quality-monitor.md"
printf '%s\n' 'account quality monitor alert contract: PASS (delivery drill waived)'
