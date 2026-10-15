const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { test } = require('node:test');

const root = path.join(__dirname, '../..');
const source = JSON.parse(fs.readFileSync(path.join(root, 'infra/cpa-test-station/ticket-source.json'), 'utf8'));
const ui = fs.readFileSync(path.join(root, 'infra/cpa-test-station/ticket-gateway-ui.html'), 'utf8');

test('ticket source and edge UI pin the same official release', () => {
  assert.equal(source.version, '1.6.3');
  assert.equal(source.artifact.goos, 'linux');
  assert.equal(source.artifact.goarch, 'amd64');
  assert.equal(source.artifact.sha256, '6474dd829babbf4206ffb5bb45a50a9a75a69d1cce681b74f9440afd1ae2a747');
  assert.equal(source.artifact.size, 7082235);
  assert.equal(source.binary_sha256, 'bf32141706c970738c740b90d544b9c118c8dbadf5d83830e89e84aad1735e2c');
  assert.equal(source.hot_load_path, 'plugins/cliproxy-ticket-gateway-v1.6.3.so');
  assert.match(ui, /const PRODUCT = \{"id":"cliproxy-ticket-gateway"[^;]*"version":"1\.6\.3"/);
});
