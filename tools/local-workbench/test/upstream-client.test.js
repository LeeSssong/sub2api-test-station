import assert from 'node:assert/strict';
import test from 'node:test';

import {
  UpstreamError,
  createUpstreamClient,
} from '../src/upstream-client.js';

const BASE_URL = 'https://kk.642636.xyz';

function createCookieStore(initialValue = null) {
  let value = initialValue;

  return {
    getSetting(key) {
      assert.equal(key, 'helan_client');
      return value;
    },
    setSetting(key, nextValue) {
      assert.equal(key, 'helan_client');
      value = nextValue;
    },
    read() {
      return value;
    },
  };
}

function jsonResponse(body, init = {}) {
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: {
      'Content-Type': 'application/json',
      ...init.headers,
    },
  });
}

test('submitExtractions posts paired AT and CDK values only in JSON', async () => {
  const requests = [];
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore(),
    sleep: async () => {},
    fetchImpl: async (url, options) => {
      requests.push({ url, options });
      return jsonResponse({
        tasks: [{ taskId: 'ext-1', status: 'processing' }],
      }, { status: 202 });
    },
  });

  const result = await client.submitExtractions([
    { accessToken: 'at-1', extractionCdk: 'extract-1', paymentCdk: 'pay-1' },
    { accessToken: 'at-2', extractionCdk: 'extract-2', paymentCdk: 'pay-2' },
  ]);

  assert.deepEqual(result, {
    tasks: [{ taskId: 'ext-1', status: 'processing' }],
  });
  assert.equal(requests.length, 1);
  assert.equal(requests[0].url, `${BASE_URL}/api/extractions/batch`);
  assert.equal(requests[0].options.method, 'POST');
  assert.deepEqual(requests[0].options.headers, {
    Accept: 'application/json',
    'Content-Type': 'application/json',
  });
  assert.deepEqual(JSON.parse(requests[0].options.body), {
    accessTokens: ['at-1', 'at-2'],
    cdks: ['extract-1', 'extract-2'],
  });
  assert.doesNotMatch(requests[0].url, /at-1|extract-1|pay-1/);
});

test('captures and reuses only the helan_client Cookie', async () => {
  const requests = [];
  const cookieStore = createCookieStore();
  const responses = [
    jsonResponse({ tasks: [] }, {
      status: 202,
      headers: {
        'Set-Cookie': 'helan_client=session-one; HttpOnly; Secure; Path=/',
      },
    }),
    jsonResponse({ ok: true, items: [] }),
  ];
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore,
    sleep: async () => {},
    fetchImpl: async (url, options) => {
      requests.push({ url, options });
      return responses.shift();
    },
  });

  await client.submitExtractions([]);
  await client.getPaymentStatuses(['pay-1']);

  assert.equal(cookieStore.read(), 'session-one');
  assert.equal(requests[0].options.headers.Cookie, undefined);
  assert.equal(requests[1].options.headers.Cookie, 'helan_client=session-one');
});

test('captures helan_client when the response sets multiple Cookies', async () => {
  const headers = new Headers();
  headers.append('Set-Cookie', 'theme=dark; Path=/');
  headers.append(
    'Set-Cookie',
    'helan_client=session-two; HttpOnly; Secure; Path=/',
  );
  headers.append('Set-Cookie', 'locale=zh-CN; Path=/');
  const cookieStore = createCookieStore();
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore,
    sleep: async () => {},
    fetchImpl: async () => new Response(JSON.stringify({ ok: true, items: [] }), {
      status: 200,
      headers,
    }),
  });

  await client.getPaymentStatuses([]);

  assert.equal(cookieStore.read(), 'session-two');
});

test('getExtraction returns Retry-After for a processing task', async () => {
  const requests = [];
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore('saved-session'),
    sleep: async () => {},
    fetchImpl: async (url, options) => {
      requests.push({ url, options });
      return new Response('处理中', {
        status: 202,
        headers: { 'Retry-After': '3' },
      });
    },
  });

  assert.deepEqual(await client.getExtraction('ext/with spaces'), {
    status: 'processing',
    retryAfterMs: 3000,
  });
  assert.equal(
    requests[0].url,
    `${BASE_URL}/api/extractions/ext%2Fwith%20spaces`,
  );
  assert.equal(requests[0].options.method, 'GET');
  assert.equal(requests[0].options.headers.Cookie, 'helan_client=saved-session');
});

test('getExtraction accepts only an HTTPS URL as success', async () => {
  const bodies = [
    'https://pay.example/checkout?id=1\n',
    'http://pay.example/insecure',
    '失败',
  ];
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore(),
    sleep: async () => {},
    fetchImpl: async () => new Response(bodies.shift(), { status: 200 }),
  });

  assert.deepEqual(await client.getExtraction('ext-1'), {
    status: 'succeeded',
    paymentUrl: 'https://pay.example/checkout?id=1',
  });
  assert.deepEqual(await client.getExtraction('ext-2'), {
    status: 'failed',
    error: 'http://pay.example/insecure',
  });
  assert.deepEqual(await client.getExtraction('ext-3'), {
    status: 'failed',
    error: '失败',
  });
});

test('submitPayments returns accepted and remaining URLs from 207', async () => {
  const requests = [];
  const partial = {
    submissions: [{ id: 'pay-1', payment_url: 'https://pay.example/1' }],
    remainingPaymentUrls: ['https://pay.example/2'],
    partial: true,
    error: 'capacity exhausted',
  };
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore(),
    sleep: async () => {},
    fetchImpl: async (url, options) => {
      requests.push({ url, options });
      return jsonResponse(partial, { status: 207 });
    },
  });

  assert.deepEqual(
    await client.submitPayments('payment-secret', [
      'https://pay.example/1',
      'https://pay.example/2',
    ]),
    partial,
  );
  assert.equal(requests[0].url, `${BASE_URL}/api/payments/submissions`);
  assert.deepEqual(JSON.parse(requests[0].options.body), {
    paymentCdk: 'payment-secret',
    paymentUrls: ['https://pay.example/1', 'https://pay.example/2'],
  });
  assert.doesNotMatch(requests[0].url, /payment-secret|pay\.example/);
});

test('getPaymentStatuses posts IDs in JSON and returns the response', async () => {
  const requests = [];
  const statusResult = {
    ok: true,
    items: [{ id: 'pay-1', state: 'completed' }],
  };
  const client = createUpstreamClient({
    baseUrl: `${BASE_URL}/`,
    cookieStore: createCookieStore(),
    sleep: async () => {},
    fetchImpl: async (url, options) => {
      requests.push({ url, options });
      return jsonResponse(statusResult);
    },
  });

  assert.deepEqual(await client.getPaymentStatuses(['pay-1']), statusResult);
  assert.equal(
    requests[0].url,
    `${BASE_URL}/api/payments/submissions/status`,
  );
  assert.deepEqual(JSON.parse(requests[0].options.body), { ids: ['pay-1'] });
  assert.doesNotMatch(requests[0].url, /pay-1/);
});

test('retries 429, 502, and 503 with bounded delays', async () => {
  const statuses = [429, 502, 503, 200];
  const delays = [];
  let attempts = 0;
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore(),
    sleep: async (milliseconds) => delays.push(milliseconds),
    fetchImpl: async () => {
      const status = statuses[attempts];
      attempts += 1;
      if (status === 200) {
        return jsonResponse({ ok: true, items: [] });
      }
      return new Response('busy', {
        status,
        headers: attempts === 1 ? { 'Retry-After': '2' } : {},
      });
    },
  });

  assert.deepEqual(await client.getPaymentStatuses([]), { ok: true, items: [] });
  assert.equal(attempts, 4);
  assert.deepEqual(delays, [2000, 500, 1000]);
});

test('honors an HTTP-date Retry-After value', async ({ mock }) => {
  const now = Date.parse('2026-08-03T12:00:00Z');
  mock.method(Date, 'now', () => now);
  const delays = [];
  let attempts = 0;
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore(),
    sleep: async (milliseconds) => delays.push(milliseconds),
    fetchImpl: async () => {
      attempts += 1;
      if (attempts === 1) {
        return new Response('busy', {
          status: 503,
          headers: { 'Retry-After': new Date(now + 4000).toUTCString() },
        });
      }
      return jsonResponse({ ok: true, items: [] });
    },
  });

  await client.getPaymentStatuses([]);

  assert.deepEqual(delays, [4000]);
});

test('getExtraction retries network rejection with bounded backoff', async () => {
  const networkError = new TypeError('socket closed');
  const delays = [];
  let attempts = 0;
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore(),
    sleep: async (milliseconds) => delays.push(milliseconds),
    fetchImpl: async () => {
      attempts += 1;
      if (attempts < 3) {
        throw networkError;
      }
      return new Response('处理中', {
        status: 202,
        headers: { 'Retry-After': '2' },
      });
    },
  });

  assert.deepEqual(await client.getExtraction('ext-1'), {
    status: 'processing',
    retryAfterMs: 2000,
  });
  assert.equal(attempts, 3);
  assert.deepEqual(delays, [250, 500]);
});

test('getPaymentStatuses bounds network rejection retries', async () => {
  const networkError = new TypeError('connection refused');
  const delays = [];
  let attempts = 0;
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore(),
    sleep: async (milliseconds) => delays.push(milliseconds),
    fetchImpl: async () => {
      attempts += 1;
      throw networkError;
    },
  });

  await assert.rejects(
    client.getPaymentStatuses(['pay-1']),
    (error) => {
      assert.ok(error instanceof UpstreamError);
      assert.equal(error.cause, networkError);
      assert.equal(error.retryable, true);
      assert.equal(error.uncertain, false);
      return true;
    },
  );
  assert.equal(attempts, 4);
  assert.deepEqual(delays, [250, 500, 1000]);
});

test('submitExtractions does not retry a network rejection', async () => {
  const networkError = new TypeError('connection reset');
  let attempts = 0;
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore(),
    sleep: async () => assert.fail('submission must not sleep before retrying'),
    fetchImpl: async () => {
      attempts += 1;
      throw networkError;
    },
  });

  await assert.rejects(
    client.submitExtractions([
      { accessToken: 'at-1', extractionCdk: 'extract-1' },
    ]),
    (error) => {
      assert.ok(error instanceof UpstreamError);
      assert.equal(error.cause, networkError);
      assert.equal(error.retryable, false);
      assert.equal(error.uncertain, true);
      return true;
    },
  );
  assert.equal(attempts, 1);
});

test('submitPayments does not retry a network rejection', async () => {
  const networkError = new TypeError('connection reset');
  let attempts = 0;
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore(),
    sleep: async () => assert.fail('submission must not sleep before retrying'),
    fetchImpl: async () => {
      attempts += 1;
      throw networkError;
    },
  });

  await assert.rejects(
    client.submitPayments('payment-cdk', ['https://pay.example/1']),
    (error) => {
      assert.ok(error instanceof UpstreamError);
      assert.equal(error.cause, networkError);
      assert.equal(error.retryable, false);
      assert.equal(error.uncertain, true);
      return true;
    },
  );
  assert.equal(attempts, 1);
});

test('throws UpstreamError after the bounded retry limit', async () => {
  const delays = [];
  let attempts = 0;
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore(),
    sleep: async (milliseconds) => delays.push(milliseconds),
    fetchImpl: async () => {
      attempts += 1;
      return new Response('still busy', { status: 503 });
    },
  });

  await assert.rejects(
    client.getPaymentStatuses(['pay-1']),
    (error) => {
      assert.ok(error instanceof UpstreamError);
      assert.equal(error.status, 503);
      assert.equal(error.body, 'still busy');
      assert.equal(error.retryable, true);
      return true;
    },
  );
  assert.equal(attempts, 4);
  assert.deepEqual(delays, [250, 500, 1000]);
});

test('throws a non-retryable UpstreamError for a rejected JSON request', async () => {
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore(),
    sleep: async () => {},
    fetchImpl: async () => jsonResponse({ error: 'invalid request' }, {
      status: 400,
    }),
  });

  await assert.rejects(
    client.submitPayments('bad-cdk', ['https://pay.example/1']),
    (error) => {
      assert.ok(error instanceof UpstreamError);
      assert.equal(error.message, 'invalid request');
      assert.equal(error.status, 400);
      assert.deepEqual(error.body, { error: 'invalid request' });
      assert.equal(error.retryable, false);
      return true;
    },
  );
});

test('preserves a non-JSON error response in UpstreamError', async () => {
  const client = createUpstreamClient({
    baseUrl: BASE_URL,
    cookieStore: createCookieStore(),
    sleep: async () => {},
    fetchImpl: async () => new Response('<h1>gateway rejected request</h1>', {
      status: 403,
      headers: { 'Content-Type': 'text/html' },
    }),
  });

  await assert.rejects(
    client.getPaymentStatuses(['pay-1']),
    (error) => {
      assert.ok(error instanceof UpstreamError);
      assert.equal(error.status, 403);
      assert.equal(error.body, '<h1>gateway rejected request</h1>');
      assert.equal(error.message, '<h1>gateway rejected request</h1>');
      return true;
    },
  );
});
