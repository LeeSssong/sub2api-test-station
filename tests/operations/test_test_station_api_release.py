import importlib.util
import unittest
import contextlib
import io
import json
import tempfile
import subprocess
import os
from unittest.mock import patch
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('release', ROOT / 'ops/deploy-sub2api-test-station-api.py')
release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release)


class ReleaseScopeTests(unittest.TestCase):
    def check_scope(self, paths, update_worker=False, deleted_migration=False):
        shell = (ROOT / 'ops/release-sub2api-test-station-api.sh').read_text()
        gate = shell[shell.index('while IFS= read -r path; do'):shell.index('binary_commit=$commit')]
        script = 'fail(){ echo "$1" >&2; exit 1; }; git(){ if [[ "$*" == *--diff-filter=D* ]]; then printf "%s\\n" "$DELETED_MIGRATION"; else printf "%s\\n" "$CHANGED_PATHS"; fi; }; ' + gate
        return subprocess.run(['bash', '-c', script], capture_output=True, text=True,
            env=dict(os.environ, CHANGED_PATHS='\n'.join(paths), update_worker=str(update_worker).lower(), previous='old', root=str(ROOT), DELETED_MIGRATION='upstream/sub2api/backend/migrations/266_quality_traffic_snapshots.sql' if deleted_migration else ''))

    def test_frontend_design_rules_allow_api_only_release_without_widening_runtime_scope(self):
        result = self.check_scope(['upstream/sub2api/frontend/DESIGN.md',
                                   'upstream/sub2api/frontend/src/components/layout/AppSidebar.vue'])
        self.assertEqual(result.returncode, 0, result.stderr)
        for path in ['package.json', 'pnpm-lock.yaml', 'vite.config.ts']:
            with self.subTest(path=path):
                self.assertNotEqual(self.check_scope(['upstream/sub2api/frontend/' + path]).returncode, 0)

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

    def test_native_group_catalog_changes_allow_api_only_release(self):
        paths = ['internal/handler/' + name for name in (
            'api_key_handler.go', 'gateway_handler.go', 'gateway_user_models.go',
            'gateway_model_catalog.go', 'gateway_user_models_test.go', 'api_key_available_groups_tools_test.go')]
        paths += ['internal/service/group_tool_mapping.go', 'internal/service/api_key_group_tool_mapping_test.go']
        result = self.check_scope(['upstream/sub2api/backend/' + path for path in paths])
        self.assertEqual(result.returncode, 0, result.stderr)

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
