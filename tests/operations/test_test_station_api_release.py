import importlib.util
import unittest
import contextlib
import io
import json
import tempfile
import subprocess
import os
import tarfile
from unittest.mock import patch
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('release', ROOT / 'ops/deploy-sub2api-test-station-api.py')
release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release)


class CanonicalTestStationBaselineTests(unittest.TestCase):
    def check_bridge(self, previous='c67f21bc79010fa2a8e04ccd8c79291274ddf82e',
                     worker=True, previous_tree='f4ad3ab7f12d47c57cbb79ad05f4f52fa225a473',
                     canonical_tree='2698b179f4ba7bc762b24ae26cc4d3052334e6e3', ancestor=0):
        shell = (ROOT / 'ops/release-sub2api-test-station-api.sh').read_text()
        self.assertTrue('# BEGIN canonical test-station baseline' in shell, 'canonical baseline gate missing')
        bridge = shell.split('# BEGIN canonical test-station baseline', 1)[1].split(
            '# END canonical test-station baseline', 1)[0]
        script = """
fail(){ echo "$1" >&2; exit 1; }
git(){ case "$1" in rev-parse) echo "$CANONICAL_TREE";; merge-base) return "$ANCESTOR";; *) return 1;; esac; }
scope_base=$previous
""" + bridge + """
printf '%s' "$scope_base"
"""
        return subprocess.run(['bash', '-eu', '-c', script], capture_output=True, text=True,
            env=dict(os.environ, previous=previous, update_worker=str(worker).lower(),
                     previous_tree=previous_tree, CANONICAL_TREE=canonical_tree, ANCESTOR=str(ancestor)))

    def test_reviewed_external_predecessor_uses_requested_canonical_main(self):
        result = self.check_bridge()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, '2a366ba1a0860a1f97295b33739c6dff26cb5f22')

    def test_bridge_rejects_wrong_identity_or_missing_worker_replacement(self):
        for args in [dict(worker=False), dict(previous_tree='0' * 40),
                     dict(canonical_tree='0' * 40), dict(ancestor=1)]:
            with self.subTest(args=args):
                self.assertNotEqual(self.check_bridge(**args).returncode, 0)

    def test_other_predecessors_keep_the_existing_scope_gate(self):
        result = self.check_bridge(previous='ordinary-predecessor', worker=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, 'ordinary-predecessor')


class FrontendBuildTests(unittest.TestCase):
    def test_installed_tools_run_locale_types_then_bundle_without_install(self):
        shell = (ROOT / 'ops/release-sub2api-test-station-api.sh').read_text()
        build = shell.split("printf 'test_station_api_release stage=frontend_build\\n'", 1)[1].split(
            "printf 'test_station_api_release stage=embedded_server_build", 1)[0]
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            binaries = root / 'upstream/sub2api/frontend/node_modules/.bin'
            binaries.mkdir(parents=True)
            for name in ('vitest', 'vue-tsc', 'vite'):
                tool = binaries / name
                tool.write_text('#!/bin/sh\n' + f'echo "{name} $*" >> "$BUILD_TRACE"\n')
                tool.chmod(0o755)
            trace = root / 'trace'
            result = subprocess.run(['bash', '-eu', '-c', build], capture_output=True, text=True,
                env=dict(os.environ, root=str(root), BUILD_TRACE=str(trace)))
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(trace.read_text().splitlines(), [
                'vitest run src/i18n/__tests__/localeKeyCompleteness.spec.ts',
                'vue-tsc -b', 'vite build'])


class ReleaseBundleTests(unittest.TestCase):
    def test_bundle_contains_resources_required_by_runtime_image(self):
        shell = (ROOT / 'ops/release-sub2api-test-station-api.sh').read_text()
        packaging = shell[shell.index('cp "$root/infra/security/chromium-seccomp.json"'):shell.index('archive_sha=')]
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            staging = root / 'staging'
            staging.mkdir()
            inputs = {
                'infra/security/chromium-seccomp.json': '{}',
                'ops/deploy-sub2api-test-station-api.py': '# deploy',
                'upstream/sub2api/backend/resources/model-pricing/prices.json': '{"test": 1}',
            }
            for name, content in inputs.items():
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(content)
            for name in ('sub2api', 'manifest.json'):
                (staging / name).write_text(name)
            result = subprocess.run(['bash', '-eu', '-c', packaging], capture_output=True, text=True,
                env=dict(os.environ, root=str(root), staging=str(staging)))
            self.assertEqual(result.returncode, 0, result.stderr)
            with tarfile.open(staging / 'bundle.tar.gz') as bundle:
                resource = 'resources/model-pricing/prices.json'
                self.assertIn(resource, bundle.getnames())
                self.assertEqual(bundle.extractfile(resource).read(), b'{"test": 1}')


class ReleaseScopeTests(unittest.TestCase):
    def check_scope(self, paths, update_worker=False, deleted_migration=False):
        shell = (ROOT / 'ops/release-sub2api-test-station-api.sh').read_text()
        gate = shell[shell.index('while IFS= read -r path; do'):shell.index('binary_commit=$commit')]
        script = 'fail(){ echo "$1" >&2; exit 1; }; git(){ if [[ "$*" == *--diff-filter=D* ]]; then printf "%s\\n" "$DELETED_MIGRATION"; else printf "%s\\n" "$CHANGED_PATHS"; fi; }; ' + gate
        return subprocess.run(['bash', '-c', script], capture_output=True, text=True,
            env=dict(os.environ, CHANGED_PATHS='\n'.join(paths), update_worker=str(update_worker).lower(), previous='old', root=str(ROOT), DELETED_MIGRATION='upstream/sub2api/backend/migrations/266_quality_traffic_snapshots.sql' if deleted_migration else ''))

    def test_004_native_modes_scope_requires_worker_and_excludes_neighbors(self):
        paths = ['upstream/sub2api/backend/cmd/server/wire_gen.go', 'upstream/sub2api/backend/internal/handler/channel_monitor_user_handler.go', 'upstream/sub2api/backend/internal/handler/channel_monitor_v2_handler.go', 'upstream/sub2api/backend/internal/handler/channel_monitor_v2_handler_test.go', 'upstream/sub2api/backend/internal/handler/channel_monitor_v3_handler.go', 'upstream/sub2api/backend/internal/handler/channel_monitor_v3_handler_test.go', 'upstream/sub2api/backend/internal/service/channel_monitor_aggregator.go', 'upstream/sub2api/backend/internal/service/channel_monitor_types.go', 'upstream/sub2api/backend/internal/service/channel_monitor_v3.go', 'upstream/sub2api/backend/internal/handler/channel_monitor_sample_presence_test.go', 'upstream/sub2api/backend/internal/handler/channel_monitor_tool_scope.go', 'upstream/sub2api/backend/internal/handler/channel_monitor_tool_scope_test.go', 'upstream/sub2api/backend/internal/handler/channel_monitor_v2_tool_scope_test.go', 'upstream/sub2api/backend/internal/service/channel_monitor_sample_presence_test.go']
        for path in paths:
            with self.subTest(path=path):
                self.assertNotEqual(self.check_scope([path]).returncode, 0)
                result = self.check_scope([path], True)
                self.assertEqual(result.returncode, 0, result.stderr)
        for path in (
            "upstream/sub2api/backend/internal/handler/channel_monitor_unreviewed.go",
            "upstream/sub2api/backend/internal/service/channel_monitor_persistence.go",
            "upstream/sub2api/backend/migrations/289_unreviewed.sql",
        ):
            self.assertNotEqual(self.check_scope([path], True).returncode, 0)

    def test_frontend_design_rules_allow_api_only_release_without_widening_runtime_scope(self):
        result = self.check_scope(['upstream/sub2api/frontend/DESIGN.md',
                                   'upstream/sub2api/frontend/src/components/layout/AppSidebar.vue'])
        self.assertEqual(result.returncode, 0, result.stderr)
        for path in ['package.json', 'pnpm-lock.yaml', 'vite.config.ts']:
            with self.subTest(path=path):
                self.assertNotEqual(self.check_scope(['upstream/sub2api/frontend/' + path]).returncode, 0)

    def test_payment_visibility_fix_allows_api_only_release(self):
        paths = ['upstream/sub2api/backend/internal/service/' + name for name in (
            'payment_config_limits.go', 'payment_config_limits_test.go',
            'payment_visible_method_instances.go')]
        result = self.check_scope(paths)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_snapshot_removal_requires_worker_and_only_deleted_migration(self):
        paths = ['upstream/sub2api/backend/internal/repository/usage_log_repo_insert.go',
                 'upstream/sub2api/backend/ent/schema/usage_log.go',
                 'upstream/sub2api/backend/internal/service/quality_traffic.go']
        self.assertNotEqual(self.check_scope(paths).returncode, 0)
        self.assertEqual(self.check_scope(paths, True).returncode, 0)
        migration = ['upstream/sub2api/backend/migrations/266_quality_traffic_snapshots.sql']
        self.assertNotEqual(self.check_scope(migration, True).returncode, 0)
        self.assertNotEqual(self.check_scope(migration, False, True).returncode, 0)
        self.assertEqual(self.check_scope(migration, True, True).returncode, 0)
        self.assertNotEqual(self.check_scope(['upstream/sub2api/backend/migrations/999.sql'], True, True).returncode, 0)

    def test_gemini_tool_mapping_candidate_allows_api_only_without_dependencies(self):
        paths = [
            'upstream/sub2api/backend/internal/service/group_tool_mapping.go',
            'upstream/sub2api/backend/internal/service/group_tool_mapping_test.go',
            'upstream/sub2api/frontend/public/xingqiao/providers/gemini.svg',
            'upstream/sub2api/frontend/src/features/ai-tools/ToolCarousel.vue',
            'upstream/sub2api/frontend/src/styles/xingqiao-ai.css',
        ]
        result = self.check_scope(paths)
        self.assertEqual(result.returncode, 0, result.stderr)
        for path in ['upstream/sub2api/frontend/pnpm-lock.yaml',
                     'upstream/sub2api/backend/internal/service/billing_service.go']:
            self.assertNotEqual(self.check_scope([path]).returncode, 0)

    def test_native_group_catalog_changes_allow_api_only_release(self):
        paths = ['internal/handler/' + name for name in (
            'api_key_handler.go', 'gateway_handler.go', 'gateway_user_models.go',
            'gateway_model_catalog.go', 'gateway_user_models_test.go', 'api_key_available_groups_tools_test.go')]
        paths += ['internal/service/group_tool_mapping.go', 'internal/service/api_key_group_tool_mapping_test.go']
        result = self.check_scope(['upstream/sub2api/backend/' + path for path in paths])
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_005_line_metrics_read_only_api_files_allow_api_only_release(self):
        reviewed = [
            'internal/handler/monitor_v4_line_metrics_test.go',
            'internal/repository/monitor_v4_line_metrics.go',
            'internal/repository/monitor_v4_line_metrics_test.go',
            'internal/repository/monitor_v4_line_metrics_postgres_test.go',
            'internal/server/routes/monitor_v4_line_metrics_test.go',
            'internal/service/monitor_v4_line_metrics.go',
            'internal/service/monitor_v4_line_metrics_test.go',
        ]
        paths = ['upstream/sub2api/backend/' + path for path in reviewed]
        result = self.check_scope(paths)
        self.assertEqual(result.returncode, 0, result.stderr)
        for path in [
            'upstream/sub2api/backend/internal/repository/monitor_v4_line_metrics_write.go',
            'upstream/sub2api/backend/internal/service/monitor_v4_line_metrics_worker.go',
            'upstream/sub2api/frontend/pnpm-lock.yaml',
        ]:
            with self.subTest(path=path):
                self.assertNotEqual(self.check_scope([path]).returncode, 0)

    def test_dependency_migration_and_unreviewed_backend_changes_remain_excluded(self):
        for path in ['go.mod', 'migrations/999.sql', 'internal/handler/auth_handler.go',
                     'internal/service/billing_service.go', 'internal/repository/group_repo.go']:
            with self.subTest(path=path):
                result = self.check_scope(['upstream/sub2api/backend/' + path])
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('unsupported runtime changes', result.stderr)

    def test_native_plaza_price_reuse_allows_api_only_release(self):
        paths = ['internal/handler/wire.go', 'internal/handler/handler_wiring_test.go',
                 'internal/service/model_plaza_service.go', 'internal/service/model_plaza_service_test.go']
        result = self.check_scope(['upstream/sub2api/backend/' + path for path in paths])
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_native_model_popularity_projection_allows_api_only_release(self):
        paths = ['internal/handler/usage_handler.go', 'internal/handler/usage_model_popularity.go',
                 'internal/handler/usage_model_popularity_test.go', 'internal/server/routes/user.go']
        result = self.check_scope(['upstream/sub2api/backend/' + path for path in paths])
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_line_check_rate_limit_allows_only_reviewed_api_files(self):
        paths = ['internal/middleware/line_check_rate_limiter.go',
                 'internal/server/middleware/line_check_rate_limit.go',
                 'internal/server/middleware/panel_rate_limit.go',
                 'internal/server/routes/monitor_v4_check_rate_limit_test.go']
        result = self.check_scope(['upstream/sub2api/backend/' + path for path in paths])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotEqual(self.check_scope([
            'upstream/sub2api/backend/internal/server/middleware/auth.go']).returncode, 0)

    def test_manual_check_admission_changes_allow_api_only_release(self):
        paths = ['upstream/sub2api/backend/internal/service/' + name for name in
                 ('monitor_v4.go', 'monitor_v4_check.go', 'monitor_v4_check_test.go')]
        result = self.check_scope(paths)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotEqual(self.check_scope([
            'upstream/sub2api/backend/internal/service/monitor_v4_runtime.go']).returncode, 0)

    def test_monitor_changes_still_require_worker_update(self):
        paths = ['upstream/sub2api/backend/internal/service/monitor_v4_runtime.go']
        self.assertNotEqual(self.check_scope(paths).returncode, 0)
        self.assertEqual(self.check_scope(paths, True).returncode, 0)

    def test_route_sla_read_queries_allow_api_only_without_widening_worker_scope(self):
        paths = ['internal/service/monitor_v4_timeline.go', 'internal/service/monitor_v4_timeline_test.go',
                 'internal/handler/monitor_v4_handler_test.go', 'internal/handler/monitor_v4_handler.go',
                 'internal/service/monitor_v4_sla_test.go', 'internal/repository/monitor_v4_timeline.go',
                 'internal/repository/ops_repo_dashboard.go', 'internal/repository/ops_sla_sql.go',
                 'internal/repository/route_sla_timeline_postgres_test.go',
                 'scripts/verify_prototype_monitor_postgres.py']
        result = self.check_scope(['upstream/sub2api/backend/' + path for path in paths])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotEqual(self.check_scope([
            'upstream/sub2api/backend/internal/service/monitor_v4_refresh.go']).returncode, 0)
        self.assertNotEqual(self.check_scope([
            'upstream/sub2api/backend/internal/repository/account_monitor_repo.go']).returncode, 0)

    def test_account_probe_event_fix_requires_worker_update(self):
        paths = ['upstream/sub2api/backend/internal/service/' + name for name in (
            'account_test_service.go', 'account_test_service_openai_test.go',
            'account_monitor_probe_test.go', 'account_probe_cost_test.go')]
        self.assertNotEqual(self.check_scope(paths).returncode, 0)
        self.assertEqual(self.check_scope(paths, True).returncode, 0)
        self.assertNotEqual(self.check_scope([
            'upstream/sub2api/backend/internal/service/account_service.go'], True).returncode, 0)

    def test_route_stream_cache_query_and_tests_allow_api_only_release(self):
        paths = ['upstream/sub2api/backend/internal/repository/' + name for name in
                 ('monitor_v4_timeline.go', 'monitor_v4_timeline_test.go', 'route_cache_timeline_postgres_test.go')]
        result = self.check_scope(paths)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotEqual(self.check_scope([
            'upstream/sub2api/backend/internal/repository/account_monitor_repo.go']).returncode, 0)

    def test_brand_icon_http_and_separate_homepage_allow_api_only_release(self):
        paths = ['upstream/sub2api/backend/internal/web/' + name for name in
                 ['embed_on.go', 'embed_test.go', 'favicon.go', 'favicon_test.go']]
        paths += ['homepage/src/domain/branding.ts', 'infra/independent-test-station/Dockerfile.homepage']
        result = self.check_scope(paths)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotEqual(self.check_scope(['infra/independent-test-station/compose.yaml']).returncode, 0)

    def test_production_sync_requires_worker_and_preserves_dependency_gate(self):
        paths = ['upstream/sub2api/backend/internal/repository/intelligence_rules.go',
                 'upstream/sub2api/backend/internal/service/quality_supported_models.go',
                 'upstream/sub2api/backend/migrations/265_monitor_v4_legacy_default.sql',
                 'upstream/sub2api/backend/migrations/deferred/241_remove_monitor_v4_operational_flag.sql',
                 'upstream/sub2api/Dockerfile']
        self.assertNotEqual(self.check_scope(paths).returncode, 0)
        self.assertEqual(self.check_scope(paths, True).returncode, 0)


class MigrationPreflightTests(unittest.TestCase):
    def test_applied_unchanged_migrations_accept_retained_history(self):
        release.verify_migration_checksums({'265.sql': 'a'*64}, '265.sql|'+'a'*64+'\n241.sql|'+'b'*64+'\n')

    def test_missing_or_changed_migration_refuses_before_candidate(self):
        for rows in ['', '265.sql|'+'b'*64+'\n']:
            with self.subTest(rows=rows), self.assertRaises(ValueError):
                release.verify_migration_checksums({'265.sql': 'a'*64}, rows)


class DrainDeadlineTests(unittest.TestCase):
    def test_expired_deadline_does_not_add_another_stop_grace_period(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            target = root / 'releases' / ('a'*40)
            target.mkdir(parents=True)
            (root / 'release-state.json').write_text(json.dumps({'source_commit': 'a'*40}))
            (target / 'deployment.json').write_text(json.dumps(dict(source_commit='a'*40,
                result='promoted', promoted_at=600, previous_api_container='old-api')))
            rows = ' 0: 00000000:1F90 0100007F:C123 01\n'
            with patch.object(release, 'ROOT', root), patch.object(release.time, 'time', return_value=1000), \
                 patch.object(release, 'run', return_value=rows) as commands, contextlib.redirect_stdout(io.StringIO()):
                release.finalize(target)
            self.assertIn(unittest.mock.call(['docker', 'stop', '--time', '0', 'old-api']), commands.call_args_list)


class APIReleaseTests(unittest.TestCase):
    def test_clones_only_api_and_keeps_dependencies_and_old_service(self):
        old = {'name': 'sub2api-test-station', 'services': {
            'test-station-api': {'image': 'old', 'environment': {'SERVER_PROCESS_ROLE': 'api', 'SECRET': 'hidden'},
                'volumes': ['app:/app/data'], 'networks': ['test-station'], 'depends_on': {'worker': {}}},
            'worker': {'image': 'old'}}, 'networks': {'test-station': {'name': 'sub2api-test-station-network'}}}
        new, service = release.candidate_config(old, 'test-station-api', 'new', 'a' * 40)
        self.assertEqual(service, 'test-station-api-green')
        self.assertEqual(new['services']['worker'], old['services']['worker'])
        self.assertEqual(new['services']['test-station-api'], old['services']['test-station-api'])
        api = new['services'][service]
        self.assertEqual(api['volumes'], ['app:/app/data'])
        self.assertEqual(api['environment']['SERVER_PROCESS_ROLE'], 'api')
        self.assertEqual(api['environment']['SUB2API_DEPLOYMENT_COMMIT'], 'a' * 40)
        self.assertNotIn('depends_on', api)
        self.assertEqual(old['services']['test-station-api']['image'], 'old')

    def test_next_slot_and_preserved_network_aliases(self):
        old = {'services': {'test-station-api-green': {'environment': {'SERVER_PROCESS_ROLE': 'api'},
            'networks': {'test-station': {'aliases': ['old-alias']}}}}}
        new, service = release.candidate_config(old, 'test-station-api-green', 'new', 'a'*40)
        self.assertEqual(service, 'test-station-api-blue')
        self.assertNotIn('old-alias', new['services'][service]['networks']['test-station']['aliases'])
        self.assertEqual(old['services']['test-station-api-green']['networks']['test-station']['aliases'], ['old-alias'])

    def test_refuses_worker_role(self):
        with self.assertRaises(ValueError):
            release.candidate_config({'services': {'api': {'environment': {'SERVER_PROCESS_ROLE': 'worker'}}}}, 'api', 'new', 'a'*40)

    def test_capture_storage_isolated_from_the_live_api(self):
        old = {'services': {'test-station-api': {'environment': {
            'SERVER_PROCESS_ROLE': 'api', 'SUB2API_CONTAINER_SLOT': 'standalone'}}}}
        new, service = release.candidate_config(old, 'test-station-api', 'new', 'a'*40)
        self.assertEqual(new['services'][service]['environment']['SUB2API_CONTAINER_SLOT'], service)
        self.assertEqual(old['services']['test-station-api']['environment']['SUB2API_CONTAINER_SLOT'], 'standalone')

    def test_worker_config_preserves_live_credentials_mounts_and_unrelated_services(self):
        live = {'services': {'test-station-worker': {'image': 'old-worker',
            'environment': {'SERVER_PROCESS_ROLE': 'worker', 'SECRET': 'preserved'},
            'volumes': ['isolated:/app/data'], 'depends_on': {'db': {}}},
            'test-station-detector': {'image': 'detector'}}, 'networks': {}}
        candidate = release.worker_config(live, 'new', 'a'*40)
        worker = candidate['services']['test-station-worker']
        self.assertEqual(worker['environment']['SECRET'], 'preserved')
        self.assertEqual(worker['volumes'], ['isolated:/app/data'])
        self.assertEqual(candidate['services']['test-station-detector'], live['services']['test-station-detector'])
        self.assertEqual(live['services']['test-station-worker']['image'], 'old-worker')
        with self.assertRaises(ValueError):
            release.worker_config({'services': {'test-station-worker': {'environment': {'SERVER_PROCESS_ROLE': 'all'}}}}, 'new', 'a'*40)

    def test_unclean_worker_exit_refuses_replacement(self):
        with patch.object(release, 'run'), patch.object(release, 'inspect', return_value={'State': {'Status': 'exited', 'ExitCode': 137}}):
            with self.assertRaises(RuntimeError):
                release.stop_worker('old-worker')

    def test_multiple_workers_fail_closed(self):
        with patch.object(release, 'run', return_value='worker-1\nworker-2\n'):
            with self.assertRaises(ValueError):
                release.active_worker()

    def test_changes_only_exact_caddy_upstream(self):
        source = ':80 {\n reverse_proxy /api/* test-station-api:8080\n # test-station-api:8080 unchanged comment\n reverse_proxy test-station-api:8080\n}\n'
        new = release.change_upstream(source, 'test-station-api', 'test-station-api-green')
        self.assertEqual(new.count('test-station-api-green:8080'), 2)
        self.assertIn('# test-station-api:8080 unchanged comment', new)
        with self.assertRaises(ValueError):
            release.change_upstream(source, 'missing', 'new')

    def test_counts_only_established_server_connections(self):
        rows = ' 0: 00000000:1F90 0100007F:C123 01\n 1: 00000000:1F90 00000000:0000 0A\n 2: 0100007F:C222 0100007F:1F90 01\n'
        self.assertEqual(release.established_connections(rows), 1)


class HostFlowTests(unittest.TestCase):
    def test_maintenance_success_orders_backup_stop_migrate_start(self):
        self.exercise(update_worker=True, maintenance=True)
    def test_maintenance_failed_migration_restores_before_new_writes(self):
        self.exercise(update_worker=True, maintenance=True, fail_native=True)
    def test_maintenance_new_worker_failure_preserves_database(self):
        self.exercise(update_worker=True, maintenance=True, fail_worker=True)
    def test_maintenance_public_failure_preserves_database(self):
        self.exercise(update_worker=True, maintenance=True, fail_probe=True)

    def exercise(self, fail_probe=False, update_worker=False, fail_worker=False, fail_rollback_probe=False, fail_migration=False, maintenance=False, fail_native=False):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / 'station'
            old_release = root / 'releases' / ('b' * 40)
            old_release.mkdir(parents=True)
            old_config = old_release / 'compose.yaml'
            old_config.write_text('{}')
            env = old_release / '.env'
            env.write_text('SECRET=never-print\n')
            env.chmod(0o600)
            caddy = old_release / 'Caddyfile'
            caddy.write_text(':80 {\n reverse_proxy test-station-api:8080\n}\n')
            previous = {'source_commit': 'b' * 40, 'source_tree': 'c' * 40, 'image_id': 'base', 'image_tag': 'old', 'release_dir': str(old_release)}
            if maintenance: previous['migration_set_sha256'] = release.OCT16_OLD
            (root / 'release-state.json').write_text(json.dumps(previous))
            bundle = Path(tmp) / 'bundle'
            bundle.mkdir()
            (bundle / 'chromium-seccomp.json').write_text((ROOT / 'infra/security/chromium-seccomp.json').read_text())
            (bundle / 'sub2api').write_bytes(b'binary')
            (bundle / 'manifest.json').write_text(json.dumps({'source_commit': 'a' * 40, 'source_tree': 'd' * 40,
                'previous_commit': 'b' * 40, 'base_image_id': 'base', 'update_worker': update_worker, 'migration_checksums': {'265.sql': 'a'*64}, 'maintenance': maintenance, 'migration_set_sha256': release.OCT16_NEW if maintenance else 'c'*64, 'binary_sha256': release.hashlib.sha256(b'binary').hexdigest()}))
            config = {'services': {'test-station-api': {'image': 'old', 'environment': {'SERVER_PROCESS_ROLE': 'api'}, 'networks': ['test-station']},
                'test-station-worker': {'image': 'old-worker-image', 'environment': {'SERVER_PROCESS_ROLE': 'worker'}, 'volumes': ['app:/app/data']}, 'test-station-detector': {'image': 'old'}},
                'networks': {'test-station': {'name': 'sub2api-test-station-network'}}}
            old = {'Image': 'base', 'Config': {'Labels': {'com.docker.compose.service': 'test-station-api', 'com.docker.compose.project.config_files': str(old_config)}}}
            worker_started = False
            worker_restored = False
            def invoke(args, data=None):
                nonlocal worker_started, worker_restored
                if fail_native and '-migrate-only' in args: raise RuntimeError('synthetic migration failure')
                if args[-4:] == ["up", "-d", "--no-deps", "test-station-worker"]:
                    worker_started = True
                    worker_restored = "previous-worker-compose.json" in " ".join(args)
                if args[-3:] == ["ps", "-q", "test-station-worker"]:
                    return "new-worker"
                if 'psql' in ' '.join(args):
                    return '' if fail_migration else '265.sql|'+'a'*64+'\n'
                if args[:3] == ['docker', 'ps', '-q']:
                    return 'caddy' if 'label=com.docker.compose.service=test-station-caddy' in args else 'restored-worker' if 'label=com.docker.compose.service=test-station-worker' in args and worker_restored else 'new-worker' if 'label=com.docker.compose.service=test-station-worker' in args and worker_started else 'old-worker' if 'label=com.docker.compose.service=test-station-worker' in args else 'old-api'
                if args[-3:] == ['config', '--format', 'json']:
                    return json.dumps(config)
                if args[:3] == ['docker', 'image', 'inspect']:
                    return 'base' if args[-1] == 'old' else 'worker-base' if args[-1] == 'old-worker-image' else 'new-image'
                if args[-3:] == ['cat', '/etc/caddy/Caddyfile'] or args[-2:] == ['cat', '/etc/caddy/Caddyfile']:
                    return caddy.read_text()
                if args[-3:] == ['ps', '-q', 'test-station-api-green']:
                    return 'new-api'
                if '/proc/net/tcp' in args:
                    return ''
                return ''
            worker = {'Image': 'worker-base', 'Config': {'Labels': {'com.docker.compose.service': 'test-station-worker', 'com.docker.compose.project.config_files': str(old_config)}}, 'State': {'Status': 'exited', 'ExitCode': 0}}
            def inspected(value):
                if value == 'old-api':
                    return old
                if value in ('old-worker', 'restored-worker'):
                    return worker
                if value == 'new-worker':
                    return dict(worker, Image='new-image')
                return {'Mounts': [{'Destination': '/etc/caddy/Caddyfile', 'Source': str(caddy)}]}
            def readiness(value):
                if fail_worker and value == 'new-worker':
                    raise RuntimeError('worker not ready')
            probes = [RuntimeError('offline'), RuntimeError('still offline') if fail_rollback_probe else None] if fail_probe else [None]
            with patch.object(release, 'ROOT', root), patch.object(release, 'run', side_effect=invoke) as commands, \
                 patch.object(release, 'inspect', side_effect=inspected), \
                 patch.object(release, 'healthy', side_effect=readiness), patch.object(release, 'reload_caddy') as reloads, \
                 patch.object(release, 'database_dump', side_effect=lambda db,p: p.write_bytes(b'backup')) as dumps, \
                 patch.object(release, 'validate_restore') as validation, patch.object(release, 'restore_database') as restoration, \
                 patch.object(release, 'public_probes', side_effect=probes), contextlib.redirect_stdout(io.StringIO()):
                if maintenance:
                    if fail_native or fail_worker or fail_probe:
                        with self.assertRaises(RuntimeError): release.deploy(bundle)
                        self.assertEqual(restoration.call_count, 1 if fail_native else 0)
                        status=json.loads((root/'releases'/('a'*40)/'deployment.json').read_text())['result']
                        self.assertEqual(status, 'rolled_back' if fail_native else 'recovery_required')
                    else:
                        release.deploy(bundle)
                        self.assertEqual(validation.call_count,1)
                        self.assertEqual(dumps.call_count,2)
                        steps=[c.args[0] for c in commands.call_args_list]
                        migrate=next(i for i,a in enumerate(steps) if '-migrate-only' in a)
                        stop=steps.index(['docker','stop','--time','60','old-api'])
                        start=next(i for i,a in enumerate(steps) if a[-4:]==['up','-d','--no-deps','test-station-api-green'])
                        self.assertLess(stop,migrate);self.assertLess(migrate,start)
                    return
                if fail_migration:
                    with self.assertRaises(ValueError):
                        release.deploy(bundle)
                    self.assertFalse(any('up' in call.args[0] or 'build' in call.args[0] for call in commands.call_args_list))
                    self.assertEqual(json.loads((root / 'release-state.json').read_text()), previous)
                    self.assertFalse((root / 'releases' / ('a'*40)).exists())
                    return
                if fail_probe or fail_worker:
                    with self.assertRaises(RuntimeError):
                        release.deploy(bundle)
                    restored_state = json.loads((root / 'release-state.json').read_text())
                    self.assertEqual({k:restored_state[k] for k in previous}, previous)
                    if update_worker and not fail_rollback_probe:
                        self.assertEqual(restored_state['active_worker_container'], 'restored-worker')
                    if fail_rollback_probe:
                        self.assertEqual(json.loads((root / 'releases' / ('a'*40) / 'deployment.json').read_text())['rollback_errors'], ['public_readiness'])
                    self.assertEqual(reloads.call_args[0][1], ':80 {\n reverse_proxy test-station-api:8080\n}\n')
                else:
                    release.deploy(bundle)
                    before = list(commands.call_args_list)
                    self.assertFalse(any('stop' in call.args[0] and 'old-api' in call.args[0] for call in before))
                    release.finalize(root / 'releases' / ('a' * 40))
                    self.assertIn(unittest.mock.call(['docker', 'stop', '--time', '10', 'old-api']), commands.call_args_list)
                    self.assertEqual(json.loads((root / 'release-state.json').read_text())['active_api_container'], 'new-api')
                starts = [call.args[0] for call in commands.call_args_list if 'up' in call.args[0]]
                self.assertEqual(starts[0][-4:], ['up', '-d', '--no-deps', 'test-station-api-green'])
                if update_worker:
                    commands_list = [call.args[0] for call in commands.call_args_list]
                    stopped = ['docker', 'kill', '--signal', 'TERM', 'old-worker']
                    self.assertIn(stopped, commands_list)
                    start_worker = next(i for i, args in enumerate(commands_list) if args[-4:] == ['up', '-d', '--no-deps', 'test-station-worker'])
                    self.assertLess(commands_list.index(stopped), start_worker)
                    if fail_probe or fail_worker:
                        self.assertGreaterEqual(len(starts), 3)
                        self.assertTrue(any('previous-worker-compose.json' in ' '.join(args) for args in starts))
                    else:
                        self.assertEqual(len(starts), 2)
                        state = json.loads((root / 'release-state.json').read_text())
                        self.assertFalse(state['api_only_release'])
                        self.assertEqual(state['active_worker_container'], 'new-worker')
                else:
                    self.assertEqual(len(starts), 1)

    def test_pending_migration_refuses_before_build_or_start(self):
        self.exercise(fail_migration=True)

    def test_ready_candidate_then_route_then_drain_without_restarting_jobs(self):
        self.exercise()

    def test_failed_public_probe_restores_old_route_and_state(self):
        self.exercise(fail_probe=True)

    def test_worker_replaced_without_overlap_and_dependencies_stay_untouched(self):
        self.exercise(update_worker=True)

    def test_worker_readiness_failure_restores_previous_worker_and_api(self):
        self.exercise(update_worker=True, fail_worker=True)

    def test_public_failure_restores_previous_worker_and_api(self):
        self.exercise(update_worker=True, fail_probe=True)

    def test_worker_restored_even_when_rollback_public_probe_also_fails(self):
        self.exercise(update_worker=True, fail_probe=True, fail_rollback_probe=True)


if __name__ == '__main__':
    unittest.main()


class V2RequestScopeMigrationTests(unittest.TestCase):
    def test_only_the_exact_new_migration_is_allowed_and_existing_checksums_are_immutable(self):
        previous = {'migration_set_sha256': release.OCT16_NEW}
        manifest = {'update_worker': True, 'migration_set_sha256': 'c'*64,
                    'migration_checksums': {'old.sql': 'a'*64, release.V2_SCOPE_MIGRATION: 'b'*64}}
        rows = 'old.sql|' + 'a'*64
        release.verify_maintenance_migrations(previous, manifest, rows)
        for change in [dict(update_worker=False),
                       dict(migration_checksums={'old.sql': 'd'*64, release.V2_SCOPE_MIGRATION: 'b'*64}),
                       dict(migration_checksums={'old.sql': 'a'*64, '999_other.sql': 'b'*64})]:
            with self.subTest(change=change), self.assertRaises(ValueError):
                release.verify_maintenance_migrations(previous, dict(manifest, **change), rows)

    def test_scope_requires_the_matching_worker(self):
        gate = ReleaseScopeTests()
        paths = ['upstream/sub2api/backend/internal/repository/channel_monitor_v2_aggregation.go',
                 'upstream/sub2api/backend/migrations/287_channel_monitor_v2_request_scope.sql']
        self.assertNotEqual(gate.check_scope(paths).returncode, 0)
        self.assertEqual(gate.check_scope(paths, True).returncode, 0)


class V2ScopedBackupTests(unittest.TestCase):
    def test_dump_includes_only_changed_tables_and_migration_ledger(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / 'monitor.dump'
            def dump(command, stdout=None, **kwargs):
                stdout.write(b'fixture dump')
                self.assertIn('--table', command)
                self.assertIn('public.schema_migrations', command)
                self.assertIn('public.channel_monitor_v2_config', command)
                self.assertNotIn('public.usage_logs', command)
                return subprocess.CompletedProcess(command, 0)
            with patch.object(release.subprocess, 'run', side_effect=dump):
                release.database_dump('postgres', target, release.V2_SCOPE_TABLES)
            self.assertEqual(target.stat().st_mode & 0o777, 0o600)

    def test_scoped_restore_preserves_raw_logs_billing_and_other_schemas(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / 'monitor.dump'; target.write_bytes(b'fixture dump')
            with patch.object(release, 'run') as run:
                release.restore_database('postgres', target, release.V2_SCOPE_TABLES)
            drop = run.call_args_list[0].args[1].decode()
            self.assertIn('DROP TABLE public.channel_monitor_v2_', drop)
            self.assertNotIn('DROP SCHEMA', drop)
            self.assertNotIn('usage_logs', drop)
            self.assertNotIn('billing', drop)
            with self.assertRaises(ValueError):
                release.restore_database('postgres', target, ['usage_logs'])


class V2CardTimelineReleaseTests(unittest.TestCase):
    migration = '288_channel_monitor_v2_card_timeline_mode.sql'
    old_digest = '5e2c3b9c073dae5e2c02950ebd21a68d1fc21b0661f3ad11bfe4df8634f24a9b'
    new_digest = 'e84547d32c582148a4a6dc3c572d497d6060db29f933418cd46681cf834f389d'

    def inputs(self):
        applied = {p.name: release.hashlib.sha256(p.read_text().strip().encode()).hexdigest()
                   for p in sorted((ROOT / 'upstream/sub2api/backend/migrations').glob('*.sql'))
                   if p.name != self.migration and p.read_text().strip()}
        raw = (ROOT / 'upstream/sub2api/backend/migrations' / self.migration).read_text().strip()
        expected = dict(applied, **{self.migration: release.hashlib.sha256(raw.encode()).hexdigest()})
        return ({'migration_set_sha256': self.old_digest},
                {'update_worker': True, 'migration_set_sha256': self.new_digest,
                 'migration_checksums': expected},
                '\n'.join(k + '|' + v for k, v in applied.items()))

    def test_exact_288_transition_allowed_and_other_changes_rejected(self):
        previous, manifest, rows = self.inputs()
        try:
            release.verify_maintenance_migrations(previous, manifest, rows)
        except ValueError as error:
            self.fail('reviewed card timeline transition rejected: ' + str(error))
        for prev, change in [
            (dict(migration_set_sha256='0'*64), {}),
            (previous, dict(update_worker=False)),
            (previous, dict(migration_set_sha256='0'*64)),
            (previous, dict(migration_checksums=dict(manifest['migration_checksums'], **{'999_other.sql':'a'*64}))),
            (previous, dict(migration_checksums={k: ('0'*64 if k=='287_channel_monitor_v2_request_scope.sql' else v)
                                                for k,v in manifest['migration_checksums'].items()})),
        ]:
            with self.subTest(change=change), self.assertRaises(ValueError):
                release.verify_maintenance_migrations(prev, dict(manifest, **change), rows)

    def test_exact_config_and_ledger_backup_without_usage_or_billing(self):
        tables = ['channel_monitor_v2_config', 'schema_migrations']
        with tempfile.TemporaryDirectory() as tmp:
            backup = Path(tmp) / 'config.dump'; backup.write_bytes(b'fixture')
            with patch.object(release, 'run') as commands:
                try:
                    release.restore_database('postgres', backup, tables)
                except ValueError as error:
                    self.fail('reviewed minimal config restore rejected: ' + str(error))
            drop = commands.call_args_list[0].args[1].decode()
            self.assertIn('public.channel_monitor_v2_config', drop)
            self.assertIn('public.schema_migrations', drop)
            self.assertNotIn('DROP SCHEMA', drop)
            self.assertNotIn('usage_logs', drop)
        shell = (ROOT / 'ops/deploy-sub2api-test-station-api.py').read_text()
        self.assertIn('CARD_TIMELINE_TABLES', shell)

    def test_release_scope_includes_exact_288_and_related_tests_only_with_worker(self):
        gate = ReleaseScopeTests()
        paths = ['upstream/sub2api/backend/' + p for p in [
            'migrations/' + self.migration,
            'internal/handler/channel_monitor_v2_postgres_test.go',
            'internal/repository/channel_monitor_v2_config_test.go',
            'internal/repository/channel_monitor_v2_repo_test.go']]
        self.assertEqual(gate.check_scope(paths, True).returncode, 0)
        self.assertNotEqual(gate.check_scope(paths, False).returncode, 0)
        self.assertNotEqual(gate.check_scope(['upstream/sub2api/backend/migrations/999_other.sql'], True).returncode, 0)
