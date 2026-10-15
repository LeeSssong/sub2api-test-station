#!/usr/bin/env python3
"""Isolated API blue/green release with optional serialized worker replacement; no migrations."""
import argparse
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time
import urllib.request

ROOT = Path('/opt/sub2api-test-station')
PROJECT = 'sub2api-test-station'


def candidate_config(config, previous_service, image, commit):
    result = copy.deepcopy(config)
    api = copy.deepcopy(config['services'][previous_service])
    if api.get('environment', {}).get('SERVER_PROCESS_ROLE') != 'api':
        raise ValueError('previous service must have API-only role')
    service = 'test-station-api-blue' if previous_service == 'test-station-api-green' else 'test-station-api-green'
    api['image'] = image
    api['environment']['SUB2API_DEPLOYMENT_COMMIT'] = commit
    api['environment']['SUB2API_CONTAINER_SLOT'] = service
    for field in ('depends_on', 'container_name'):
        api.pop(field, None)
    networks = api.get('networks', {})
    if isinstance(networks, dict):
        for network in networks.values():
            if isinstance(network, dict):
                network['aliases'] = []
    result['services'][service] = api
    return result, service


def change_upstream(config, previous, candidate):
    lines = config.splitlines(keepends=True)
    changed = 0
    for i, line in enumerate(lines):
        if line.lstrip().startswith('reverse_proxy '):
            lines[i], count = re.subn(r'(?<![\w-])' + re.escape(previous) + r':8080(?![\w-])', candidate + ':8080', line)
            changed += count
    if not changed:
        raise ValueError('active Caddy upstream is missing')
    return ''.join(lines)


def established_connections(rows):
    return sum(1 for line in rows.splitlines() if len(line.split()) >= 4
               and line.split()[1].endswith(':1F90') and line.split()[3] == '01')


def worker_config(config, image, commit):
    result = copy.deepcopy(config)
    worker = result['services']['test-station-worker']
    if worker.get('environment', {}).get('SERVER_PROCESS_ROLE') != 'worker':
        raise ValueError('expected a dedicated worker role')
    worker['image'] = image
    worker['environment']['SUB2API_DEPLOYMENT_COMMIT'] = commit
    worker['environment']['SUB2API_CONTAINER_SLOT'] = 'test-station-worker'
    worker['stop_grace_period'] = '300s'
    worker.pop('depends_on', None)
    return result


def active_worker():
    value = run(['docker', 'ps', '-q', '--filter', 'label=com.docker.compose.project=' + PROJECT,
                 '--filter', 'label=com.docker.compose.service=test-station-worker']).strip()
    if not value or '\n' in value:
        raise ValueError('expected exactly one running worker')
    return value


def stop_worker(container, timeout=300):
    # Signal and wait; never SIGKILL a worker or overlap singleton consumers.
    run(['docker', 'kill', '--signal', 'TERM', container])
    deadline = time.monotonic() + timeout
    while True:
        state = inspect(container)['State']
        if state['Status'] == 'exited':
            if state.get('ExitCode') != 0 or state.get('OOMKilled', False):
                raise RuntimeError('worker did not exit cleanly')
            return
        if time.monotonic() >= deadline:
            raise RuntimeError('worker graceful shutdown timed out')
        time.sleep(1)


def load_worker_config(container):
    current = inspect(container)
    labels = current['Config']['Labels']
    path = Path(labels['com.docker.compose.project.config_files'])
    env = path.parent / '.env'
    if not str(path).startswith(str(ROOT / 'releases') + '/') or path.is_symlink() or env.is_symlink() or (env.stat().st_mode & 0o777) != 0o600:
        raise ValueError('unsafe worker config')
    config = json.loads(run(['docker', 'compose', '-p', PROJECT, '--env-file', str(env), '-f', str(path), 'config', '--format', 'json']))
    if config['networks']['test-station']['name'] != PROJECT + '-network':
        raise ValueError('worker network mismatch')
    worker = config['services']['test-station-worker']
    if worker.get('environment', {}).get('SERVER_PROCESS_ROLE') != 'worker':
        raise ValueError('expected a dedicated worker role')
    if run(['docker', 'image', 'inspect', '--format', '{{.Id}}', worker['image']]).strip() != current['Image']:
        raise ValueError('active worker config image mismatch')
    return config, env, current['Image']


def restore_worker(release, meta):
    # On partial replacement the running worker may still be the original one.
    current = run(['docker', 'ps', '-q', '--filter', 'label=com.docker.compose.project=' + PROJECT,
                   '--filter', 'label=com.docker.compose.service=test-station-worker']).strip()
    if '\n' in current:
        raise ValueError('multiple workers during rollback')
    if current and current != meta['previous_worker_container']:
        stop_worker(current)
    compose = ['docker', 'compose', '-p', PROJECT, '--env-file', str(release / 'previous-worker.env'),
               '-f', str(release / 'previous-worker-compose.json')]
    run(compose + ['up', '-d', '--no-deps', 'test-station-worker'])
    restored = active_worker()
    healthy(restored)
    if inspect(restored)['Image'] != meta['previous_worker_image_id']:
        raise RuntimeError('worker rollback image mismatch')
    return restored


def run(args, data=None):
    value = subprocess.run(args, input=data, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if value.returncode:
        # Compose/inspect outputs may contain secrets; never include them in errors.
        raise RuntimeError('command failed: ' + args[0] + ' (exit ' + str(value.returncode) + ')')
    return value.stdout.decode()


def inspect(container):
    return json.loads(run(['docker', 'inspect', container]))[0]


def private_write(path, data):
    path = Path(path)
    tmp = path.with_name('.' + path.name + '.new')
    fd = os.open(str(tmp), os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, 'w') as f:
        f.write(data)
        f.flush()
        os.fsync(f.fileno())
    os.replace(tmp, path)


def save(path, data):
    private_write(path, json.dumps(data, indent=2) + '\n')


def healthy(container, timeout=120):
    deadline = time.monotonic() + timeout
    while True:
        status = inspect(container)['State']
        if status.get('Health', {}).get('Status') == 'healthy':
            return
        if status.get('Status') not in ('running', 'created') or time.monotonic() >= deadline:
            raise RuntimeError('candidate readiness failed')
        time.sleep(1)


def public_probes():
    for path, expected in [('/health', 'ok'), ('/readyz', 'ready')]:
        with urllib.request.urlopen('http://127.0.0.1' + path, timeout=10) as response:
            if json.load(response) != {'status': expected}:
                raise RuntimeError('public readiness failed')


def reload_caddy(container, config):
    run(['docker', 'exec', '-i', container, 'caddy', 'validate', '--config', '-', '--adapter', 'caddyfile'], config.encode())
    run(['docker', 'exec', '-i', container, 'caddy', 'reload', '--config', '-', '--adapter', 'caddyfile'], config.encode())


def rollback(release):
    previous = json.loads((release / 'previous-state.json').read_text())
    meta = json.loads((release / 'deployment.json').read_text())
    errors = []
    config = (release / 'previous-Caddyfile').read_text()
    try:
        run(['docker', 'start', meta['previous_api_container']])
        healthy(meta['previous_api_container'])
        reload_caddy(meta['caddy_container'], config)
        private_write(meta['caddy_host_path'], config)
    except Exception:
        errors.append('api_route')
    # Always attempt worker recovery, even if route recovery or probes fail.
    if meta.get('worker_update_started'):
        try:
            restored = restore_worker(release, meta)
            meta['restored_worker_container'] = restored
            previous.update(active_worker_container=restored, worker_image_id=meta['previous_worker_image_id'])
        except Exception:
            errors.append('worker')
    try:
        public_probes()
    except Exception:
        errors.append('public_readiness')
    # Keep failed candidate alive for in-flight connections; never kill it at cutback.
    if errors:
        meta.update(result='rollback_failed', rolled_back=False, rollback_errors=errors)
        save(release / 'deployment.json', meta)
        raise RuntimeError('rollback failed stages=' + ','.join(errors))
    save(ROOT / 'release-state.json', previous)
    meta.update(result='rolled_back', rolled_back=True)
    save(release / 'deployment.json', meta)
    print('test_station_api status=rolled_back', flush=True)


def verify_migration_checksums(expected, rows):
    if not isinstance(expected, dict) or not expected:
        raise ValueError('migration checksums required')
    applied = {}
    for row in rows.splitlines():
        name, checksum = row.split('|', 1)
        applied[name] = checksum
    for name, checksum in expected.items():
        if not re.fullmatch(r'[a-f0-9]{64}', checksum) or applied.get(name) != checksum:
            raise ValueError('migration not applied with expected checksum: ' + name)


def deploy(bundle):
    manifest = json.loads((bundle / 'manifest.json').read_text())
    commit, tree = manifest['source_commit'], manifest['source_tree']
    if not all(re.fullmatch('[a-f0-9]{40}', value) for value in (commit, tree)):
        raise ValueError('invalid source identity')
    update_worker = manifest.get('update_worker', False)
    if not isinstance(update_worker, bool):
        raise ValueError('invalid worker update flag')
    binary = bundle / 'sub2api'
    if binary.is_symlink() or hashlib.sha256(binary.read_bytes()).hexdigest() != manifest['binary_sha256']:
        raise ValueError('binary checksum mismatch')
    previous = json.loads((ROOT / 'release-state.json').read_text())
    if previous['source_commit'] != manifest['previous_commit'] or previous['image_id'] != manifest['base_image_id']:
        raise ValueError('active release changed')
    api_id = previous.get('active_api_container') or run(['docker', 'ps', '-q', '--filter', 'label=com.docker.compose.project=' + PROJECT,
        '--filter', 'label=com.docker.compose.service=test-station-api']).strip()
    if not api_id or '\n' in api_id:
        raise ValueError('expected exactly one active API')
    old = inspect(api_id)
    if old['Image'] != previous['image_id']:
        raise ValueError('active image mismatch')
    labels = old['Config']['Labels']
    old_service = labels['com.docker.compose.service']
    old_config = Path(labels['com.docker.compose.project.config_files'])
    old_env = old_config.parent / '.env'
    if not str(old_config).startswith(str(ROOT / 'releases') + '/') or old_env.is_symlink() or (old_env.stat().st_mode & 0o777) != 0o600:
        raise ValueError('unsafe active config')
    config = json.loads(run(['docker', 'compose', '-p', PROJECT, '--env-file', str(old_env), '-f', str(old_config), 'config', '--format', 'json']))
    if config['networks']['test-station']['name'] != PROJECT + '-network':
        raise ValueError('network mismatch')
    db_id = run(['docker', 'ps', '-q', '--filter', 'label=com.docker.compose.project=' + PROJECT,
                 '--filter', 'label=com.docker.compose.service=test-station-postgres']).strip()
    if not db_id or '\n' in db_id:
        raise ValueError('expected exactly one isolated database')
    rows = run(['docker', 'exec', '-i', db_id, 'sh', '-c',
                'exec psql -X -v ON_ERROR_STOP=1 -At -U "$POSTGRES_USER" -d "$POSTGRES_DB"'],
               b"SELECT filename || '|' || checksum FROM schema_migrations ORDER BY filename;\n")
    verify_migration_checksums(manifest.get('migration_checksums'), rows)
    caddy_id = run(['docker', 'ps', '-q', '--filter', 'label=com.docker.compose.project=' + PROJECT,
                    '--filter', 'label=com.docker.compose.service=test-station-caddy']).strip()
    if not caddy_id or '\n' in caddy_id:
        raise ValueError('expected exactly one Caddy')
    caddy = inspect(caddy_id)
    caddy_path = next(m['Source'] for m in caddy['Mounts'] if m['Destination'] == '/etc/caddy/Caddyfile')
    if not caddy_path.startswith(str(ROOT / 'releases') + '/'):
        raise ValueError('unsafe Caddy path')
    caddy_config = Path(caddy_path).read_text()
    image = PROJECT + '-runtime:' + commit
    candidate, service = candidate_config(config, old_service, image, commit)
    previous_worker_config = None
    if update_worker:
        worker_id = active_worker()
        previous_worker_config, worker_env, worker_image_id = load_worker_config(worker_id)
        # Read the live worker's own Compose config, not a stale API release copy.
        candidate['services']['test-station-worker'] = worker_config(previous_worker_config, image, commit)['services']['test-station-worker']
    new_caddy = change_upstream(caddy_config, old_service, service)
    release = ROOT / 'releases' / commit
    release.mkdir(mode=0o700)
    save(release / 'previous-state.json', previous)
    private_write(release / 'previous-Caddyfile', caddy_config)
    private_write(release / '.env', old_env.read_text())
    private_write(release / 'chromium-seccomp.json', (bundle / 'chromium-seccomp.json').read_text())
    for name in [service] + (['test-station-worker'] if update_worker else []):
        candidate['services'][name]['security_opt'] = ['seccomp=' + str(release / 'chromium-seccomp.json')]
    save(release / 'compose.yaml', candidate)
    if update_worker:
        save(release / 'previous-worker-compose.json', previous_worker_config)
        private_write(release / 'previous-worker.env', worker_env.read_text())
    base_tag = previous['image_tag']
    if run(['docker', 'image', 'inspect', '--format', '{{.Id}}', base_tag]).strip() != manifest['base_image_id']:
        raise ValueError('base dependency identity mismatch')
    (bundle / 'Dockerfile').write_text('FROM ' + base_tag + '\nUSER root\nRUN apk add --no-cache chromium font-noto-cjk\nENV INTELLIGENCE_CHROMIUM_PATH=/usr/bin/chromium-browser\nUSER 1000:1000\nCOPY --chown=1000:1000 --chmod=0555 sub2api /app/sub2api\nLABEL org.opencontainers.image.revision="' + commit + '"\n')
    started = time.monotonic()
    run(['docker', 'build', '--pull=false', '-t', image, str(bundle)])
    image_id = run(['docker', 'image', 'inspect', '--format', '{{.Id}}', image]).strip()
    meta = {'source_commit': commit, 'source_tree': tree, 'base_image_id': manifest['base_image_id'], 'image_id': image_id,
        'binary_source_commit': manifest.get('binary_source_commit', commit),
        'binary_source_tree': manifest.get('binary_source_tree', tree),
        'previous_api_container': api_id, 'previous_api_service': old_service, 'candidate_service': service,
        'caddy_container': caddy_id, 'caddy_host_path': caddy_path, 'result': 'prepared', 'rolled_back': False, 'stage_seconds': {}}
    if update_worker:
        meta.update(previous_worker_container=worker_id, previous_worker_image_id=worker_image_id, worker_update_started=False)
    save(release / 'deployment.json', meta)
    compose = ['docker', 'compose', '-p', PROJECT, '--env-file', str(release / '.env'), '-f', str(release / 'compose.yaml')]
    try:
        run(compose + ['config', '--quiet'])
        run(compose + ['up', '-d', '--no-deps', service])
        candidate_id = run(compose + ['ps', '-q', service]).strip()
        healthy(candidate_id)
        meta['candidate_container'] = candidate_id
        meta['stage_seconds']['build_and_ready'] = round(time.monotonic() - started, 2)
        run(['docker', 'exec', candidate_id, 'wget', '-q', '-O', '/dev/null', 'http://127.0.0.1:8080/readyz'])
        if update_worker:
            worker_started = time.monotonic()
            meta['worker_update_started'] = True
            save(release / 'deployment.json', meta)
            stop_worker(worker_id)
            run(compose + ['up', '-d', '--no-deps', 'test-station-worker'])
            new_worker = active_worker()
            healthy(new_worker)
            meta['candidate_worker_container'] = new_worker
            meta['stage_seconds']['worker_replace'] = round(time.monotonic() - worker_started, 2)
        cutover_at = time.time()
        reload_caddy(caddy_id, new_caddy)
        public_probes()
        private_write(caddy_path, new_caddy)
        private_write(release / 'Caddyfile', new_caddy)
        meta['result'] = 'promoted'
        meta['promoted_at'] = cutover_at
        save(release / 'deployment.json', meta)
        state = dict(previous, source_commit=commit, source_tree=tree, image_id=image_id, image_tag=image,
            release_dir=str(release), previous_release_dir=previous['release_dir'], active_api_container=candidate_id,
            migration_set_sha256=manifest['migration_set_sha256'],
            active_api_service=service, api_only_release=not update_worker, binary_sha256=manifest['binary_sha256'], result='succeeded', rolled_back=False)
        if update_worker:
            state['active_worker_container'] = new_worker
            state['worker_image_id'] = image_id
        state['binary_source_commit'] = manifest.get('binary_source_commit', commit)
        state['binary_source_tree'] = manifest.get('binary_source_tree', tree)
        state.pop('image_archive_sha256', None)
        state['updated_at'] = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
        save(ROOT / 'release-state.json', state)
        print('test_station_api status=promoted source_commit=' + commit + ' image_id=' + image_id, flush=True)
    except Exception:
        rollback(release)
        raise
    # The caller verifies the actual page before finalizing connection drain.
    print('test_station_api verification_pending=true release_dir=' + str(release), flush=True)


def finalize(release):
    meta = json.loads((release / 'deployment.json').read_text())
    state = json.loads((ROOT / 'release-state.json').read_text())
    if state['source_commit'] != meta['source_commit'] or meta['result'] != 'promoted':
        raise ValueError('not the current promoted release')
    start = time.monotonic()
    deadline = meta['promoted_at'] + 300
    remaining = 0
    while True:
        rows = run(['docker', 'exec', meta['previous_api_container'], 'cat', '/proc/net/tcp', '/proc/net/tcp6'])
        remaining = established_connections(rows)
        elapsed = time.monotonic() - start
        if not remaining or time.time() >= deadline:
            break
        time.sleep(min(2, max(0, deadline - time.time())))
    stop_grace = max(0, min(10, int(deadline - time.time())))
    run(['docker', 'stop', '--time', str(stop_grace), meta['previous_api_container']])
    meta['result'] = 'succeeded'
    meta['drain'] = {'seconds': round(time.monotonic() - start, 2), 'remaining_at_deadline': remaining, 'maximum_seconds': 300}
    save(release / 'deployment.json', meta)
    print('test_station_api status=succeeded drain_seconds=' + str(meta['drain']['seconds']), flush=True)


def main():
    p = argparse.ArgumentParser()
    p.add_argument('action', choices=['deploy', 'finalize', 'rollback'])
    p.add_argument('path', type=Path)
    args = p.parse_args()
    if os.geteuid() != 0 or ROOT.is_symlink():
        raise ValueError('protected test-station root required')
    lock = open(ROOT / '.api-release.lock', 'a')
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    path = args.path.resolve()
    if args.action == 'deploy':
        if not str(path).startswith('/var/tmp/sub2api-test-station-api.'):
            raise ValueError('unsafe staging directory')
        deploy(path)
    else:
        if path.parent != ROOT / 'releases':
            raise ValueError('unsafe release directory')
        globals()[args.action](path)


if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print('test_station_api status=failed reason=' + str(error), flush=True)
        raise SystemExit(1)
