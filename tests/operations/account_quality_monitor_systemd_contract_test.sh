#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
service="$root/infra/systemd/sub2api-account-quality-monitor.service"
failure="$root/infra/systemd/sub2api-account-quality-monitor-failure.service"
timer="$root/infra/systemd/sub2api-account-quality-monitor.timer"
awk 'BEGIN{s="Unit"} /^\[/ {s=substr($0,2,length($0)-2)} /^OnFailure=/ {if (s != "Unit") exit 1}' "$service"
grep -q '^User=root$' "$service"
grep -q '^Group=root$' "$service"
grep -q '^OnFailure=sub2api-account-quality-monitor-failure.service$' "$service"
grep -q '^ExecStart=/opt/sub2api/production/ops/account-quality/run-account-quality-monitor.sh$' "$service"
grep -q '^ExecStart=/opt/sub2api/production/ops/account-quality/account-quality-failure-signal.sh$' "$failure"
grep -q '^OnUnitActiveSec=15m$' "$timer"
grep -q '^RandomizedDelaySec=2m$' "$timer"
grep -q '^Persistent=true$' "$timer"
printf '%s\n' 'account quality monitor systemd contract: PASS'
