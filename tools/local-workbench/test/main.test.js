import assert from 'node:assert/strict';
import { once } from 'node:events';
import { request } from 'node:http';
import test from 'node:test';

import { createApplication } from '../src/main.js';

function getJson(port, path) {
  return new Promise((resolve, reject) => {
    const clientRequest = request({ host: '127.0.0.1', port, path }, (response) => {
      let body = '';
      response.setEncoding('utf8');
      response.on('data', (chunk) => {
        body += chunk;
      });
      response.on('end', () => {
        resolve({ statusCode: response.statusCode, body: JSON.parse(body) });
      });
    });
    clientRequest.on('error', reject);
    clientRequest.end();
  });
}

test('application starts a local workbench server with an isolated database', async () => {
  const application = createApplication({ databasePath: ':memory:' });
  const address = await application.start({ port: 0 });

  try {
    const response = await getJson(address.port, '/api/batches');

    assert.equal(response.statusCode, 200);
    assert.deepEqual(response.body, { batches: [] });
  } finally {
    const closed = once(application.server, 'close');
    await application.shutdown();
    await closed;
  }
});
