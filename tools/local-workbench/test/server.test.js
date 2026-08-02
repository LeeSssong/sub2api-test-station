import assert from 'node:assert/strict';
import { once } from 'node:events';
import { request } from 'node:http';
import test from 'node:test';

import { createServer } from '../src/server.js';

function batch(id, rows = []) {
  return {
    id,
    status: 'pending',
    createdAt: '2026-08-03T00:00:00.000Z',
    updatedAt: '2026-08-03T00:00:00.000Z',
    summary: { total: rows.length, pending: rows.length },
    tasks: rows.map((row, index) => ({
      id: index + 1,
      batchId: id,
      rowNumber: index + 1,
      ...row,
      status: 'pending',
    })),
  };
}

function createDependencies() {
  const batches = new Map();
  let nextId = 1;
  const started = [];

  return {
    database: {
      createBatch(rows) {
        const created = batch(nextId, rows);
        nextId += 1;
        batches.set(created.id, created);
        return created;
      },
      getBatch(id) {
        return batches.get(id) ?? null;
      },
      getUnfinishedBatches() {
        return [...batches.values()];
      },
      close() {},
    },
    runner: {
      resume() {
        return Promise.resolve([]);
      },
      start(id) {
        started.push(id);
        return new Promise(() => {});
      },
    },
    started,
  };
}

async function withServer(run) {
  const dependencies = createDependencies();
  const app = createServer(dependencies);
  app.listen(0, '127.0.0.1');
  await once(app, 'listening');
  const { port } = app.address();

  try {
    await run({ port, ...dependencies });
  } finally {
    await app.close();
  }
}

function httpRequest(port, method, path, body) {
  return new Promise((resolve, reject) => {
    const payload = body === undefined ? undefined : JSON.stringify(body);
    const clientRequest = request({
      host: '127.0.0.1',
      port,
      method,
      path,
      headers: payload === undefined ? {} : {
        'content-type': 'application/json',
        'content-length': Buffer.byteLength(payload),
      },
    }, (response) => {
      let responseBody = '';
      response.setEncoding('utf8');
      response.on('data', (chunk) => {
        responseBody += chunk;
      });
      response.on('end', () => {
        resolve({
          statusCode: response.statusCode,
          headers: response.headers,
          body: responseBody ? JSON.parse(responseBody) : null,
        });
      });
    });
    clientRequest.on('error', reject);
    clientRequest.end(payload);
  });
}

test('POST /api/batches rejects invalid rows with JSON validation errors', async () => {
  await withServer(async ({ port }) => {
    const response = await httpRequest(port, 'POST', '/api/batches', {
      rows: [{ accessToken: 'token', extractionCdk: '', paymentCdk: 'payment' }],
    });

    assert.equal(response.statusCode, 400);
    assert.equal(response.headers['content-type'], 'application/json; charset=utf-8');
    assert.equal(response.body.error, 'Validation failed');
    assert.deepEqual(response.body.errors, [
      { row: 1, message: 'Missing extraction CDK' },
    ]);
  });
});

test('POST /api/batches creates a batch and starts it after responding', async () => {
  await withServer(async ({ port, started }) => {
    const response = await httpRequest(port, 'POST', '/api/batches', {
      rows: [{ accessToken: 'token', extractionCdk: 'extract', paymentCdk: 'payment' }],
    });

    assert.equal(response.statusCode, 201);
    assert.equal(response.body.batch.id, 1);
    assert.equal(response.body.batch.tasks[0].accessToken, '***');
    assert.deepEqual(started, [1]);
  });
});

test('GET /api/batches returns batch history without task secrets', async () => {
  await withServer(async ({ port, database }) => {
    database.createBatch([
      { accessToken: 'token-secret', extractionCdk: 'extract-secret', paymentCdk: 'payment-secret' },
    ]);

    const response = await httpRequest(port, 'GET', '/api/batches');

    assert.equal(response.statusCode, 200);
    assert.deepEqual(response.body.batches, [{
      id: 1,
      status: 'pending',
      createdAt: '2026-08-03T00:00:00.000Z',
      updatedAt: '2026-08-03T00:00:00.000Z',
      summary: { total: 1, pending: 1 },
    }]);
  });
});

test('GET /api/batches/:id returns the selected batch with masked secrets', async () => {
  await withServer(async ({ port, database }) => {
    database.createBatch([
      { accessToken: 'token-secret', extractionCdk: 'extract-secret', paymentCdk: 'payment-secret' },
    ]);

    const response = await httpRequest(port, 'GET', '/api/batches/1');

    assert.equal(response.statusCode, 200);
    assert.equal(response.body.batch.id, 1);
    assert.equal(response.body.batch.tasks[0].accessToken, 'tok...ret');
    assert.equal(response.body.batch.tasks[0].extractionCdk, 'ext...ret');
    assert.equal(response.body.batch.tasks[0].paymentCdk, 'pay...ret');
  });
});

test('GET /api/batches/:id returns a JSON not-found response', async () => {
  await withServer(async ({ port }) => {
    const response = await httpRequest(port, 'GET', '/api/batches/99');

    assert.equal(response.statusCode, 404);
    assert.deepEqual(response.body, { error: 'Batch not found' });
  });
});
