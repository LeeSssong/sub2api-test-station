#!/bin/sh
set -eu

unit=${MONITOR_UNIT:-${T10_UNIT_NAME:-unknown}}
service_result=${MONITOR_SERVICE_RESULT:-${SERVICE_RESULT:-unknown}}
exit_code=${MONITOR_EXIT_CODE:-${EXIT_CODE:-unknown}}
exit_status=${MONITOR_EXIT_STATUS:-${EXIT_STATUS:-unknown}}
case "$unit" in sub2api-account-quality-monitor.service) ;; *) unit=unknown ;; esac
case "$service_result" in success|exit-code|signal|core-dump|watchdog|start-limit-hit|resources|timeout|unknown) ;; *) service_result=unknown ;; esac
case "$exit_code" in exited|killed|dumped|unknown) ;; *) exit_code=unknown ;; esac
case "$exit_status" in 40|41|42|43|44|45|46|203|203/EXEC) ;; *) exit_status=unknown ;; esac
case "$exit_status" in
  203|203/EXEC) exit_status=203; failure_phase=systemd; reason_code=systemd_exec_203 ;;
  40) failure_phase=preflight; reason_code=path_or_mode_preflight ;;
  41) failure_phase=evidence; reason_code=uid10002_evidence_write ;;
  42) failure_phase=credentials; reason_code=admin_key_read ;;
  43) failure_phase=runtime; reason_code=docker_start_or_runtime ;;
  44) failure_phase=collector; reason_code=collector_nonzero ;;
  45) failure_phase=resource; reason_code=timeout_or_resource ;;
  46) failure_phase=publish; reason_code=evidence_publish ;;
  *) failure_phase=unknown; reason_code=unknown ;;
esac
stable="schema_version=t10.failure.v1 unit=$unit service_result=$service_result exit_code=$exit_code exit_status=$exit_status failure_phase=$failure_phase reason_code=$reason_code"
dedupe_key=$(printf '%s' "$stable" | sha256sum | awk '{print $1}')
command -v logger >/dev/null 2>&1 || exit 1
logger -t sub2api-account-quality "$stable dedupe_key=$dedupe_key"
