const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { test } = require('node:test');

const file = process.env.TICKET_PLUGIN_UI || path.join(__dirname, '../../infra/cpa-test-station/ticket-gateway-ui.html');
const html = fs.readFileSync(file, 'utf8');
const script = [...html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/g)].map(match => match[1]).find(source => source.includes('root.TicketPluginUI'));
assert.ok(script, 'plugin application script exists');
const context = vm.createContext({ URL, TextEncoder, TextDecoder });
vm.runInContext(script, context);
const { nativeSession } = context.TicketPluginUI;

function fixture(version, overrides = {}) {
  const origin = 'https://cpa-test.xingqiaolab.top';
  const host = 'cpa-test.xingqiaolab.top';
  const userAgent = 'session-contract-test';
  const state = { apiBase: origin, managementKey: 'test-only-admin-key', rememberPassword: true, ...overrides };
  const serialized = Buffer.from(JSON.stringify({ state, version: 0 }));
  const mask = Buffer.from(version === 'v2'
    ? `cli-proxy-api-webui::secure-storage|v2|${host}`
    : `cli-proxy-api-webui::secure-storage|${host}|${userAgent}`);
  const encoded = Buffer.from(serialized.map((byte, index) => byte ^ mask[index % mask.length]));
  const storage = new Map([
    ['isLoggedIn', 'true'],
    ['cli-proxy-auth', `enc::${version}::${encoded.toString('base64')}`],
  ]);
  const parent = { location: { href: `${origin}/management.html#/plugin-pages/cliproxy-ticket-gateway/0` } };
  return {
    parent, top: parent, frameElement: { tagName: 'IFRAME' },
    location: { origin, host, pathname: '/v0/resource/plugins/cliproxy-ticket-gateway/ui' },
    navigator: { userAgent }, TextEncoder, TextDecoder,
    atob: value => Buffer.from(value, 'base64').toString('binary'),
    localStorage: { getItem: key => storage.get(key) ?? null },
    storage,
  };
}

for (const version of ['v1', 'v2']) {
  test(`accepts remembered ${version} native panel session`, () => {
    assert.equal(nativeSession(fixture(version)).key, 'test-only-admin-key');
  });
}

test('rejects logged-out sessions', () => {
  const env = fixture('v2');
  env.storage.set('isLoggedIn', 'false');
  assert.throws(() => nativeSession(env), /native_session_required/);
});

test('rejects keys that were not remembered', () => {
  assert.throws(() => nativeSession(fixture('v2', { rememberPassword: false })), /native_session_required/);
});

test('rejects a session for another API origin', () => {
  assert.throws(() => nativeSession(fixture('v2', { apiBase: 'https://other.example' })), /native_session_required/);
});

test('rejects direct resource access outside the native iframe', () => {
  const env = fixture('v2');
  env.parent = env;
  assert.throws(() => nativeSession(env), /native_context_required/);
});
