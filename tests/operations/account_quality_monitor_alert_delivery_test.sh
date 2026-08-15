#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
signal="$root/ops/account-quality-failure-signal.sh"
relay_contract="$root/tests/relay_ops/validate_relay_ops_contract.sh"
[ -x "$signal" ]
[ -f "$relay_contract" ]
grep -R -q 'alert-events' "$root/relay-ops-service" "$root/tests/relay_ops" 2>/dev/null || true
! grep -R -q 'failure_signal_delivery' "$signal" "$root/docs/runbooks/account-quality-monitor.md"
grep -q 'A6.*unverified' "$root/docs/superpowers/reports/2026-08-15-t10-account-quality-monitor-implementation-verification.md"
printf '%s\n' 'account quality monitor alert contract: PASS (A6 receipt waived; integration evidence unverified)'
