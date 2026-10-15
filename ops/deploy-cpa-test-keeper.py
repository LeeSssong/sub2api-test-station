#!/usr/bin/env python3
"""Install Keeper on the existing test CPA without restarting its gateway.
Run as root on 43.133.75.82 with a verified clean-main bundle and plugin ZIP.
"""
import fcntl
import hashlib
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import time
import urllib.request
import zipfile

ROOT = Path('/opt/cpa-test-station')
PUBLIC = 'https://cpa-test.xingqiaolab.top'
EDGE = 'cpa-test-station-edge-cpa-edge-1'
KEEPER = 'cpa-test-station-keeper-cpa-usage-keeper-1'


def run(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT).strip()


def atomic(path, data, mode=0o600):
    temp = path.with_name(path.name + '.keeper-new')
    with open(temp, 'xb') as stream:
        os.chmod(temp, mode)
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temp, path)


def request(url, key=None, body=None, method=None, opener=None, headers=None):
    hdr = dict(headers or {})
    if key:
        hdr['Authorization'] = 'Bearer ' + key
    if body is not None:
        hdr['Content-Type'] = 'application/json'
    req = urllib.request.Request(url, data=None if body is None else json.dumps(body).encode(), headers=hdr, method=method)
    with (opener or urllib.request.build_opener()).open(req, timeout=15) as resp:
        raw = resp.read()
        return json.loads(raw) if raw and 'json' in resp.headers.get('Content-Type', '') else raw


def main():
    started = time.monotonic()
    bundle, plugin_zip, commit, tree = sys.argv[1:]
    bundle = Path(bundle)
    assert os.geteuid() == 0
    locks = []
    for path in ['/opt/sub2api-test-station/.api-release.lock', str(ROOT / '.keeper-release.lock')]:
        lock = open(path, 'a')
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        locks.append(lock)
    source = json.loads((bundle / 'keeper-source.json').read_text())
    assert hashlib.sha256((ROOT / 'edge/Caddyfile').read_bytes()).hexdigest() == source['previous_caddy_sha256'], 'Live edge configuration changed; review before deployment'
    assert hashlib.sha256(Path(plugin_zip).read_bytes()).hexdigest() == source['plugin_sha256']
    assert source['image'] in json.loads(run('docker', 'image', 'inspect', source['image'], '--format', '{{json .RepoDigests}}'))
    key = (ROOT / 'secrets/cpa-management-key').read_text().strip()
    manager_key = (ROOT / 'secrets/cpamp-admin-key').read_text().strip()
    manager = request('http://127.0.0.1:18317/status', manager_key)
    assert manager['collector']['transport'] == 'subscribe'
    plugins = request('http://127.0.0.1:8317/v0/management/plugins', key)
    assert not any(p['id'] == 'keeper' for p in plugins['plugins']), 'Keeper already exists; review before upgrade'
    plugin_path = ROOT / 'cliproxyapi/plugins/keeper.so'
    assert not plugin_path.exists()
    compose = ROOT / 'compose.keeper.yaml'
    assert not compose.exists(), 'Existing deployment must not be overwritten'
    before = {name: run('docker', 'inspect', name, '--format', '{{.State.StartedAt}}') for name in ['cpa-test-station-cpa-gateway-1', 'cpa-test-station-cpa-manager-1']}
    stamp = time.strftime('%Y%m%dT%H%M%SZ', time.gmtime())
    backup = ROOT / 'backups' / ('keeper-' + stamp)
    backup.mkdir(parents=True, mode=0o700)
    shutil.copy2(ROOT / 'edge/Caddyfile', backup / 'Caddyfile')
    shutil.copy2(ROOT / 'cliproxyapi/config.yaml', backup / 'config.yaml')
    os.chmod(backup / 'config.yaml', 0o600)
    timings = {}
    report = dict(target='43.133.75.82', source_commit=commit, source_tree=tree, image=source['image'], plugin_version=source['plugin_version'], plugin_sha256=source['plugin_sha256'], backup=str(backup), phases=timings, production='not queried or changed')
    route_changed = plugin_changed = service_started = False
    try:
        env = ROOT / 'secrets/keeper.env'
        assert not env.exists(), 'Existing Keeper secrets must not be overwritten'
        atomic(env, ('CPA_MANAGEMENT_KEY=' + key + '\nLOGIN_PASSWORD=' + secrets.token_urlsafe(36) + '\n').encode())
        atomic(compose, (bundle / 'compose.keeper.yaml').read_bytes(), 0o644)
        atomic(ROOT / 'keeper-source.json', (bundle / 'keeper-source.json').read_bytes(), 0o644)
        run('docker', 'compose', '-f', str(compose), 'config', '--quiet')
        t = time.monotonic()
        service_started = True
        print('Starting isolated Keeper service', flush=True)
        print(run('docker', 'compose', '-f', str(compose), 'up', '-d', '--wait', '--wait-timeout', '90'), flush=True)
        timings['keeper_ready_seconds'] = round(time.monotonic() - t, 2)
        candidate = ROOT / 'edge/Caddyfile.keeper-candidate'
        atomic(candidate, (bundle / 'Caddyfile').read_bytes(), 0o644)
        run('docker', 'exec', EDGE, 'caddy', 'validate', '--config', '/etc/caddy/Caddyfile.keeper-candidate', '--adapter', 'caddyfile')
        route_changed = True
        os.replace(candidate, ROOT / 'edge/Caddyfile')
        run('docker', 'exec', EDGE, 'caddy', 'reload', '--config', '/etc/caddy/Caddyfile', '--adapter', 'caddyfile')
        assert request(PUBLIC + '/keeper/healthz')['status'] == 'ok'
        print('Public Keeper route healthy', flush=True)
        with zipfile.ZipFile(plugin_zip) as archive:
            assert archive.namelist() == ['keeper.so']
            atomic(plugin_path, archive.read('keeper.so'), 0o755)
        plugin_changed = True
        request('http://127.0.0.1:8317/v0/management/plugins/keeper/config', key, {'enabled': True, 'priority': 1, 'keeper_url': PUBLIC + '/keeper/'}, 'PATCH')
        deadline = time.monotonic() + 45
        while True:
            state = request('http://127.0.0.1:8317/v0/management/plugins', key)
            keeper = next((p for p in state['plugins'] if p['id'] == 'keeper'), {})
            if keeper.get('registered') and keeper.get('effective_enabled'):
                break
            assert time.monotonic() < deadline, 'Plugin hot-load did not finish'
            time.sleep(1)
        resource = request(PUBLIC + '/v0/resource/plugins/keeper/open', manager_key)
        assert b'embed=cpamc' in resource and b'/keeper/' in resource
        password = next(line.split('=', 1)[1] for line in env.read_text().splitlines() if line.startswith('LOGIN_PASSWORD='))
        browser = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        headers = {'X-CPA-Usage-Keeper-Request': 'fetch', 'Origin': PUBLIC}
        request(PUBLIC + '/keeper/api/v1/auth/login', body={'password': password}, opener=browser, headers=headers)
        session = request(PUBLIC + '/keeper/api/v1/auth/session', opener=browser, headers=headers)
        assert session.get('authenticated') is True
        request(PUBLIC + '/keeper/api/v1/auth/logout', body={}, opener=browser, headers=headers)
        assert request('http://127.0.0.1:18317/status', manager_key)['collector']['transport'] == 'subscribe'
        for name, initial in before.items():
            assert run('docker', 'inspect', name, '--format', '{{.State.StartedAt}}') == initial
        report.update(result='installed', rollback=False, checks=['Keeper health', 'public HTTPS health', 'plugin registered/enabled', 'CPAMP resource route', 'Keeper password login/session/logout', 'CPAMP remains subscribed', 'CPA/CPAMP not restarted'], limitations=['Browser embed and new real request collection require separate verification', 'Existing history is not imported; prediction requires new quota samples', 'Sub billing integration not configured'])
        print('Keeper and CPAMP plugin verified', flush=True)
    except Exception as exc:
        # Restore the route first; keep secrets, images and data for diagnosis.
        if route_changed:
            atomic(ROOT / 'edge/Caddyfile', (backup / 'Caddyfile').read_bytes(), 0o644)
            run('docker', 'exec', EDGE, 'caddy', 'reload', '--config', '/etc/caddy/Caddyfile', '--adapter', 'caddyfile')
        if plugin_changed:
            request('http://127.0.0.1:8317/v0/management/plugins/keeper/config', key, {'enabled': False}, 'PATCH')
        if service_started:
            run('docker', 'compose', '-f', str(compose), 'stop')
        report.update(result='failed', rollback=True, error_type=type(exc).__name__)
        raise
    finally:
        timings['total_seconds'] = round(time.monotonic() - started, 2)
        for filename in ['Caddyfile', 'compose.keeper.yaml', 'keeper-source.json']:
            report.setdefault('file_sha256', {})[filename] = hashlib.sha256((bundle / filename).read_bytes()).hexdigest()
        atomic(backup / 'release.json', (json.dumps(report, indent=2) + '\n').encode())
        print('Release record: ' + str(backup / 'release.json'), flush=True)


if __name__ == '__main__':
    main()
