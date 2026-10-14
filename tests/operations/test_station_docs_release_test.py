import hashlib
import json
from pathlib import Path
import runpy
import tempfile
import unittest
from unittest.mock import patch


REPO = Path(__file__).resolve().parents[2]
MODULE = runpy.run_path(str(REPO / 'ops/release-test-station-docs.py'))
BASE = (REPO / 'infra/independent-test-station/Caddyfile').read_text()


class DocsReleaseTest(unittest.TestCase):
    def test_route_changes_only_docs_and_supports_next_release(self):
        route = MODULE['docs_route']
        candidate = route(BASE, 'docs-0123456789ab')
        self.assertIn('reverse_proxy docs-0123456789ab:80', candidate)
        start = BASE.index('  @docs path ')
        end = BASE.index('  }\n', start) + 4
        self.assertTrue(candidate.startswith(BASE[:start]))
        self.assertTrue(candidate.endswith(BASE[end:]))
        self.assertEqual(route(candidate, 'docs-abcdef012345').count('# docs-release begin'), 1)
        for config, service in [('unknown', 'docs-0123456789ab'), (BASE, 'bad;command')]:
            with self.assertRaises(ValueError):
                route(config, service)

    def test_rejects_changed_artifact(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'index.html').write_text('verified')
            manifest = {'checksums': {'index.html': hashlib.sha256(b'verified').hexdigest()}}
            MODULE['verify_files'](root, manifest)
            (root / 'index.html').write_text('changed')
            with self.assertRaises(ValueError):
                MODULE['verify_files'](root, manifest)

    def test_post_switch_failure_restores_route_and_state(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            config = root / 'releases/previous/Caddyfile'
            config.parent.mkdir(parents=True)
            config.write_text(BASE)
            previous = {'source_commit': 'a' * 40, 'homepage': {'preserved': True}}
            (root / 'release-state.json').write_text(json.dumps(previous))
            bundle = root / 'bundle'
            (bundle / 'docs').mkdir(parents=True)
            (bundle / 'docs/index.html').write_text('verified')
            manifest = {'source_commit': 'b' * 40, 'source_tree': 'c' * 40,
                        'previous_api_commit': 'a' * 40,
                        'checksums': {'index.html': hashlib.sha256(b'verified').hexdigest()}}
            (bundle / 'manifest.json').write_text(json.dumps(manifest))
            reloads = []

            def command(args, data=None):
                if args[1] == 'ps':
                    return 'public-caddy'
                if 'adapt' in args or 'http://127.0.0.1:2019/config/' in args:
                    return '{}'
                if args[-1] == 'http://docs-bbbbbbbbbbbb/docs/':
                    return 'verified'
                return 'candidate' if args[-2:] == ['-q', 'docs-bbbbbbbbbbbb'] else ''

            fake_caddy = {'Image': 'sha256:' + 'd' * 64, 'Mounts': [
                {'Source': str(config), 'Destination': '/etc/caddy/Caddyfile'}]}
            namespace = MODULE['deploy'].__globals__
            with patch.dict(namespace, ROOT=root, run=command,
                            inspect=lambda _: fake_caddy,
                            reload_caddy=lambda _, value: reloads.append(value),
                            public_checks=lambda _: (_ for _ in ()).throw(ValueError('probe failed'))):
                with self.assertRaisesRegex(ValueError, 'probe failed'):
                    MODULE['deploy'](bundle)
            self.assertEqual(len(reloads), 2)
            self.assertEqual(reloads[-1], BASE)
            self.assertEqual(config.read_text(), BASE)
            self.assertEqual(json.loads((root / 'release-state.json').read_text()), previous)
            record = json.loads((root / 'docs-releases' / ('b' * 40) / 'deployment.json').read_text())
            self.assertEqual(record['result'], 'failed')
            self.assertTrue(record['rolled_back'])


if __name__ == '__main__':
    unittest.main()
