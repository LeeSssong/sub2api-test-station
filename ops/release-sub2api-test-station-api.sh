#!/usr/bin/env bash
set -euo pipefail
umask 077
fail(){ printf 'test_station_api_release status=failed: %s\n' "$1" >&2; exit 1; }
root=$(pwd -P)
[[ -d "$root/.git" && $(git branch --show-current) == main ]] || fail 'use the root main checkout'
[[ -z $(git status --porcelain) ]] || fail 'main must be clean'
git fetch origin main >/dev/null 2>&1 || fail 'origin fetch failed'
commit=$(git rev-parse HEAD)
tree=$(git rev-parse 'HEAD^{tree}')
[[ "$commit" == $(git rev-parse origin/main) && "$tree" == $(git rev-parse 'origin/main^{tree}') ]] || fail 'main must match pushed origin/main'
: "${TEST_STATION_APPROVED_HOST:?explicitly approved test-station host required}"
[[ "$TEST_STATION_APPROVED_HOST" == 43.133.75.82 ]] || fail 'invalid test-station host'
ssh_opts=(-o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=15 -o ServerAliveInterval=10 -o ServerAliveCountMax=18)
config=$(ssh -G "${ssh_opts[@]}" sub2api-test-station 2>/dev/null)
grep -Fqx "hostname $TEST_STATION_APPROVED_HOST" <<<"$config" || fail 'SSH alias differs from approved host'
grep -Fqx 'user ubuntu' <<<"$config" || fail 'test station must use ubuntu'
grep -Fqx 'port 22' <<<"$config" || fail 'test station must use port 22'
staging="$root/.release/test-station-api-$commit"
mkdir -p "$staging"
ssh -T "${ssh_opts[@]}" sub2api-test-station 'sudo -n cat /opt/sub2api-test-station/release-state.json' >"$staging/previous-state.json"
previous=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["source_commit"])' "$staging/previous-state.json")
update_worker=${TEST_STATION_UPDATE_WORKER:-false}
[[ "$update_worker" == true || "$update_worker" == false ]] || fail 'invalid worker update flag'
# Monitor service changes require the same new binary in API and singleton worker.
# Native group catalogue handlers and their read-only tool mapping can update API only.
# The reviewed user popularity projection reuses native read-only aggregates, API only.
# The reviewed manual check admission only affects API handlers; worker work is unchanged.
# Route SLA timeline and admin raw counts are read-only API queries, not worker jobs.
# Runtime dependencies and unreviewed backend changes remain excluded.
# The exact production integration files require the serialized worker update.
# Embedded migrations must already be applied with identical checksums on the host.
# Homepage files are deployed independently and are not inputs to the API binary.
# The embedded brand icon handler only reads existing public settings.
while IFS= read -r path; do
  case "$path" in
    # Reviewed removal of request quality snapshots; API and worker must agree.
    upstream/sub2api/backend/ent/migrate/schema.go|\
    upstream/sub2api/backend/ent/mutation.go|\
    upstream/sub2api/backend/ent/schema/usage_log.go|\
    upstream/sub2api/backend/ent/usagelog.go|\
    upstream/sub2api/backend/ent/usagelog/usagelog.go|\
    upstream/sub2api/backend/ent/usagelog/where.go|\
    upstream/sub2api/backend/ent/usagelog_create.go|\
    upstream/sub2api/backend/ent/usagelog_update.go|\
    upstream/sub2api/backend/internal/handler/gateway_web_search.go|\
    upstream/sub2api/backend/internal/handler/grok_audio.go|\
    upstream/sub2api/backend/internal/handler/openai_gateway_handler.go|\
    upstream/sub2api/backend/internal/repository/account_quality.go|\
    upstream/sub2api/backend/internal/repository/monitor_v4_quality_traffic.go|\
    upstream/sub2api/backend/internal/repository/quality_traffic.go|\
    upstream/sub2api/backend/internal/repository/quality_traffic_postgres_test.go|\
    upstream/sub2api/backend/internal/repository/usage_log_snapshot_removal_postgres_test.go|\
    upstream/sub2api/backend/internal/repository/usage_log_repo_insert.go|\
    upstream/sub2api/backend/internal/repository/usage_log_repo_insert_shape_unit_test.go|\
    upstream/sub2api/backend/internal/repository/usage_log_repo_request_type_test.go|\
    upstream/sub2api/backend/internal/repository/usage_log_session_id_unit_test.go|\
    upstream/sub2api/backend/internal/service/antigravity_gateway_claude.go|\
    upstream/sub2api/backend/internal/service/antigravity_gateway_compat.go|\
    upstream/sub2api/backend/internal/service/antigravity_gateway_gemini.go|\
    upstream/sub2api/backend/internal/service/antigravity_gateway_upstream.go|\
    upstream/sub2api/backend/internal/service/gateway_forward.go|\
    upstream/sub2api/backend/internal/service/gateway_forward_as_chat_completions.go|\
    upstream/sub2api/backend/internal/service/gateway_forward_as_responses.go|\
    upstream/sub2api/backend/internal/service/gateway_service.go|\
    upstream/sub2api/backend/internal/service/gateway_systemone.go|\
    upstream/sub2api/backend/internal/service/gateway_usage_billing.go|\
    upstream/sub2api/backend/internal/service/gemini_chat_completions_compat_service.go|\
    upstream/sub2api/backend/internal/service/gemini_messages_compat_service.go|\
    upstream/sub2api/backend/internal/service/grok_audio.go|\
    upstream/sub2api/backend/internal/service/grok_media.go|\
    upstream/sub2api/backend/internal/service/openai_alpha_search.go|\
    upstream/sub2api/backend/internal/service/openai_embeddings.go|\
    upstream/sub2api/backend/internal/service/openai_gateway_chat_completions.go|\
    upstream/sub2api/backend/internal/service/openai_gateway_forward.go|\
    upstream/sub2api/backend/internal/service/openai_gateway_messages.go|\
    upstream/sub2api/backend/internal/service/openai_gateway_service.go|\
    upstream/sub2api/backend/internal/service/openai_gateway_usage.go|\
    upstream/sub2api/backend/internal/service/openai_images.go|\
    upstream/sub2api/backend/internal/service/openai_live.go|\
    upstream/sub2api/backend/internal/service/openai_ws_forwarder_ingress.go|\
    upstream/sub2api/backend/internal/service/openai_ws_forwarder_v2.go|\
    upstream/sub2api/backend/internal/service/openai_ws_http_bridge.go|\
    upstream/sub2api/backend/internal/service/openai_ws_v2_passthrough_adapter.go|\
    upstream/sub2api/backend/internal/service/pelican_scheduled.go|\
    upstream/sub2api/backend/internal/service/quality_traffic.go|\
    upstream/sub2api/backend/internal/service/quality_traffic_test.go|\
    upstream/sub2api/backend/internal/service/scheduled_test_port.go|\
    upstream/sub2api/backend/internal/service/seedance.go|\
    upstream/sub2api/backend/internal/service/usage_log.go)
      [[ "$update_worker" == true ]] || fail 'snapshot removal requires a worker update' ;;
    upstream/sub2api/backend/migrations/266_quality_traffic_snapshots.sql)
      [[ "$update_worker" == true && ! -e "$root/$path" &&
         "$(git diff --diff-filter=D --name-only "$previous" HEAD -- "$path")" == "$path" ]] \
        || fail 'only deletion of snapshot migration with worker update is supported' ;;
    upstream/sub2api/frontend/src/*|upstream/sub2api/frontend/DESIGN.md|docs/*|ops/*|tests/*|artifacts/*) ;;
    homepage/*|infra/independent-test-station/Dockerfile.homepage|\
    upstream/sub2api/backend/internal/web/embed_on.go|\
    upstream/sub2api/backend/internal/web/embed_test.go|\
    upstream/sub2api/backend/internal/web/favicon.go|\
    upstream/sub2api/backend/internal/web/favicon_test.go) ;;
    upstream/sub2api/backend/internal/handler/usage_handler.go|\
    upstream/sub2api/backend/internal/handler/usage_model_popularity.go|\
    upstream/sub2api/backend/internal/handler/usage_model_popularity_test.go|\
    upstream/sub2api/backend/internal/middleware/line_check_rate_limiter.go|\
    upstream/sub2api/backend/internal/server/middleware/line_check_rate_limit.go|\
    upstream/sub2api/backend/internal/server/middleware/panel_rate_limit.go|\
    upstream/sub2api/backend/internal/server/routes/monitor_v4_check_rate_limit_test.go|\
    upstream/sub2api/backend/internal/service/monitor_v4.go|\
    upstream/sub2api/backend/internal/service/monitor_v4_check.go|\
    upstream/sub2api/backend/internal/service/monitor_v4_check_test.go|\
    upstream/sub2api/backend/internal/service/monitor_v4_timeline.go|\
    upstream/sub2api/backend/internal/service/monitor_v4_timeline_test.go|\
    upstream/sub2api/backend/internal/service/monitor_v4_sla_test.go|\
    upstream/sub2api/backend/internal/handler/monitor_v4_handler.go|\
    upstream/sub2api/backend/internal/handler/monitor_v4_handler_test.go|\
    upstream/sub2api/backend/internal/repository/monitor_v4_timeline.go|\
    upstream/sub2api/backend/internal/repository/monitor_v4_timeline_test.go|\
    upstream/sub2api/backend/internal/repository/route_cache_timeline_postgres_test.go|\
    upstream/sub2api/backend/internal/repository/ops_repo_dashboard.go|\
    upstream/sub2api/backend/internal/repository/ops_sla_sql.go|\
    upstream/sub2api/backend/internal/repository/route_sla_timeline_postgres_test.go|\
    upstream/sub2api/backend/scripts/verify_prototype_monitor_postgres.py|\
    upstream/sub2api/backend/internal/server/routes/user.go|\
    upstream/sub2api/backend/internal/handler/api_key_handler.go|\
    upstream/sub2api/backend/internal/handler/gateway_handler.go|\
    upstream/sub2api/backend/internal/handler/gateway_user_models.go|\
    upstream/sub2api/backend/internal/handler/gateway_model_catalog.go|\
    upstream/sub2api/backend/internal/handler/gateway_user_models_test.go|\
    upstream/sub2api/backend/internal/handler/api_key_available_groups_tools_test.go|\
    upstream/sub2api/backend/internal/handler/wire.go|\
    upstream/sub2api/backend/internal/handler/handler_wiring_test.go|\
    upstream/sub2api/backend/internal/service/model_plaza_service.go|\
    upstream/sub2api/backend/internal/service/model_plaza_service_test.go|\
    upstream/sub2api/backend/internal/service/group_tool_mapping.go|\
    upstream/sub2api/backend/internal/service/api_key_group_tool_mapping_test.go) ;;
    upstream/sub2api/backend/internal/service/account_test_service.go|\
    upstream/sub2api/backend/internal/service/account_test_service_openai_test.go|\
    upstream/sub2api/backend/internal/service/account_monitor_probe_test.go|\
    upstream/sub2api/backend/internal/service/account_probe_cost_test.go|\
    upstream/sub2api/backend/internal/service/monitor_v4*.go)
      [[ "$update_worker" == true ]] || fail 'monitor backend changes require a worker update' ;;
    upstream/sub2api/Dockerfile|\
    upstream/sub2api/backend/internal/repository/account_quality_models.go|\
    upstream/sub2api/backend/internal/repository/account_quality_models_test.go|\
    upstream/sub2api/backend/internal/repository/intelligence_rules.go|\
    upstream/sub2api/backend/internal/repository/intelligence_rules_integration_test.go|\
    upstream/sub2api/backend/internal/repository/migrations_runner.go|\
    upstream/sub2api/backend/internal/repository/migrations_test_main_online_test.go|\
    upstream/sub2api/backend/internal/repository/pelican_group_tests_repo.go|\
    upstream/sub2api/backend/internal/service/model_rate_limit.go|\
    upstream/sub2api/backend/internal/service/pelican_scheduled.go|\
    upstream/sub2api/backend/internal/service/quality_supported_models.go|\
    upstream/sub2api/backend/internal/service/quality_supported_models_test.go|\
    upstream/sub2api/backend/migrations/265_monitor_v4_legacy_default.sql|\
    upstream/sub2api/backend/migrations/241_remove_monitor_v4_operational_flag.sql|\
    upstream/sub2api/backend/migrations/deferred/241_remove_monitor_v4_operational_flag.sql|\
    upstream/sub2api/backend/migrations/deferred/README.md)
      [[ "$update_worker" == true ]] || fail 'production integration requires a worker update' ;;
    # Exact production synchronization scope; worker parity required.
    TASK-CONTEXT.md|\
    docs/diagnostics/2026-10-09-stream-timing-diagnostics.md|\
    docs/implementation/2026-10-13-intelligence-drawing-judge.md|\
    docs/project/releases/2026-10-12-ranxi-2105-user-tps-production.md|\
    docs/project/releases/2026-10-13-all-worktrees-production.md|\
    docs/project/releases/2026-10-13-sol-500k-billing-production.md|\
    docs/project/releases/2026-10-16-site-navigation-production.md|\
    docs/superpowers/plans/2026-10-10-narra-test-station.md|\
    docs/superpowers/plans/2026-10-12-ranxi-2105-user-tps.md|\
    docs/superpowers/plans/2026-10-13-intelligence-drawing-judge.md|\
    docs/superpowers/plans/2026-10-15-snowluma-migration.md|\
    docs/superpowers/specs/2026-10-10-narra-test-station-design.md|\
    homepage/src/App.test.tsx|\
    homepage/src/components/Header.test.tsx|\
    homepage/src/components/Header.tsx|\
    homepage/src/sections/HeroSection.test.tsx|\
    homepage/src/sections/HeroSection.tsx|\
    infra/cpa-test-station/Caddyfile|\
    infra/cpa-test-station/README.md|\
    infra/cpa-test-station/compose.keeper.yaml|\
    infra/cpa-test-station/compose.yaml|\
    infra/cpa-test-station/cpamc/management.html|\
    infra/cpa-test-station/cpamc/source.json|\
    infra/cpa-test-station/keeper-pool-ui/README.md|\
    infra/cpa-test-station/keeper-pool-ui/source.json|\
    infra/cpa-test-station/keeper-pool-ui/upstream.patch|\
    infra/cpa-test-station/keeper-pool-ui/web.tar.gz|\
    infra/cpa-test-station/keeper-source.json|\
    infra/cpa-test-station/legacy-entry.Caddyfile|\
    infra/cpa-test-station/ticket-gateway-ui.html|\
    infra/cpa-test-station/ticket-source.json|\
    infra/independent-test-station/Caddyfile|\
    infra/narra-test-station/.env.example|\
    infra/narra-test-station/README.md|\
    infra/narra-test-station/compose.yaml|\
    infra/qqbot/service/__init__.py|\
    infra/qqbot/service/app.py|\
    infra/qqbot/service/business.py|\
    infra/qqbot/service/migrate.py|\
    infra/qqbot/service/model.py|\
    infra/qqbot/service/protocol.py|\
    infra/qqbot/service/qq-desktop.py|\
    infra/qqbot/service/requirements.txt|\
    infra/qqbot/service/snowluma.service|\
    infra/qqbot/service/state.py|\
    infra/qqbot/service/xingqiao-bot.service|\
    infra/qqbot/service/xingqiao-qq-desktop.service|\
    infra/qqbot/service/xingqiao-qq-webdesktop.service|\
    infra/qqbot/service/xingqiao-qq.service|\
    infra/qqbot/tests/test_native_app.py|\
    infra/qqbot/tests/test_native_migrate.py|\
    infra/qqbot/tests/test_native_model.py|\
    infra/qqbot/tests/test_native_service.py|\
    infra/security/README.md|\
    infra/security/chromium-seccomp.json|\
    ops/deploy-cpa-test-keeper-pool.py|\
    ops/deploy-cpa-test-keeper.py|\
    ops/deploy-narra-test-station.py|\
    ops/deploy-narra-ui-blue-green.py|\
    ops/switch-cpa-test-cpamc.py|\
    ops/update-cpa-test-ticket.py|\
    tests/infra/ticket-plugin-release.test.cjs|\
    tests/infra/ticket-plugin-session.test.cjs|\
    tests/operations/narra_deploy_test.py|\
    tests/operations/narra_ui_release_test.py|\
    upstream/sub2api/Dockerfile|\
    upstream/sub2api/Makefile|\
    upstream/sub2api/XINGQIAO_UPSTREAM.md|\
    upstream/sub2api/backend/cmd/server/VERSION|\
    upstream/sub2api/backend/internal/config/config.go|\
    upstream/sub2api/backend/internal/handler/admin/account_handler.go|\
    upstream/sub2api/backend/internal/handler/admin/account_request_timezone_test.go|\
    upstream/sub2api/backend/internal/handler/admin/setting_handler.go|\
    upstream/sub2api/backend/internal/handler/admin/setting_handler_audit.go|\
    upstream/sub2api/backend/internal/handler/admin/setting_handler_update.go|\
    upstream/sub2api/backend/internal/handler/dto/excel_bps_credential_state_test.go|\
    upstream/sub2api/backend/internal/handler/dto/mappers.go|\
    upstream/sub2api/backend/internal/handler/dto/settings.go|\
    upstream/sub2api/backend/internal/handler/dto/types.go|\
    upstream/sub2api/backend/internal/handler/openai_chat_keepalive_error_test.go|\
    upstream/sub2api/backend/internal/handler/setting_handler.go|\
    upstream/sub2api/backend/internal/handler/setting_handler_public_test.go|\
    upstream/sub2api/backend/internal/repository/excel_bps_credential_state_test.go|\
    upstream/sub2api/backend/internal/repository/gateway_cache_bps_state.go|\
    upstream/sub2api/backend/internal/repository/gateway_cache_bps_state_test.go|\
    upstream/sub2api/backend/internal/repository/intelligence_drawing.go|\
    upstream/sub2api/backend/internal/repository/intelligence_drawing_integration_test.go|\
    upstream/sub2api/backend/internal/repository/openai_excel_oauth_repo.go|\
    upstream/sub2api/backend/internal/server/api_contract_test.go|\
    upstream/sub2api/backend/internal/server/routes/admin.go|\
    upstream/sub2api/backend/internal/service/account_test_bps_evidence.go|\
    upstream/sub2api/backend/internal/service/account_test_bps_evidence_test.go|\
    upstream/sub2api/backend/internal/service/account_test_service.go|\
    upstream/sub2api/backend/internal/service/account_test_service_bps_probe.go|\
    upstream/sub2api/backend/internal/service/admin_account.go|\
    upstream/sub2api/backend/internal/service/basispoints/catalog_cache.go|\
    upstream/sub2api/backend/internal/service/basispoints/persistent_state.go|\
    upstream/sub2api/backend/internal/service/basispoints/plan.go|\
    upstream/sub2api/backend/internal/service/basispoints/plan_test.go|\
    upstream/sub2api/backend/internal/service/basispoints/tools.go|\
    upstream/sub2api/backend/internal/service/domain_constants.go|\
    upstream/sub2api/backend/internal/service/excel_bps_credential_state.go|\
    upstream/sub2api/backend/internal/service/excel_bps_credential_state_test.go|\
    upstream/sub2api/backend/internal/service/intelligence_dashboard.go|\
    upstream/sub2api/backend/internal/service/intelligence_drawing.go|\
    upstream/sub2api/backend/internal/service/intelligence_drawing_render.go|\
    upstream/sub2api/backend/internal/service/intelligence_drawing_render_test.go|\
    upstream/sub2api/backend/internal/service/intelligence_drawing_test.go|\
    upstream/sub2api/backend/internal/service/intelligence_rules.go|\
    upstream/sub2api/backend/internal/service/openai_account_runtime_block_fastpath.go|\
    upstream/sub2api/backend/internal/service/openai_account_scheduler.go|\
    upstream/sub2api/backend/internal/service/openai_bulk_account_settings.go|\
    upstream/sub2api/backend/internal/service/openai_chat_bridge_keepalive_test.go|\
    upstream/sub2api/backend/internal/service/openai_codex_models_service.go|\
    upstream/sub2api/backend/internal/service/openai_codex_models_service_test.go|\
    upstream/sub2api/backend/internal/service/openai_codex_ticket.go|\
    upstream/sub2api/backend/internal/service/openai_excel_bps.go|\
    upstream/sub2api/backend/internal/service/openai_excel_bps_model_cooldown.go|\
    upstream/sub2api/backend/internal/service/openai_excel_bps_model_cooldown_test.go|\
    upstream/sub2api/backend/internal/service/openai_excel_oauth.go|\
    upstream/sub2api/backend/internal/service/openai_excel_oauth_recovery_test.go|\
    upstream/sub2api/backend/internal/service/openai_excel_oauth_test.go|\
    upstream/sub2api/backend/internal/service/openai_gateway_chat_completions.go|\
    upstream/sub2api/backend/internal/service/openai_gateway_forward.go|\
    upstream/sub2api/backend/internal/service/openai_gateway_scheduling.go|\
    upstream/sub2api/backend/internal/service/openai_gateway_service.go|\
    upstream/sub2api/backend/internal/service/openai_request_locale.go|\
    upstream/sub2api/backend/internal/service/openai_request_locale_test.go|\
    upstream/sub2api/backend/internal/service/openai_request_locale_transport_test.go|\
    upstream/sub2api/backend/internal/service/openai_request_timezone.go|\
    upstream/sub2api/backend/internal/service/openai_request_timezones.txt|\
    upstream/sub2api/backend/internal/service/openai_stream_timing.go|\
    upstream/sub2api/backend/internal/service/openai_stream_timing_test.go|\
    upstream/sub2api/backend/internal/service/openai_subscription_long_context_restore_test.go|\
    upstream/sub2api/backend/internal/service/openai_ws_forwarder_ingress.go|\
    upstream/sub2api/backend/internal/service/openai_ws_v2_passthrough_adapter.go|\
    upstream/sub2api/backend/internal/service/pelican_group_tests.go|\
    upstream/sub2api/backend/internal/service/pelican_test_service.go|\
    upstream/sub2api/backend/internal/service/quality_judge.go|\
    upstream/sub2api/backend/internal/service/scheduled_test_port.go|\
    upstream/sub2api/backend/internal/service/setting_parse.go|\
    upstream/sub2api/backend/internal/service/setting_request_timezone.go|\
    upstream/sub2api/backend/internal/service/setting_request_timezone_test.go|\
    upstream/sub2api/backend/internal/service/setting_request_timezone_update_test.go|\
    upstream/sub2api/backend/internal/service/setting_service.go|\
    upstream/sub2api/backend/internal/service/setting_update.go|\
    upstream/sub2api/backend/internal/service/settings_view.go|\
    upstream/sub2api/backend/internal/service/wire.go|\
    upstream/sub2api/deploy/config.example.yaml|\
    upstream/sub2api/docs/features/bps-credential-diagnostics.md|\
    upstream/sub2api/docs/features/bps-model-permission-cooldown.md|\
    upstream/sub2api/docs/features/bps-redis-state.md|\
    upstream/sub2api/docs/features/excel-bps-refresh-recovery.md|\
    upstream/sub2api/docs/features/request-timezone.md|\
    upstream/sub2api/docs/screenshots/bps-credential-diagnostics/README.md|\
    upstream/sub2api/docs/screenshots/bps-credential-diagnostics/states.png|\
    upstream/sub2api/docs/screenshots/bps-test-evidence/README.md|\
    upstream/sub2api/docs/screenshots/bps-test-evidence/responses.png|\
    upstream/sub2api/docs/screenshots/request-timezone/README.md|\
    upstream/sub2api/docs/screenshots/request-timezone/disabled.png|\
    upstream/sub2api/docs/screenshots/request-timezone/enabled.png|\
    upstream/sub2api/frontend/src/api/admin/accounts.ts|\
    upstream/sub2api/frontend/src/api/admin/intelligenceRules.ts|\
    upstream/sub2api/frontend/src/api/admin/pelicanTests.ts|\
    upstream/sub2api/frontend/src/api/admin/settings.ts|\
    upstream/sub2api/frontend/src/api/intelligenceTests.ts|\
    upstream/sub2api/frontend/src/components/account/AccountStatusIndicator.vue|\
    upstream/sub2api/frontend/src/components/account/AccountTestEvidence.vue|\
    upstream/sub2api/frontend/src/components/account/AccountTestModal.vue|\
    upstream/sub2api/frontend/src/components/account/AccountUsageCell.vue|\
    upstream/sub2api/frontend/src/components/account/CreateAccountModal.vue|\
    upstream/sub2api/frontend/src/components/account/EditAccountModal.vue|\
    upstream/sub2api/frontend/src/components/account/ExcelBPSCredentialStatus.vue|\
    upstream/sub2api/frontend/src/components/account/OpenAIRequestTimezoneField.vue|\
    upstream/sub2api/frontend/src/components/account/__tests__/AccountTestModal.spec.ts|\
    upstream/sub2api/frontend/src/components/account/__tests__/ExcelBPSCredentialStatus.spec.ts|\
    upstream/sub2api/frontend/src/components/account/__tests__/OpenAIRequestTimezoneField.spec.ts|\
    upstream/sub2api/frontend/src/components/admin/operations/IntelligenceRulesPanel.vue|\
    upstream/sub2api/frontend/src/components/admin/usage/UsageTable.vue|\
    upstream/sub2api/frontend/src/components/admin/usage/__tests__/UsageTable.spec.ts|\
    upstream/sub2api/frontend/src/components/layout/AppSidebar.vue|\
    upstream/sub2api/frontend/src/components/settings/RequestTimezoneSettings.vue|\
    upstream/sub2api/frontend/src/components/user/intelligence/IntelligenceDrawingRibbon.vue|\
    upstream/sub2api/frontend/src/components/user/intelligence/IntelligenceGroupCard.vue|\
    upstream/sub2api/frontend/src/components/user/intelligence/IntelligenceResultDialog.vue|\
    upstream/sub2api/frontend/src/components/user/intelligence/__tests__/IntelligenceDrawingBadge.spec.ts|\
    upstream/sub2api/frontend/src/components/user/intelligence/__tests__/IntelligenceResultDialog.spec.ts|\
    upstream/sub2api/frontend/src/components/user/intelligence/intelligence.css|\
    upstream/sub2api/frontend/src/features/ai-tools/RouteHistoryStrip.vue|\
    upstream/sub2api/frontend/src/features/ai-tools/__tests__/RouteHistoryStrip.spec.ts|\
    upstream/sub2api/frontend/src/features/channel-monitor-v3/V3SettingsPanel.vue|\
    upstream/sub2api/frontend/src/i18n/locales/en/admin/accounts.ts|\
    upstream/sub2api/frontend/src/i18n/locales/en/admin/settings.ts|\
    upstream/sub2api/frontend/src/i18n/locales/en/dashboard.ts|\
    upstream/sub2api/frontend/src/i18n/locales/en/requestTiming.ts|\
    upstream/sub2api/frontend/src/i18n/locales/zh/admin/accounts.ts|\
    upstream/sub2api/frontend/src/i18n/locales/zh/admin/settings.ts|\
    upstream/sub2api/frontend/src/i18n/locales/zh/dashboard.ts|\
    upstream/sub2api/frontend/src/i18n/locales/zh/requestTiming.ts|\
    upstream/sub2api/frontend/src/router/__tests__/performance-monitor-route.spec.ts|\
    upstream/sub2api/frontend/src/router/index.ts|\
    upstream/sub2api/frontend/src/types/accountTestEvidence.ts|\
    upstream/sub2api/frontend/src/types/index.ts|\
    upstream/sub2api/frontend/src/utils/__tests__/intelligenceRules.spec.ts|\
    upstream/sub2api/frontend/src/utils/__tests__/pelicanPreview.spec.ts|\
    upstream/sub2api/frontend/src/utils/__tests__/usageTps.spec.ts|\
    upstream/sub2api/frontend/src/utils/excelBpsCredentialState.ts|\
    upstream/sub2api/frontend/src/utils/intelligenceRules.ts|\
    upstream/sub2api/frontend/src/utils/pelicanPreview.ts|\
    upstream/sub2api/frontend/src/utils/usageTps.ts|\
    upstream/sub2api/frontend/src/views/admin/AccountsView.vue|\
    upstream/sub2api/frontend/src/views/admin/SettingsView.vue|\
    upstream/sub2api/frontend/src/views/admin/__tests__/SettingsView.spec.ts|\
    upstream/sub2api/frontend/src/views/user/DashboardView.vue|\
    upstream/sub2api/frontend/src/views/user/ImageWorkstationView.vue|\
    upstream/sub2api/frontend/src/views/user/IntelligenceTestView.vue|\
    upstream/sub2api/frontend/src/views/user/__tests__/DashboardView.parity.spec.ts|\
    upstream/sub2api/frontend/src/views/user/__tests__/ImageWorkstationView.spec.ts|\
    workstations/narra/.dockerignore|\
    workstations/narra/.env.example|\
    workstations/narra/.gitattributes|\
    workstations/narra/.gitignore|\
    workstations/narra/AGENTS.md|\
    workstations/narra/Dockerfile|\
    workstations/narra/Dockerfile.site-runtime|\
    workstations/narra/LICENSE|\
    workstations/narra/README.md|\
    workstations/narra/UPSTREAM.md|\
    workstations/narra/contracts/gateway/v1/envelope.json|\
    workstations/narra/contracts/gateway/v1/scenarios/envelope.json|\
    workstations/narra/contracts/generation/v1/errors.json|\
    workstations/narra/contracts/generation/v1/models.json|\
    workstations/narra/contracts/generation/v1/scenarios/channels.json|\
    workstations/narra/contracts/generation/v1/scenarios/lifecycle.json|\
    workstations/narra/contracts/generation/v1/scenarios/media.json|\
    workstations/narra/contracts/generation/v1/scenarios/secret.json|\
    workstations/narra/contracts/generation/v1/schema.json|\
    workstations/narra/contracts/generation/v1/states.json|\
    workstations/narra/contracts/prompts/v1/default-sources.json|\
    workstations/narra/contracts/runtime/v1/environment.json|\
    workstations/narra/contracts/runtime/v1/redaction.json|\
    workstations/narra/docker-compose.e2e.yml|\
    workstations/narra/docker-compose.yml|\
    workstations/narra/eslint.config.mjs|\
    workstations/narra/next-env.d.ts|\
    workstations/narra/next.config.ts|\
    workstations/narra/package.json|\
    workstations/narra/pet-browser-verify.json|\
    workstations/narra/pnpm-lock.yaml|\
    workstations/narra/pnpm-workspace.yaml|\
    workstations/narra/postcss.config.mjs|\
    workstations/narra/prisma.config.ts|\
    workstations/narra/prisma/create-prisma-client.ts|\
    workstations/narra/prisma/migrations/20260423000000_initial_schema/migration.sql|\
    workstations/narra/prisma/migrations/20260423165000_single_image_works/migration.sql|\
    workstations/narra/prisma/migrations/20260427090000_likes_redeem_codes_multi_sources/migration.sql|\
    workstations/narra/prisma/migrations/20260428083000_generation_image_options/migration.sql|\
    workstations/narra/prisma/migrations/20260502143000_add_turnstile_config/migration.sql|\
    workstations/narra/prisma/migrations/20260502153000_generation_image_dimensions/migration.sql|\
    workstations/narra/prisma/migrations/20260505000000_add_conversation/migration.sql|\
    workstations/narra/prisma/migrations/20260505160000_api_keys_external_api/migration.sql|\
    workstations/narra/prisma/migrations/20260509000000_add_performance_indexes/migration.sql|\
    workstations/narra/prisma/migrations/20260512000000_add_login_cover_config/migration.sql|\
    workstations/narra/prisma/migrations/20260530090000_go_worker_generation_jobs/migration.sql|\
    workstations/narra/prisma/migrations/20260601000000_video_workspace/migration.sql|\
    workstations/narra/prisma/migrations/20260602000000_prompt_library/migration.sql|\
    workstations/narra/prisma/migrations/20260807130000_generation_worker_contract_v1/migration.sql|\
    workstations/narra/prisma/migrations/20260807140000_generation_media_storage_meta/migration.sql|\
    workstations/narra/prisma/migrations/20260831214800_add_user_ban/migration.sql|\
    workstations/narra/prisma/migrations/20260901120000_add_content_moderation/migration.sql|\
    workstations/narra/prisma/schema.prisma|\
    workstations/narra/prisma/seed.ts|\
    workstations/narra/scripts/fixtures/synthetic-foundation-provenance.json|\
    workstations/narra/scripts/fixtures/synthetic-foundation-schema.prisma|\
    workstations/narra/scripts/lib/command-runner.mjs|\
    workstations/narra/scripts/lib/disposable-postgres.mjs|\
    workstations/narra/scripts/lib/migration-runtime.mjs|\
    workstations/narra/scripts/lib/migration-safety.mjs|\
    workstations/narra/scripts/migrate-deploy.mjs|\
    workstations/narra/scripts/migrations/baseline.mjs|\
    workstations/narra/scripts/migrations/prehistory.mjs|\
    workstations/narra/scripts/migrations/repair-allowlist.mjs|\
    workstations/narra/scripts/migrations/repair.mjs|\
    workstations/narra/scripts/site-cleanup.mjs|\
    workstations/narra/scripts/start-prod.mjs|\
    workstations/narra/scripts/tests/migration-safety.test.mjs|\
    workstations/narra/scripts/tests/release-startup.test.mjs|\
    workstations/narra/scripts/verify-ci.mjs|\
    workstations/narra/scripts/verify-e2e.mjs|\
    workstations/narra/scripts/verify-gateway.mjs|\
    workstations/narra/scripts/verify-migrations.mjs|\
    workstations/narra/scripts/verify-worker-contracts.mjs|\
    workstations/narra/src/app/admin/generations/page.tsx|\
    workstations/narra/src/app/api/generate/route.ts|\
    workstations/narra/src/app/api/healthz/route.ts|\
    workstations/narra/src/app/api/me/generations/[id]/route.ts|\
    workstations/narra/src/app/api/me/generations/route.ts|\
    workstations/narra/src/app/api/provider-models/probe/route.ts|\
    workstations/narra/src/app/api/readyz/route.ts|\
    workstations/narra/src/app/api/workstation/enter/route.ts|\
    workstations/narra/src/app/api/workstation/media/[userId]/[file]/route.ts|\
    workstations/narra/src/app/api/workstation/source/route.ts|\
    workstations/narra/src/app/create/page.tsx|\
    workstations/narra/src/app/error.tsx|\
    workstations/narra/src/app/favicon.ico|\
    workstations/narra/src/app/globals.css|\
    workstations/narra/src/app/layout.tsx|\
    workstations/narra/src/app/loading.tsx|\
    workstations/narra/src/app/not-found.tsx|\
    workstations/narra/src/app/page.tsx|\
    workstations/narra/src/components/admin/admin-actions.tsx|\
    workstations/narra/src/components/admin/admin-frame.tsx|\
    workstations/narra/src/components/admin/admin-nav.tsx|\
    workstations/narra/src/components/admin/admin-pagination.tsx|\
    workstations/narra/src/components/admin/admin-works-board.tsx|\
    workstations/narra/src/components/admin/api-admin-panel.tsx|\
    workstations/narra/src/components/admin/channel-manager.tsx|\
    workstations/narra/src/components/admin/check-in-benefit-form.tsx|\
    workstations/narra/src/components/admin/cleanup-failed-button.tsx|\
    workstations/narra/src/components/admin/generation-search-bar.tsx|\
    workstations/narra/src/components/admin/invite-table-actions.tsx|\
    workstations/narra/src/components/admin/login-cover-form.tsx|\
    workstations/narra/src/components/admin/moderation-config-form.tsx|\
    workstations/narra/src/components/admin/moderation-reviews-board.tsx|\
    workstations/narra/src/components/admin/oauth-provider-manager.tsx|\
    workstations/narra/src/components/admin/prompt-source-manager.tsx|\
    workstations/narra/src/components/admin/redeem-code-manager.tsx|\
    workstations/narra/src/components/admin/settings-sub-nav.tsx|\
    workstations/narra/src/components/admin/showcase-auto-approve-toggle.tsx|\
    workstations/narra/src/components/admin/turnstile-config-form.tsx|\
    workstations/narra/src/components/admin/user-admin-card.tsx|\
    workstations/narra/src/components/admin/user-search-bar.tsx|\
    workstations/narra/src/components/api/api-key-console.tsx|\
    workstations/narra/src/components/auth/logout-button.tsx|\
    workstations/narra/src/components/auth/turnstile-widget.tsx|\
    workstations/narra/src/components/benefits/check-in-button.tsx|\
    workstations/narra/src/components/create/constants.ts|\
    workstations/narra/src/components/create/generator-studio.tsx|\
    workstations/narra/src/components/create/hooks/use-image-poller.ts|\
    workstations/narra/src/components/create/hooks/use-reference-images.ts|\
    workstations/narra/src/components/create/hooks/use-sessions.ts|\
    workstations/narra/src/components/create/parts/advanced-settings.tsx|\
    workstations/narra/src/components/create/parts/chat-stream.tsx|\
    workstations/narra/src/components/create/parts/composer.tsx|\
    workstations/narra/src/components/create/parts/generation-bubble.tsx|\
    workstations/narra/src/components/create/parts/history-rail.tsx|\
    workstations/narra/src/components/create/parts/image-zoom-modal.tsx|\
    workstations/narra/src/components/create/parts/session-sidebar.tsx|\
    workstations/narra/src/components/create/site-studio.tsx|\
    workstations/narra/src/components/create/types.ts|\
    workstations/narra/src/components/create/utils.ts|\
    workstations/narra/src/components/invites/invite-claim-board.tsx|\
    workstations/narra/src/components/layout/page-transition.tsx|\
    workstations/narra/src/components/marketing/auth-cover.tsx|\
    workstations/narra/src/components/marketing/auth-form.tsx|\
    workstations/narra/src/components/marketing/auth-shell.tsx|\
    workstations/narra/src/components/marketing/featured-gallery.tsx|\
    workstations/narra/src/components/marketing/header-shell.tsx|\
    workstations/narra/src/components/marketing/mobile-nav.tsx|\
    workstations/narra/src/components/marketing/site-header.tsx|\
    workstations/narra/src/components/pet/pet-catalog.ts|\
    workstations/narra/src/components/pet/pet-companion.tsx|\
    workstations/narra/src/components/pet/pet-state.ts|\
    workstations/narra/src/components/pet/pet-toggle.tsx|\
    workstations/narra/src/components/pet/pet-turnaround.ts|\
    workstations/narra/src/components/prompts/prompt-library-board.tsx|\
    workstations/narra/src/components/settings/profile-form.tsx|\
    workstations/narra/src/components/settings/redeem-code-form.tsx|\
    workstations/narra/src/components/ui/alert.tsx|\
    workstations/narra/src/components/ui/empty-state.tsx|\
    workstations/narra/src/components/ui/skeleton.tsx|\
    workstations/narra/src/components/ui/spinner.tsx|\
    workstations/narra/src/components/video/constants.ts|\
    workstations/narra/src/components/video/parts/video-composer.tsx|\
    workstations/narra/src/components/video/parts/video-generating-progress.tsx|\
    workstations/narra/src/components/video/parts/video-history-rail.tsx|\
    workstations/narra/src/components/video/parts/video-stage.tsx|\
    workstations/narra/src/components/video/types.ts|\
    workstations/narra/src/components/video/video-studio.tsx|\
    workstations/narra/src/components/works/download-image.ts|\
    workstations/narra/src/components/works/image-lightbox.tsx|\
    workstations/narra/src/components/works/my-works-board.tsx|\
    workstations/narra/src/components/works/prompt-modal.tsx|\
    workstations/narra/src/components/works/video-works-section.tsx|\
    workstations/narra/src/components/works/work-detail-panel.tsx|\
    workstations/narra/src/components/works/work-share-button.tsx|\
    workstations/narra/src/components/works/work-showcase-controls.tsx|\
    workstations/narra/src/lib/api-config.ts|\
    workstations/narra/src/lib/api-errors.ts|\
    workstations/narra/src/lib/api-keys.ts|\
    workstations/narra/src/lib/auth/linuxdo-oauth.ts|\
    workstations/narra/src/lib/auth/oauth-config.ts|\
    workstations/narra/src/lib/auth/password.ts|\
    workstations/narra/src/lib/auth/register-user.ts|\
    workstations/narra/src/lib/auth/session-token.ts|\
    workstations/narra/src/lib/auth/session.ts|\
    workstations/narra/src/lib/auth/turnstile.ts|\
    workstations/narra/src/lib/benefits/check-in.ts|\
    workstations/narra/src/lib/benefits/config.ts|\
    workstations/narra/src/lib/constants.ts|\
    workstations/narra/src/lib/credits.ts|\
    workstations/narra/src/lib/db.ts|\
    workstations/narra/src/lib/env.ts|\
    workstations/narra/src/lib/external-api/chat.ts|\
    workstations/narra/src/lib/external-api/http.ts|\
    workstations/narra/src/lib/external-api/image-edits.ts|\
    workstations/narra/src/lib/external-api/images.ts|\
    workstations/narra/src/lib/external-api/responses.ts|\
    workstations/narra/src/lib/external-api/source-images.ts|\
    workstations/narra/src/lib/generation/contracts.ts|\
    workstations/narra/src/lib/generation/external-api.ts|\
    workstations/narra/src/lib/generation/gateway-client.ts|\
    workstations/narra/src/lib/generation/gateway-contract.ts|\
    workstations/narra/src/lib/generation/image-dimensions.ts|\
    workstations/narra/src/lib/generation/job-refund.ts|\
    workstations/narra/src/lib/generation/limits.ts|\
    workstations/narra/src/lib/generation/parse-generate-request.ts|\
    workstations/narra/src/lib/generation/sizes.ts|\
    workstations/narra/src/lib/image-url.ts|\
    workstations/narra/src/lib/invites/claim-invite.server.ts|\
    workstations/narra/src/lib/invites/claim-invite.ts|\
    workstations/narra/src/lib/logging/redact.ts|\
    workstations/narra/src/lib/moderation/check.ts|\
    workstations/narra/src/lib/moderation/config.ts|\
    workstations/narra/src/lib/moderation/sensitive-words.ts|\
    workstations/narra/src/lib/prisma-mappers.ts|\
    workstations/narra/src/lib/prompt-preview.ts|\
    workstations/narra/src/lib/prompts/service.ts|\
    workstations/narra/src/lib/prompts/types.ts|\
    workstations/narra/src/lib/providers/built-in-provider-core.ts|\
    workstations/narra/src/lib/providers/built-in-provider.ts|\
    workstations/narra/src/lib/providers/model-catalog.ts|\
    workstations/narra/src/lib/providers/provider-secret.ts|\
    workstations/narra/src/lib/public-env.ts|\
    workstations/narra/src/lib/readiness.ts|\
    workstations/narra/src/lib/redeem-codes.ts|\
    workstations/narra/src/lib/runtime-environment-contract.ts|\
    workstations/narra/src/lib/server/api-auth.ts|\
    workstations/narra/src/lib/server/current-user.ts|\
    workstations/narra/src/lib/server/http.ts|\
    workstations/narra/src/lib/server/login-cover.ts|\
    workstations/narra/src/lib/server/safe-remote-url.ts|\
    workstations/narra/src/lib/server/works.ts|\
    workstations/narra/src/lib/storage/persist-generated-image.ts|\
    workstations/narra/src/lib/types.ts|\
    workstations/narra/src/lib/utils.ts|\
    workstations/narra/src/lib/validators.ts|\
    workstations/narra/src/lib/work-showcase.ts|\
    workstations/narra/src/lib/workstation/capabilities.ts|\
    workstations/narra/src/lib/workstation/files.ts|\
    workstations/narra/src/lib/workstation/origin.ts|\
    workstations/narra/src/lib/workstation/policy.ts|\
    workstations/narra/src/lib/workstation/site.ts|\
    workstations/narra/src/proxy.ts|\
    workstations/narra/src/tests/contracts/gateway-contract.test.ts|\
    workstations/narra/src/tests/contracts/generation-contract.test.ts|\
    workstations/narra/src/tests/contracts/runtime-environment-contract.test.ts|\
    workstations/narra/src/tests/integration/worker-contracts/legacy-schema.sql|\
    workstations/narra/src/tests/integration/worker-contracts/postgres-runner.ts|\
    workstations/narra/src/tests/integration/worker-contracts/runner-vitest.config.ts|\
    workstations/narra/src/tests/integration/worker-contracts/worker-contracts-db.test.ts|\
    workstations/narra/src/tests/integration/worker-contracts/worker-contracts.test.ts|\
    workstations/narra/src/tests/setup/server-only.ts|\
    workstations/narra/src/tests/setup/vitest.setup.ts|\
    workstations/narra/src/tests/unit/admin-channels-provider.test.ts|\
    workstations/narra/src/tests/unit/admin-channels-route.test.ts|\
    workstations/narra/src/tests/unit/admin-generations-cleanup-route.test.ts|\
    workstations/narra/src/tests/unit/admin-generations-page.test.tsx|\
    workstations/narra/src/tests/unit/admin-generations-route.test.ts|\
    workstations/narra/src/tests/unit/admin-invite-batch-route.test.ts|\
    workstations/narra/src/tests/unit/admin-user-delete-route.test.ts|\
    workstations/narra/src/tests/unit/api-config.test.ts|\
    workstations/narra/src/tests/unit/api-key-console.test.tsx|\
    workstations/narra/src/tests/unit/auth-form.test.tsx|\
    workstations/narra/src/tests/unit/built-in-provider.test.ts|\
    workstations/narra/src/tests/unit/channel-manager.test.tsx|\
    workstations/narra/src/tests/unit/channel-validators.test.ts|\
    workstations/narra/src/tests/unit/check-in.test.ts|\
    workstations/narra/src/tests/unit/credits.test.ts|\
    workstations/narra/src/tests/unit/env-security.test.ts|\
    workstations/narra/src/tests/unit/external-generation-service.test.ts|\
    workstations/narra/src/tests/unit/external-source-images.test.ts|\
    workstations/narra/src/tests/unit/external-v1-routes.test.ts|\
    workstations/narra/src/tests/unit/featured-gallery.test.tsx|\
    workstations/narra/src/tests/unit/featured-route.test.ts|\
    workstations/narra/src/tests/unit/featured-video-serialization.test.ts|\
    workstations/narra/src/tests/unit/featured-works.test.ts|\
    workstations/narra/src/tests/unit/gateway-client.test.ts|\
    workstations/narra/src/tests/unit/generate-route-security.test.ts|\
    workstations/narra/src/tests/unit/generation-admin-list.test.tsx|\
    workstations/narra/src/tests/unit/generation-cancel-route.test.ts|\
    workstations/narra/src/tests/unit/generation-search-bar.test.tsx|\
    workstations/narra/src/tests/unit/generation-serialization.test.ts|\
    workstations/narra/src/tests/unit/generation-sizes.test.ts|\
    workstations/narra/src/tests/unit/generator-studio-edit.test.tsx|\
    workstations/narra/src/tests/unit/generator-studio-feedback.test.tsx|\
    workstations/narra/src/tests/unit/generator-studio-ratio.test.tsx|\
    workstations/narra/src/tests/unit/generator-studio-retry-cancel.test.tsx|\
    workstations/narra/src/tests/unit/generator-studio-session-history.test.tsx|\
    workstations/narra/src/tests/unit/health-routes.test.ts|\
    workstations/narra/src/tests/unit/image-dimensions.test.ts|\
    workstations/narra/src/tests/unit/image-lightbox.test.tsx|\
    workstations/narra/src/tests/unit/image-url.test.ts|\
    workstations/narra/src/tests/unit/invite-batch-view-button.test.tsx|\
    workstations/narra/src/tests/unit/invite-claim.test.ts|\
    workstations/narra/src/tests/unit/job-refund.test.ts|\
    workstations/narra/src/tests/unit/linuxdo-oauth.test.ts|\
    workstations/narra/src/tests/unit/log-redaction.test.ts|\
    workstations/narra/src/tests/unit/model-catalog-fetch.test.ts|\
    workstations/narra/src/tests/unit/model-catalog.test.ts|\
    workstations/narra/src/tests/unit/moderation-check.test.ts|\
    workstations/narra/src/tests/unit/moderation-config.test.ts|\
    workstations/narra/src/tests/unit/my-works-board.test.tsx|\
    workstations/narra/src/tests/unit/parse-generate-request.test.ts|\
    workstations/narra/src/tests/unit/persist-generated-image.test.ts|\
    workstations/narra/src/tests/unit/pet-companion.test.tsx|\
    workstations/narra/src/tests/unit/prisma-client.test.ts|\
    workstations/narra/src/tests/unit/prisma-mappers.test.ts|\
    workstations/narra/src/tests/unit/prompt-modal-copy.test.tsx|\
    workstations/narra/src/tests/unit/prompt-preview.test.ts|\
    workstations/narra/src/tests/unit/prompt-sync-forwarder.test.ts|\
    workstations/narra/src/tests/unit/provider-models-probe-route.test.ts|\
    workstations/narra/src/tests/unit/provider-secret.test.ts|\
    workstations/narra/src/tests/unit/proxy-image-route.test.ts|\
    workstations/narra/src/tests/unit/proxy-video-route.test.ts|\
    workstations/narra/src/tests/unit/readiness.test.ts|\
    workstations/narra/src/tests/unit/redeem-code.test.ts|\
    workstations/narra/src/tests/unit/register-route-bootstrap.test.ts|\
    workstations/narra/src/tests/unit/register-user.test.ts|\
    workstations/narra/src/tests/unit/safe-remote-url.test.ts|\
    workstations/narra/src/tests/unit/sensitive-words.test.ts|\
    workstations/narra/src/tests/unit/session-token.test.ts|\
    workstations/narra/src/tests/unit/site-capabilities.test.ts|\
    workstations/narra/src/tests/unit/site-files.test.ts|\
    workstations/narra/src/tests/unit/site-image-url.test.ts|\
    workstations/narra/src/tests/unit/site-workstation.test.ts|\
    workstations/narra/src/tests/unit/use-image-poller.test.tsx|\
    workstations/narra/src/tests/unit/video-generating-progress.test.tsx|\
    workstations/narra/src/tests/unit/video-stage.test.tsx|\
    workstations/narra/src/tests/unit/work-delete-route.test.ts|\
    workstations/narra/src/tests/unit/work-like-route.test.ts|\
    workstations/narra/src/tests/unit/work-share-button.test.tsx|\
    workstations/narra/src/tests/unit/work-showcase.test.ts|\
    workstations/narra/tsconfig.json|\
    workstations/narra/vitest.config.ts|\
    workstations/narra/vitest.site.config.ts|\
    workstations/narra/worker/Dockerfile|\
    workstations/narra/worker/cmd/backfill-media/main.go|\
    workstations/narra/worker/cmd/prompt-sync/main.go|\
    workstations/narra/worker/cmd/rollback-preflight/main.go|\
    workstations/narra/worker/cmd/rollback-preflight/main_test.go|\
    workstations/narra/worker/cmd/worker/main.go|\
    workstations/narra/worker/go.mod|\
    workstations/narra/worker/go.sum|\
    workstations/narra/worker/internal/worker/backfill.go|\
    workstations/narra/worker/internal/worker/config.go|\
    workstations/narra/worker/internal/worker/config_test.go|\
    workstations/narra/worker/internal/worker/contract_test.go|\
    workstations/narra/worker/internal/worker/contracts.go|\
    workstations/narra/worker/internal/worker/dimensions.go|\
    workstations/narra/worker/internal/worker/dimensions_test.go|\
    workstations/narra/worker/internal/worker/gateway.go|\
    workstations/narra/worker/internal/worker/gateway_db_test.go|\
    workstations/narra/worker/internal/worker/gateway_enqueue.go|\
    workstations/narra/worker/internal/worker/gateway_response.go|\
    workstations/narra/worker/internal/worker/gateway_test.go|\
    workstations/narra/worker/internal/worker/gateway_wait.go|\
    workstations/narra/worker/internal/worker/generation.go|\
    workstations/narra/worker/internal/worker/generation_test.go|\
    workstations/narra/worker/internal/worker/job_lifecycle.go|\
    workstations/narra/worker/internal/worker/lifecycle.go|\
    workstations/narra/worker/internal/worker/logging.go|\
    workstations/narra/worker/internal/worker/logging_test.go|\
    workstations/narra/worker/internal/worker/prompts.go|\
    workstations/narra/worker/internal/worker/prompts_test.go|\
    workstations/narra/worker/internal/worker/runtime_state.go|\
    workstations/narra/worker/internal/worker/runtime_state_test.go|\
    workstations/narra/worker/internal/worker/schema.go|\
    workstations/narra/worker/internal/worker/secret.go|\
    workstations/narra/worker/internal/worker/secret_test.go|\
    workstations/narra/worker/internal/worker/server.go|\
    workstations/narra/worker/internal/worker/server_test.go|\
    workstations/narra/worker/internal/worker/site_storage_test.go|\
    workstations/narra/worker/internal/worker/storage.go|\
    workstations/narra/worker/internal/worker/storage_test.go|\
    workstations/narra/worker/internal/worker/topology_lock.go|\
    workstations/narra/worker/internal/worker/topology_lock_test.go|\
    workstations/narra/worker/internal/worker/video.go|\
    workstations/narra/worker/internal/worker/video_test.go|\
    workstations/narra/worker/internal/worker/worker.go|\
    workstations/narra/worker/internal/worker/worker_contracts_integration_test.go|\
    workstations/narra/worker/internal/worker/worker_runtime_test.go)
      [[ "$update_worker" == true ]] || fail "main synchronization requires worker update" ;;
    *) fail "release excludes unsupported runtime changes: $path" ;;
  esac
done < <(git diff --name-only "$previous" HEAD)
binary_commit=$commit
binary_tree=$tree
if [[ -n ${TEST_STATION_REUSE_BUILD_FROM:-} ]]; then
  reuse=$TEST_STATION_REUSE_BUILD_FROM
  [[ "$reuse" == "$root/.release/test-station-api-"* && -d "$reuse" && ! -L "$reuse" ]] || fail 'reuse requires a local release artifact'
  binary_commit=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["source_commit"])' "$reuse/manifest.json")
  binary_tree=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["source_tree"])' "$reuse/manifest.json")
  [[ "$binary_commit" =~ ^[a-f0-9]{40}$ && "$binary_tree" == $(git rev-parse "$binary_commit^{tree}") ]] || fail 'reused source identity is invalid'
  git diff --quiet "$binary_commit" HEAD -- upstream/sub2api || fail 'runtime inputs changed; rebuild required'
  python3 - "$reuse" "$staging/previous-state.json" <<'PY'
import hashlib,json,pathlib,sys
root=pathlib.Path(sys.argv[1]); manifest=json.loads((root/'manifest.json').read_text())
previous=json.load(open(sys.argv[2]))
binary=root/'sub2api'
if binary.is_symlink() or hashlib.sha256(binary.read_bytes()).hexdigest()!=manifest['binary_sha256']:
    raise SystemExit('reused binary checksum mismatch')
if previous['image_id']!=manifest['base_image_id']:
    raise SystemExit('runtime dependencies changed; rebuild required')
PY
  cp "$reuse/sub2api" "$staging/sub2api"
  printf 'test_station_api_release stage=reused_build binary_source_commit=%s\n' "$binary_commit"
else
  printf 'test_station_api_release stage=frontend_build\n'
  (cd "$root/upstream/sub2api/frontend"; pnpm run build)
  printf 'test_station_api_release stage=embedded_server_build\n'
  version=$(cat "$root/upstream/sub2api/backend/cmd/server/VERSION")
  build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  (cd "$root/upstream/sub2api/backend"; CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embed -trimpath \
    -ldflags="-s -w -X main.Version=$version -X main.Commit=$commit -X main.Date=$build_date -X main.BuildType=release" \
    -o "$staging/sub2api" ./cmd/server)
fi
python3 - "$staging" "$commit" "$tree" "$binary_commit" "$binary_tree" "$update_worker" <<'PY'
import hashlib,json,pathlib,sys
root=pathlib.Path(sys.argv[1]); previous=json.loads((root/'previous-state.json').read_text())
manifest={'source_commit':sys.argv[2],'source_tree':sys.argv[3], 'previous_commit':previous['source_commit'],
          'base_image_id':previous['image_id'],'binary_sha256':hashlib.sha256((root/'sub2api').read_bytes()).hexdigest(),
          'binary_source_commit':sys.argv[4], 'binary_source_tree':sys.argv[5], 'update_worker':sys.argv[6]=='true'}
migrations=root.parents[1]/'upstream/sub2api/backend/migrations'
manifest['migration_checksums']={p.name:hashlib.sha256(p.read_text().strip().encode()).hexdigest() for p in sorted(migrations.glob('*.sql')) if p.read_text().strip()}
digest=hashlib.sha256()
for name, checksum in sorted(manifest['migration_checksums'].items()):
    digest.update((name+'\0'+checksum+'\n').encode())
manifest['migration_set_sha256']=digest.hexdigest()
(root/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
PY
[[ -z $(git status --porcelain) && "$commit" == $(git rev-parse HEAD) ]] || fail 'source changed during build'
cp "$root/infra/security/chromium-seccomp.json" "$staging/chromium-seccomp.json"
cp "$root/ops/deploy-sub2api-test-station-api.py" "$staging/deploy.py"
tar -czf "$staging/bundle.tar.gz" -C "$staging" sub2api manifest.json deploy.py chromium-seccomp.json
archive_sha=$(shasum -a 256 "$staging/bundle.tar.gz" | awk '{print $1}')
remote=$(ssh -T "${ssh_opts[@]}" sub2api-test-station 'mktemp -d /var/tmp/sub2api-test-station-api.XXXXXX')
[[ "$remote" =~ ^/var/tmp/sub2api-test-station-api\.[a-zA-Z0-9]+$ ]] || fail 'unsafe remote staging path'
printf '%s\n' "$remote" >"$staging/remote-staging-path"
printf 'test_station_api_release stage=transfer\n'
scp -q "${ssh_opts[@]}" "$staging/bundle.tar.gz" "sub2api-test-station:$remote/bundle.tar.gz"
ssh -T "${ssh_opts[@]}" sub2api-test-station \
  "test \"\$(sha256sum '$remote/bundle.tar.gz' | cut -d' ' -f1)\" = '$archive_sha' && tar -xzf '$remote/bundle.tar.gz' -C '$remote' && sudo -n python3 '$remote/deploy.py' deploy '$remote'"
printf 'test_station_api_release status=promoted verification_required=true staging=%s\n' "$staging"
