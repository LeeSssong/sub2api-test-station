import { createBatchRunner } from './batch-runner.js';
import { createDatabase } from './database.js';
import { createServer } from './server.js';
import { createUpstreamClient } from './upstream-client.js';

const DEFAULT_DATABASE_PATH = 'data/workbench.sqlite';
const DEFAULT_PORT = 4318;

function configuredPort(value) {
  const port = Number(value);
  return Number.isInteger(port) && port >= 0 && port <= 65_535
    ? port
    : DEFAULT_PORT;
}

export function createApplication({
  databasePath = process.env.WORKBENCH_DB ?? DEFAULT_DATABASE_PATH,
  baseUrl = 'https://kk.642636.xyz',
} = {}) {
  const database = createDatabase(databasePath);
  const upstream = createUpstreamClient({
    baseUrl,
    cookieStore: database,
  });
  const runner = createBatchRunner({ database, upstream });
  const server = createServer({ database, runner });

  return {
    server,
    start({
      port = configuredPort(process.env.PORT),
      host = '127.0.0.1',
    } = {}) {
      return server.start({ port, host });
    },
    shutdown() {
      return server.shutdown();
    },
  };
}

async function main() {
  const application = createApplication();
  const shutdown = async () => {
    try {
      await application.shutdown();
    } finally {
      process.exit(0);
    }
  };

  process.once('SIGINT', shutdown);
  process.once('SIGTERM', shutdown);

  try {
    const address = await application.start();
    console.log(`Local workbench listening at http://${address.address}:${address.port}`);
  } catch (error) {
    console.error(error);
    await application.shutdown().catch(() => {});
    process.exitCode = 1;
  }
}

if (import.meta.main) {
  void main();
}
