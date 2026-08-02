import { createReadStream, statSync } from 'node:fs';
import { createServer as createHttpServer } from 'node:http';
import { extname, resolve, sep } from 'node:path';

import { validateRows } from './input-parser.js';

const JSON_CONTENT_TYPE = 'application/json; charset=utf-8';
const CONTENT_TYPES = {
  '.css': 'text/css; charset=utf-8',
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
};

function maskSecret(value) {
  if (typeof value !== 'string' || value.length === 0) {
    return value;
  }
  if (value.length <= 6) {
    return '***';
  }
  return `${value.slice(0, 3)}...${value.slice(-3)}`;
}

function batchSummary(batch) {
  return {
    id: batch.id,
    status: batch.status,
    createdAt: batch.createdAt,
    updatedAt: batch.updatedAt,
    summary: batch.summary,
  };
}

function publicBatch(batch) {
  return {
    ...batchSummary(batch),
    tasks: batch.tasks.map((task) => ({
      ...task,
      accessToken: maskSecret(task.accessToken),
      extractionCdk: maskSecret(task.extractionCdk),
      paymentCdk: maskSecret(task.paymentCdk),
    })),
  };
}

function sendJson(response, statusCode, body) {
  response.writeHead(statusCode, { 'content-type': JSON_CONTENT_TYPE });
  response.end(JSON.stringify(body));
}

function readJson(request) {
  return new Promise((resolve, reject) => {
    let body = '';
    request.setEncoding('utf8');
    request.on('data', (chunk) => {
      body += chunk;
      if (body.length > 1_000_000) {
        reject(new Error('Request body is too large'));
        request.destroy();
      }
    });
    request.on('end', () => {
      try {
        resolve(JSON.parse(body));
      } catch {
        reject(new Error('Request body must be valid JSON'));
      }
    });
    request.on('error', reject);
  });
}

function parseBatchId(pathname) {
  const match = /^\/api\/batches\/(\d+)$/.exec(pathname);
  return match ? Number(match[1]) : null;
}

function serveStatic(response, pathname, publicDirectory) {
  const relativePath = pathname === '/' ? 'index.html' : pathname.replace(/^\/+/, '');
  const path = resolve(publicDirectory, relativePath);

  if (path !== publicDirectory && !path.startsWith(`${publicDirectory}${sep}`)) {
    sendJson(response, 404, { error: 'Not found' });
    return;
  }

  try {
    if (!statSync(path).isFile()) {
      sendJson(response, 404, { error: 'Not found' });
      return;
    }
  } catch {
    sendJson(response, 404, { error: 'Not found' });
    return;
  }

  response.writeHead(200, {
    'content-type': CONTENT_TYPES[extname(path)] ?? 'application/octet-stream',
  });
  createReadStream(path).pipe(response);
}

export function createServer({
  database,
  runner,
  publicDirectory = resolve(import.meta.dirname, '../public'),
}) {
  if (!database || !runner) {
    throw new TypeError('database and runner are required');
  }

  const resolvedPublicDirectory = resolve(publicDirectory);
  let recoveryStarted = false;

  const server = createHttpServer(async (request, response) => {
    const url = new URL(request.url, 'http://localhost');
    const batchId = parseBatchId(url.pathname);

    try {
      if (request.method === 'GET' && url.pathname === '/api/batches') {
        const batches = (database.listBatches?.() ?? database.getUnfinishedBatches())
          .map(batchSummary);
        sendJson(response, 200, { batches });
        return;
      }

      if (request.method === 'GET' && batchId !== null) {
        const batch = database.getBatch(batchId);
        if (!batch) {
          sendJson(response, 404, { error: 'Batch not found' });
          return;
        }
        sendJson(response, 200, { batch: publicBatch(batch) });
        return;
      }

      if (request.method === 'POST' && url.pathname === '/api/batches') {
        const payload = await readJson(request);
        if (!Array.isArray(payload?.rows) || payload.rows.length === 0) {
          sendJson(response, 400, {
            error: 'Validation failed',
            errors: [{ row: 0, message: 'At least one row is required' }],
          });
          return;
        }

        const validation = validateRows(payload.rows);
        if (!validation.valid) {
          sendJson(response, 400, { error: 'Validation failed', errors: validation.errors });
          return;
        }

        const batch = database.createBatch(payload.rows.map((row) => ({
          accessToken: row.accessToken.trim(),
          extractionCdk: row.extractionCdk.trim(),
          paymentCdk: row.paymentCdk.trim(),
        })));
        sendJson(response, 201, { batch: publicBatch(batch) });
        void Promise.resolve(runner.start(batch.id)).catch(() => {});
        return;
      }

      if (url.pathname.startsWith('/api/')) {
        sendJson(response, 404, { error: 'Not found' });
        return;
      }

      serveStatic(response, url.pathname, resolvedPublicDirectory);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unexpected server error';
      sendJson(response, 400, { error: message });
    }
  });

  function resume() {
    if (recoveryStarted) {
      return;
    }
    recoveryStarted = true;
    void Promise.resolve(runner.resume()).catch(() => {});
  }

  server.once('listening', resume);
  server.start = async ({ port = 3000, host = '127.0.0.1' } = {}) => {
    server.listen(port, host);
    await new Promise((resolve, reject) => {
      server.once('listening', resolve);
      server.once('error', reject);
    });
    return server.address();
  };
  server.shutdown = async () => {
    await new Promise((resolve, reject) => {
      server.close((error) => (error ? reject(error) : resolve()));
    });
    database.close?.();
  };

  return server;
}
