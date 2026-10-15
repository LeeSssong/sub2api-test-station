#!/usr/bin/env python3
"""Atomically publish the pinned Keeper pool UI on the existing test edge."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import time
import urllib.request

ROOT = Path('/opt/cpa-test-station')
EDGE = 'cpa-test-station-edge-cpa-edge-1'
PUBLIC = 'https://cpa-test.xingqiaolab.top'


def run(*args):
    return subprocess.check_output(args, stderr=subprocess.STDOUT, text=True).strip()


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def atomic(path, data, mode=0o644):
    temp = path.with_name(path.name + '.pool-next')
    with open(temp, 'xb') as stream:
        os.chmod(temp, mode)
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temp, path)


def fetch(path):
    with urllib.request.urlopen(PUBLIC + path, timeout=15) as response:
        return response.read(), response.headers


def main():
    start = time.monotonic()
    bundle, commit, tree = sys.argv[1:]
    bundle = Path(bundle)
    assert os.geteuid() == 0
    locks = []
    for path in [Path('/opt/sub2api-test-station/.api-release.lock'), ROOT / '.keeper-release.lock']:
        lock = open(path, 'a')
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        locks.append(lock)
    metadata = json.loads((bundle / 'source.json').read_text())
    assert sha(bundle / 'web.tar.gz') == metadata['web_sha256']
    assert sha(bundle / 'upstream.patch') == metadata['patch_sha256']
    caddy = ROOT / 'edge/Caddyfile'
    assert sha(caddy) == metadata['previous_caddy_sha256'], 'Live edge changed; review before publication'
    names = ['cpa-test-station-cpa-gateway-1', 'cpa-test-station-cpa-manager-1', 'cpa-test-station-keeper-cpa-usage-keeper-1']
    before = {name: run('docker', 'inspect', name, '--format', '{{.State.StartedAt}}') for name in names}
    web_health, _ = fetch('/keeper/healthz')
    assert json.loads(web_health)['status'] == 'ok'
    version_root = ROOT / 'edge/keeper-ui' / metadata['web_sha256'][:16]
    version_root.mkdir(parents=True, exist_ok=False)
    with tarfile.open(bundle / 'web.tar.gz') as archive:
        for member in archive.getmembers():
            assert member.isfile() and not member.name.startswith('/') and '..' not in Path(member.name).parts
        archive.extractall(version_root, filter='data')
    for name, digest in metadata['file_sha256'].items():
        assert sha(version_root / name) == digest
    stamp = time.strftime('%Y%m%dT%H%M%SZ', time.gmtime())
    backup = ROOT / 'backups' / ('keeper-pool-' + stamp)
    backup.mkdir(parents=True, mode=0o700)
    shutil.copy2(caddy, backup / 'Caddyfile')
    link = ROOT / 'edge/keeper-current'
    previous = os.readlink(link) if link.is_symlink() else None
    assert not link.exists() or link.is_symlink(), 'Existing UI path is not a managed symlink'
    record = dict(target='43.133.75.82', source_commit=commit, source_tree=tree, web_sha256=metadata['web_sha256'], patch_sha256=metadata['patch_sha256'], previous_ui=previous, backup=str(backup), rollback=False, checks=[], phases={}, production='not queried or modified')
    candidate = ROOT / 'edge/Caddyfile.pool-candidate'
    atomic(candidate, (bundle / 'Caddyfile').read_bytes())
    switched = False
    try:
        check_start = time.monotonic()
        run('docker', 'exec', EDGE, 'caddy', 'validate', '--config', '/etc/caddy/Caddyfile.pool-candidate', '--adapter', 'caddyfile')
        record['phases']['config_validation_seconds'] = round(time.monotonic() - check_start, 2)
        temp_link = link.with_name('keeper-current-next')
        temp_link.symlink_to('keeper-ui/' + version_root.name)
        os.replace(temp_link, link)
        switched = True
        os.replace(candidate, caddy)
        cutover = time.monotonic()
        run('docker', 'exec', EDGE, 'caddy', 'reload', '--config', '/etc/caddy/Caddyfile', '--adapter', 'caddyfile')
        record['phases']['reload_seconds'] = round(time.monotonic() - cutover, 2)
        for path in ['/keeper/', '/keeper/overview?embed=cpamc', '/keeper/settings']:
            body, headers = fetch(path)
            assert hashlib.sha256(body).hexdigest() == metadata['file_sha256']['index.html']
            assert 'no-store' in headers.get('Cache-Control', '')
            assert "frame-ancestors 'self'" == headers.get('Content-Security-Policy')
        for name, digest in metadata['file_sha256'].items():
            if name == 'index.html':
                continue
            body, _ = fetch('/keeper/' + name)
            assert hashlib.sha256(body).hexdigest() == digest
        health, _ = fetch('/keeper/healthz')
        assert json.loads(health)['status'] == 'ok'
        for name, started in before.items():
            assert run('docker', 'inspect', name, '--format', '{{.State.StartedAt}}') == started
        record.update(result='published', checks=['Caddy validation', 'HTTPS index and routes match pinned artifact', 'all public assets match checksums', 'HTML no-store and same-origin iframe CSP', 'Keeper backend health', 'CPA/CPAMP/Keeper not restarted'], limitations=['Pool capacity is an equal-capacity estimate, not a guaranteed quota', 'Fresh sample baseline is stored per browser', 'Simulated debit is not the settlement ledger'])
        print('Keeper pool UI published; no application restart', flush=True)
    except Exception:
        if switched:
            atomic(caddy, (backup / 'Caddyfile').read_bytes())
            if previous is not None:
                restored = link.with_name('keeper-current-restore')
                restored.symlink_to(previous)
                os.replace(restored, link)
            else:
                link.unlink()
            run('docker', 'exec', EDGE, 'caddy', 'reload', '--config', '/etc/caddy/Caddyfile', '--adapter', 'caddyfile')
        record.update(result='failed', rollback=True)
        raise
    finally:
        record['phases']['total_seconds'] = round(time.monotonic() - start, 2)
        record['caddy_sha256'] = sha(bundle / 'Caddyfile')
        atomic(backup / 'release.json', (json.dumps(record, indent=2) + '\n').encode(), 0o600)
        print('Release record: ' + str(backup / 'release.json'), flush=True)


if __name__ == '__main__':
    main()
