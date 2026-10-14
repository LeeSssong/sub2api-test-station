#!/usr/bin/env python3
"""Release only /docs from clean, pushed root main, using a read-only static mount."""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import runpy
import shutil
import subprocess
import sys
import tarfile
import time
import urllib.request

common = runpy.run_path(str(Path(__file__).with_name('release-test-station-homepage.py')))
run, save, inspect, reload_caddy = [common[k] for k in ('run', 'save', 'inspect', 'reload_caddy')]
ROOT, SSH, PROJECT = [common[k] for k in ('ROOT', 'SSH', 'PROJECT')]


def docs_route(config, service):
    if not re.fullmatch(r'docs-[a-f0-9]{12}', service):
        raise ValueError('invalid docs service')
    pattern = r'  # docs-release begin\n.*?  # docs-release end\n'
    old = re.search(pattern, config, re.S)
    if old is None:
        old = re.search(r'  @docs path /docs /docs/\*\n  handle @docs \{\n    root \* /srv/home\n    file_server\n  \}\n', config)
    if old is None:
        raise ValueError('unknown docs routing; refusing to change it')
    block = ('  # docs-release begin\n  @docs path /docs /docs/*\n'
             '  handle @docs {\n    reverse_proxy ' + service + ':80\n  }\n'
             '  # docs-release end\n')
    return config[:old.start()] + block + config[old.end():]


def verify_files(root, manifest):
    for name, expected in manifest['checksums'].items():
        if Path(name).is_absolute() or '..' in Path(name).parts:
            raise ValueError('unsafe asset path')
        if hashlib.sha256((root / name).read_bytes()).hexdigest() != expected:
            raise ValueError('artifact mismatch: ' + name)


def public_checks(manifest):
    for path in ('/health', '/readyz'):
        with urllib.request.urlopen('http://127.0.0.1' + path, timeout=15) as response:
            if json.load(response).get('status') not in ('ok', 'ready'):
                raise ValueError('health check failed')
    names = ['index.html', 'manifest.json', 'release-manifest.json'] + manifest['visible_images']
    for name in names:
        with urllib.request.urlopen('http://127.0.0.1/docs/' + name, timeout=15) as response:
            if hashlib.sha256(response.read()).hexdigest() != manifest['checksums'][name]:
                raise ValueError('public checksum differs: ' + name)


def deploy(bundle):
    started = time.monotonic()
    manifest = json.loads((bundle / 'manifest.json').read_text())
    commit, tree = manifest['source_commit'], manifest['source_tree']
    if not all(re.fullmatch('[a-f0-9]{40}', x) for x in (commit, tree)):
        raise ValueError('invalid source identity')
    verify_files(bundle / 'docs', manifest)
    previous = json.loads((ROOT / 'release-state.json').read_text())
    if previous['source_commit'] != manifest['previous_api_commit']:
        raise ValueError('API changed during preparation')
    cid = run(['docker', 'ps', '-q', '--filter', 'label=com.docker.compose.project=' + PROJECT,
               '--filter', 'label=com.docker.compose.service=test-station-caddy']).strip()
    if not cid or '\n' in cid:
        raise ValueError('expected one public Caddy')
    caddy = inspect(cid)
    config_path = Path(next(m['Source'] for m in caddy['Mounts'] if m['Destination'] == '/etc/caddy/Caddyfile'))
    if config_path.is_symlink() or not str(config_path).startswith(str(ROOT / 'releases') + '/'):
        raise ValueError('unexpected Caddy path')
    old = config_path.read_text()
    adapted = json.loads(run(['docker', 'exec', '-i', cid, 'caddy', 'adapt', '--config', '-', '--adapter', 'caddyfile'], old.encode()))
    loaded = json.loads(run(['docker', 'exec', cid, 'wget', '-qO-', 'http://127.0.0.1:2019/config/']))
    if adapted != loaded:
        raise ValueError('persisted routing differs from live routing')
    service = 'docs-' + commit[:12]
    config = docs_route(old, service)
    release = ROOT / 'docs-releases' / commit
    release.mkdir(parents=True, exist_ok=False)
    shutil.copytree(bundle / 'docs', release / 'docs')
    save(release / 'previous-Caddyfile', old)
    save(release / 'Caddyfile', config)
    save(release / 'manifest.json', manifest)
    save(release / 'previous-release-state.json', previous)
    compose = {'name': PROJECT, 'services': {service: {
        'image': caddy['Image'], 'restart': 'unless-stopped', 'pull_policy': 'never',
        'command': ['caddy', 'file-server', '--root', '/srv', '--listen', ':80'],
        'volumes': [str(release / 'docs') + ':/srv/docs:ro'],
        'networks': ['test-station'], 'stop_grace_period': '300s'}},
        'networks': {'test-station': {'external': True, 'name': PROJECT + '-network'}}}
    save(release / 'compose.json', compose)
    record = dict(manifest, result='prepared', rolled_back=False, stage_seconds={},
                  caddy_host_path=str(config_path), caddy_container=cid,
                  runtime_image=caddy['Image'], rollback_config=str(release / 'previous-Caddyfile'))
    save(release / 'deployment.json', record)
    switched = False
    try:
        run(['docker', 'compose', '-p', PROJECT, '-f', str(release / 'compose.json'), 'up', '-d', '--no-deps', service])
        candidate = run(['docker', 'compose', '-p', PROJECT, '-f', str(release / 'compose.json'), 'ps', '-q', service]).strip()
        deadline = time.monotonic() + 30
        while True:
            try:
                data = run(['docker', 'exec', cid, 'wget', '-qO-', 'http://' + service + '/docs/'])
                if hashlib.sha256(data.encode()).hexdigest() != manifest['checksums']['index.html']:
                    raise ValueError('candidate content differs')
                break
            except RuntimeError:
                if time.monotonic() >= deadline:
                    raise
                time.sleep(1)
        record['stage_seconds']['prepare_and_ready'] = round(time.monotonic() - started, 2)
        # Reload has an atomic runtime boundary. Mark the attempt before calling
        # so a lost response also restores the known previous configuration.
        switched = True
        reload_caddy(cid, config)
        public_checks(manifest)
        # Replace the restart configuration atomically; reload uses stdin, as
        # elsewhere in this release chain, to avoid stale bind-mounted inodes.
        stat = config_path.stat()
        save(config_path, config)
        os.chmod(config_path, stat.st_mode & 0o777)
        os.chown(config_path, stat.st_uid, stat.st_gid)
        previous['docs'] = {'source_commit': commit, 'source_tree': tree,
                            'release_dir': str(release), 'service': service,
                            'manifest_sha256': manifest['checksums']['release-manifest.json']}
        save(ROOT / 'release-state.json', previous)
        record.update(result='succeeded', candidate_container=candidate,
                      duration_seconds=round(time.monotonic() - started, 2))
        record['stage_seconds']['switch_verify_and_persist'] = round(record['duration_seconds'] - record['stage_seconds']['prepare_and_ready'], 2)
        save(release / 'deployment.json', record)
        print(json.dumps({'result': 'succeeded', 'release_dir': str(release),
                          'duration_seconds': record['duration_seconds']}))
    except Exception:
        if switched:
            reload_caddy(cid, old)
            save(config_path, old)
            save(ROOT / 'release-state.json', json.loads((release / 'previous-release-state.json').read_text()))
        record.update(result='failed', rolled_back=switched)
        save(release / 'deployment.json', record)
        raise


def publish():
    root = Path.cwd()
    if not (root / '.git').is_dir() or run(['git', 'branch', '--show-current']).strip() != 'main' or run(['git', 'status', '--porcelain']).strip():
        raise ValueError('use clean root main')
    run(['git', 'fetch', 'origin', 'main'])
    commit, tree = [run(['git', 'rev-parse', x]).strip() for x in ('HEAD', 'HEAD^{tree}')]
    if [commit, tree] != [run(['git', 'rev-parse', x]).strip() for x in ('origin/main', 'origin/main^{tree}')]:
        raise ValueError('main differs from origin/main')
    ssh_config = run(['ssh', '-G', 'sub2api-test-station']).splitlines()
    if not all(x in ssh_config for x in ('hostname 43.133.75.82', 'user ubuntu', 'port 22', 'stricthostkeychecking true')):
        raise ValueError('unexpected SSH identity')
    previous = json.loads(run(SSH + ['sudo -n cat /opt/sub2api-test-station/release-state.json']))
    stage = root / '.release' / ('docs-' + commit)
    stage.mkdir(parents=True, exist_ok=False)
    shutil.copytree(root / 'homepage/public/docs', stage / 'docs')
    for name in ('release-test-station-docs.py', 'release-test-station-homepage.py'):
        shutil.copyfile(root / 'ops' / name, stage / name)
    checksums = {p.relative_to(stage / 'docs').as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
                 for p in sorted((stage / 'docs').rglob('*')) if p.is_file()}
    images = sorted(set(re.findall(r'<img[^>]+src="([^"]+)"', (stage / 'docs/index.html').read_text())))
    save(stage / 'manifest.json', {'source_commit': commit, 'source_tree': tree,
                                 'previous_api_commit': previous['source_commit'],
                                 'checksums': checksums, 'visible_images': images})
    archive = stage / 'bundle.tar.gz'
    with tarfile.open(archive, 'w:gz') as tar:
        for name in ('docs', 'manifest.json', 'release-test-station-docs.py', 'release-test-station-homepage.py'):
            tar.add(stage / name, arcname=name)
    remote = run(SSH + ['mktemp -d /var/tmp/sub2api-test-station-docs.XXXXXX']).strip()
    if not re.fullmatch('/var/tmp/sub2api-test-station-docs.[a-zA-Z0-9]+', remote):
        raise ValueError('unexpected staging path')
    subprocess.run(['scp', '-q', str(archive), 'sub2api-test-station:' + remote + '/bundle.tar.gz'], check=True)
    checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
    run(SSH + ["test \"$(sha256sum " + remote + "/bundle.tar.gz | cut -d' ' -f1)\" = " + checksum +
               ' && tar -xzf ' + remote + '/bundle.tar.gz -C ' + remote])
    print(run(SSH + ['sudo -n python3 ' + remote + '/release-test-station-docs.py --host ' + remote]))


if __name__ == '__main__':
    if len(sys.argv) == 3 and sys.argv[1] == '--host':
        bundle = Path(sys.argv[2]).resolve()
        if os.geteuid() != 0 or not re.fullmatch('/var/tmp/sub2api-test-station-docs.[a-zA-Z0-9]+', str(bundle)):
            raise ValueError('invalid host bundle')
        lock = open(ROOT / '.api-release.lock', 'a')
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        deploy(bundle)
    elif len(sys.argv) == 1:
        publish()
    else:
        raise ValueError('usage: release-test-station-docs.py')
