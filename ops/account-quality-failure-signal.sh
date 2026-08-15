#!/bin/sh
set -eu

phase=${T10_FAILURE_PHASE:-unknown}
reason=${T10_REASON_CODE:-unknown}
result=${SYSTEMD_RESULT:-unknown}
status=${SYSTEMD_EXEC_MAIN_STATUS:-unknown}
unit=${T10_UNIT_NAME:-sub2api-account-quality-monitor.service}
case "$phase" in systemd|preflight|evidence|credentials|runtime|collector|resource|publish) ;; *) phase=unknown ;; esac
case "$reason" in path_missing|path_mode|credential_invalid|docker_unavailable|mount_write|collector_failed|resource_limit|publish_failed|exec_failed|timeout|unknown) ;; *) reason=unknown ;; esac
case "$result" in [0-9]|[0-9][0-9]|[0-9][0-9][0-9]|success|failure|timeout|exit-code|signal|unknown) ;; *) result=unknown ;; esac
case "$status" in [0-9]|[0-9][0-9]|[0-9][0-9][0-9]|success|failure|timeout|exit-code|signal|unknown) ;; *) status=unknown ;; esac
safe_unit=$(printf '%s' "$unit" | sed 's/[^A-Za-z0-9_.@-]/_/g' | cut -c1-80)
dedupe=$(printf 't10.failure.v1|%s|%s|%s|%s|%s' "$safe_unit" "$phase" "$reason" "$result" "$status" | sha256sum | awk '{print $1}')
payload="t10.failure.v1 phase=$phase reason=$reason systemd_result=$result exec_status=$status dedupe=$dedupe"
command -v logger >/dev/null 2>&1 || { printf '%s\n' "$payload" >&2; exit 1; }
logger -t sub2api-account-quality "$payload"
