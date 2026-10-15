#!/usr/bin/env python3
"""Install a pinned ticket plugin with a user-authorized brief CPA outage.
Run as root on the CPA test host with a bundle from clean, pushed main.
"""
import copy
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time
import urllib.request
import yaml
import zipfile

ROOT = Path('/opt/cpa-test-station')
GATEWAY = 'cpa-test-station-cpa-gateway-1'
PUBLIC = 'https://cpa-test.xingqiaolab.top'
ID = 'cliproxy-ticket-gateway'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def run(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT).strip()


def atomic(path, data, mode=0o644):
    temporary = path.with_name(path.name + '.ticket-new')
    with temporary.open('wb') as stream:
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())
    os.chmod(temporary, mode)
    os.replace(temporary, path)


def write_config(path, data):
    # The running container binds this file's inode; retain it across restart.
    with path.open('wb') as stream:
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())


def main():
    bundle, archive, commit, tree = sys.argv[1:]
    assert re.fullmatch(r'[0-9a-f]{40}', commit)
    assert re.fullmatch(r'[0-9a-f]{40}', tree)
    bundle = Path(bundle)
    locks = []
    for path in ['/opt/sub2api-test-station/.api-release.lock', str(ROOT / '.keeper-release.lock'), str(ROOT / '.ticket-release.lock')]:
        lock = open(path, 'a')
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        locks.append(lock)
    started = time.monotonic()
    source = json.loads((bundle / 'ticket-source.json').read_text())
    version = source['version']
    assert re.fullmatch(r'\d+\.\d+\.\d+', version)
    package = Path(archive).read_bytes()
    assert sha(package) == source['artifact']['sha256']
    assert len(package) == source['artifact']['size']
    with zipfile.ZipFile(archive) as z:
        binary = z.read(ID + '.so')
    assert sha(binary) == source['binary_sha256']
    ui = (bundle / 'ticket-gateway-ui.html').read_bytes()
    assert sha(ui) == source['ui_sha256']
    config_path = ROOT / 'cliproxyapi/config.yaml'
    plugin_path = ROOT / 'cliproxyapi' / source['hot_load_path']
    assert plugin_path.parent == ROOT / 'cliproxyapi/plugins'
    assert not plugin_path.exists(), 'Candidate already exists; inspect its state first'
    key = (ROOT / 'secrets/cpa-management-key').read_text().strip()

    def api(path, base='http://127.0.0.1:8317'):
        q = urllib.request.Request(base + path, headers={'Authorization': 'Bearer ' + key})
        with urllib.request.urlopen(q, timeout=12) as response:
            return json.load(response)

    before = json.loads(run('docker', 'inspect', *run('docker', 'ps', '-q').split()))
    before = {c['Name']: (c['Id'], c['State']['StartedAt']) for c in before}
    initial = api('/v8/management/plugins')
    old = next(p for p in initial['plugins'] if p['id'] == ID)
    old_version = old['metadata']['version']
    assert old['registered'] and old['effective_enabled']
    backup = ROOT / 'backups' / time.strftime('ticket-' + version + '-%Y%m%dT%H%M%SZ', time.gmtime())
    backup.mkdir(mode=0o700)
    original = config_path.read_bytes()
    old_ui = (ROOT / 'edge/ticket-gateway-ui.html').read_bytes()
    atomic(backup / 'config.yaml', original, 0o600)
    atomic(backup / 'previous-ui.html', old_ui, 0o600)
    record = dict(target='43.133.75.82 CPA', version=version, commit=commit, tree=tree, source=source,
                  rollback_directory=str(backup), verification=[], phases={}, result='in_progress')
    atomic(backup / 'release.json', (json.dumps(record, indent=2) + '\n').encode(), 0o600)
    interrupted = False
    try:
        interrupted = True
        outage_started = time.monotonic()
        run('docker', 'stop', '--time', '30', GATEWAY)
        # Snapshot the final config after all previous process writes have stopped.
        original = config_path.read_bytes()
        atomic(backup / 'config.yaml', original, 0o600)
        parsed = yaml.safe_load(original)
        entry = parsed['plugins']['configs'][ID]
        assert entry['store']['version'] == old_version
        desired = copy.deepcopy(parsed)
        desired['plugins']['configs'][ID]['store']['version'] = version
        pattern = r'(?m)^(\s*version:\s*)([\"\']?)' + re.escape(old_version) + r'\2(\s*)$'
        amended, count = re.subn(pattern, lambda m: m[1] + m[2] + version + m[2] + m[3], original.decode())
        assert count == 1 and yaml.safe_load(amended) == desired, 'Only store.version may change'
        atomic(plugin_path, binary)
        write_config(config_path, amended.encode())
        atomic(ROOT / 'edge/ticket-gateway-ui.html', ui)
        run('docker', 'start', GATEWAY)
        deadline = time.monotonic() + 90
        while True:
            try:
                plugins = api('/v8/management/plugins')['plugins']
                active = next(p for p in plugins if p['id'] == ID)
                status = api('/v0/management/plugins/' + ID + '/status')
                assert active['registered'] and active['effective_enabled']
                assert active['metadata']['version'] == version
                assert status['configured'] and not status['paused']
                assert not status.get('update_requires_reconnect')
                assert not status.get('update_confirmation_required')
                assert status.get('update_handoff_state', '') in ['', 'preserved']
                assert run('docker', 'inspect', GATEWAY, '--format', '{{.State.Health.Status}}') == 'healthy'
                break
            except (Exception, AssertionError):
                if time.monotonic() >= deadline:
                    raise RuntimeError('CPA did not become ready with the expected plugin') from None
                time.sleep(1)
        record['phases']['outage_and_readiness_seconds'] = round(time.monotonic() - outage_started, 2)
        assert api('/v8/management/config/plugins/configs/' + ID) == desired['plugins']['configs'][ID]
        record['verification'] += ['Official archive/binary SHA256 verified', 'Plugin registered and enabled at ' + version,
                                   'CPA healthy; configured and not paused; reconnect/confirmation not required',
                                   'Only store.version changed; installation identity and business configuration preserved']
        for path, expected in [('/v0/resource/plugins/' + ID + '/ui', source['ui_sha256'])]:
            with urllib.request.urlopen(PUBLIC + path, timeout=15) as response:
                assert response.status == 200 and sha(response.read()) == expected
        with urllib.request.urlopen(PUBLIC + '/management.html', timeout=15) as response:
            assert response.status == 200
        public_plugins = api('/v8/management/plugins', PUBLIC)['plugins']
        assert next(p for p in public_plugins if p['id'] == ID)['metadata']['version'] == version
        record['verification'] += ['Public HTTPS management HTTP 200', 'Public UI SHA256 matches committed main',
                                   'Public authenticated API reports ' + version]
        after = json.loads(run('docker', 'inspect', *[name.lstrip('/') for name in before]))
        for c in after:
            old_id, old_start = before[c['Name']]
            assert c['Id'] == old_id
            if c['Name'] != '/' + GATEWAY:
                assert c['State']['StartedAt'] == old_start
        record['verification'].append('Container IDs unchanged; only CPA gateway restarted')
        record['status'] = {k: status.get(k) for k in ['configured', 'paused', 'relay_fresh', 'update_handoff_state', 'update_requires_reconnect', 'update_confirmation_required']}
        record['result'] = 'success'
    except Exception as error:
        record['result'] = 'rolled_back'
        record['error_type'] = type(error).__name__
        if interrupted:
            run('docker', 'stop', '--time', '30', GATEWAY)
            write_config(config_path, original)
            atomic(ROOT / 'edge/ticket-gateway-ui.html', old_ui)
            if plugin_path.exists():
                shutil.move(str(plugin_path), str(backup / plugin_path.name))
            run('docker', 'start', GATEWAY)
            deadline = time.monotonic() + 90
            while True:
                try:
                    active = next(p for p in api('/v8/management/plugins')['plugins'] if p['id'] == ID)
                    assert active['registered'] and active['metadata']['version'] == old_version
                    assert run('docker', 'inspect', GATEWAY, '--format', '{{.State.Health.Status}}') == 'healthy'
                    break
                except Exception:
                    if time.monotonic() >= deadline:
                        record['result'] = 'rollback_unverified'
                        break
                    time.sleep(1)
        raise RuntimeError('Update failed; see protected release record for rollback result') from None
    finally:
        record['total_seconds'] = round(time.monotonic() - started, 2)
        record['limitations'] = ['No real billable inference performed', 'Sub2API application not synchronized; only CPA plugin updated', 'Production host not queried or changed']
        record['rollback'] = 'Stop CPA; restore backup config.yaml into existing bound inode; restore previous-ui.html; move v' + version + ' artifact outside scanned plugin directory; start CPA. Previous plugin retained. Do not restore business data.'
        atomic(backup / 'release.json', (json.dumps(record, indent=2) + '\n').encode(), 0o600)
        print(json.dumps(record, ensure_ascii=False))


if __name__ == '__main__':
    main()
