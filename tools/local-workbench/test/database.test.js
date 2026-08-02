import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, rm } from 'node:fs/promises';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { DatabaseSync } from 'node:sqlite';

import { createDatabase } from '../src/database.js';

async function withDatabase(run) {
  const directory = await mkdtemp(join(tmpdir(), 'local-workbench-'));
  const database = createDatabase(join(directory, 'workbench.sqlite'));

  try {
    await run(database);
  } finally {
    database.close();
    await rm(directory, { recursive: true, force: true });
  }
}

function createTrackedConnectionFactory({ failExec, failPrepare }) {
  const tracker = {
    closed: false,
    factory(path) {
      const connection = new DatabaseSync(path);

      return {
        exec(sql) {
          if (failExec && sql.includes(failExec)) {
            throw new Error(`forced exec failure: ${failExec}`);
          }
          return connection.exec(sql);
        },
        prepare(sql) {
          if (failPrepare && sql.includes(failPrepare)) {
            throw new Error(`forced prepare failure: ${failPrepare}`);
          }
          return connection.prepare(sql);
        },
        close() {
          tracker.closed = true;
          connection.close();
        },
      };
    },
  };

  return tracker;
}

const rows = [
  {
    accessToken: 'token-2',
    extractionCdk: 'extract-2',
    paymentCdk: 'payment-2',
  },
  {
    accessToken: 'token-1',
    extractionCdk: 'extract-1',
    paymentCdk: 'payment-1',
  },
];

test('createBatch persists paired rows in their original order', async () => {
  await withDatabase(async (database) => {
    const batch = database.createBatch(rows);

    assert.equal(batch.status, 'pending');
    assert.deepEqual(
      batch.tasks.map((task) => ({
        rowNumber: task.rowNumber,
        accessToken: task.accessToken,
        extractionCdk: task.extractionCdk,
        paymentCdk: task.paymentCdk,
        status: task.status,
      })),
      [
        {
          rowNumber: 1,
          accessToken: 'token-2',
          extractionCdk: 'extract-2',
          paymentCdk: 'payment-2',
          status: 'pending',
        },
        {
          rowNumber: 2,
          accessToken: 'token-1',
          extractionCdk: 'extract-1',
          paymentCdk: 'payment-1',
          status: 'pending',
        },
      ],
    );

    const reopened = database.getBatch(batch.id);
    assert.deepEqual(reopened.tasks, batch.tasks);
  });
});

test('updateTaskStatus persists progress and updates the batch summary', async () => {
  await withDatabase(async (database) => {
    const batch = database.createBatch(rows);
    const [firstTask, secondTask] = batch.tasks;

    database.updateTaskStatus(firstTask.id, 'extracting');
    database.updateTaskStatus(secondTask.id, 'failed', {
      error: 'payment rejected',
    });

    assert.equal(database.getTask(firstTask.id).status, 'extracting');
    assert.deepEqual(
      database.getTask(secondTask.id),
      {
        ...secondTask,
        status: 'failed',
        error: 'payment rejected',
        updatedAt: database.getTask(secondTask.id).updatedAt,
      },
    );
    assert.deepEqual(database.getBatchSummary(batch.id), {
      total: 2,
      pending: 0,
      extracting: 1,
      failed: 1,
    });
  });
});

test('task progress fields preserve accepted and remaining payment work across reopen', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'local-workbench-'));
  const path = join(directory, 'workbench.sqlite');
  const firstDatabase = createDatabase(path);

  try {
    const batch = firstDatabase.createBatch(rows);
    const [acceptedTask, remainingTask] = batch.tasks;

    firstDatabase.updateTaskStatus(acceptedTask.id, 'extracting', {
      extractionTaskId: 'extraction-task-1',
      queuePosition: 4,
      workstationId: 'workstation-7',
    });
    firstDatabase.updateTaskStatus(acceptedTask.id, 'payment_pending', {
      paymentUrl: 'https://payment.example/accepted',
      paymentTaskId: 'payment-task-1',
      paymentStatus: 'pending',
    });
    firstDatabase.updateTaskStatus(remainingTask.id, 'payment_ready', {
      extractionTaskId: 'extraction-task-2',
      paymentUrl: 'https://payment.example/remaining',
      paymentStatus: 'remaining',
      queuePosition: 9,
      workstationId: 'workstation-7',
    });
    firstDatabase.updateBatchStatus(batch.id, 'running');
    firstDatabase.close();

    const reopenedDatabase = createDatabase(path);
    try {
      const [accepted, remaining] = reopenedDatabase.getUnfinishedBatches()[0]
        .tasks;

      assert.deepEqual(
        {
          status: accepted.status,
          extractionTaskId: accepted.extractionTaskId,
          paymentUrl: accepted.paymentUrl,
          paymentTaskId: accepted.paymentTaskId,
          paymentStatus: accepted.paymentStatus,
          queuePosition: accepted.queuePosition,
          workstationId: accepted.workstationId,
        },
        {
          status: 'payment_pending',
          extractionTaskId: 'extraction-task-1',
          paymentUrl: 'https://payment.example/accepted',
          paymentTaskId: 'payment-task-1',
          paymentStatus: 'pending',
          queuePosition: 4,
          workstationId: 'workstation-7',
        },
      );
      assert.deepEqual(
        {
          status: remaining.status,
          extractionTaskId: remaining.extractionTaskId,
          paymentUrl: remaining.paymentUrl,
          paymentTaskId: remaining.paymentTaskId,
          paymentStatus: remaining.paymentStatus,
          queuePosition: remaining.queuePosition,
          workstationId: remaining.workstationId,
        },
        {
          status: 'payment_ready',
          extractionTaskId: 'extraction-task-2',
          paymentUrl: 'https://payment.example/remaining',
          paymentTaskId: null,
          paymentStatus: 'remaining',
          queuePosition: 9,
          workstationId: 'workstation-7',
        },
      );
    } finally {
      reopenedDatabase.close();
    }
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test('updateTaskStatus rejects statuses outside the task whitelist', async () => {
  await withDatabase(async (database) => {
    const task = database.createBatch([rows[0]]).tasks[0];

    assert.throws(
      () => database.updateTaskStatus(task.id, 'unknown-state'),
      /invalid task status/i,
    );
    assert.equal(database.getTask(task.id).status, 'pending');
  });
});

test('updateBatchStatus rejects statuses outside the batch whitelist', async () => {
  await withDatabase(async (database) => {
    const batch = database.createBatch([rows[0]]);

    assert.throws(
      () => database.updateBatchStatus(batch.id, 'unknown-state'),
      /invalid batch status/i,
    );
    assert.equal(database.getBatch(batch.id).status, 'pending');
  });
});

test('createDatabase creates a missing parent directory', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'local-workbench-'));
  const path = join(directory, 'nested', 'data', 'workbench.sqlite');

  try {
    const database = createDatabase(path);
    try {
      assert.equal(database.createBatch([rows[0]]).tasks.length, 1);
    } finally {
      database.close();
    }
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test('createDatabase transactionally upgrades an old schema without losing data', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'local-workbench-'));
  const path = join(directory, 'legacy.sqlite');
  const legacyDatabase = new DatabaseSync(path);

  legacyDatabase.exec(`
    PRAGMA foreign_keys = ON;

    CREATE TABLE batches (
      id INTEGER PRIMARY KEY,
      status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed')),
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL
    ) STRICT;

    CREATE TABLE tasks (
      id INTEGER PRIMARY KEY,
      batch_id INTEGER NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
      row_number INTEGER NOT NULL,
      access_token TEXT NOT NULL,
      extraction_cdk TEXT NOT NULL,
      payment_cdk TEXT NOT NULL,
      status TEXT NOT NULL CHECK (status IN ('pending', 'extracting', 'completed', 'failed')),
      error TEXT,
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL,
      UNIQUE (batch_id, row_number)
    ) STRICT;

    CREATE INDEX tasks_batch_id_row_number_idx
      ON tasks (batch_id, row_number);

    CREATE TABLE settings (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL
    ) STRICT;

    INSERT INTO batches VALUES (
      41, 'running', '2026-08-02T01:02:03.000Z', '2026-08-02T01:02:04.000Z'
    );
    INSERT INTO tasks VALUES (
      73, 41, 1, 'legacy-token', 'legacy-extraction', 'legacy-payment',
      'extracting', 'legacy retry',
      '2026-08-02T01:02:03.000Z', '2026-08-02T01:02:04.000Z'
    );
    INSERT INTO settings VALUES ('helan_client', 'legacy-cookie');
  `);
  legacyDatabase.close();

  try {
    const database = createDatabase(path);
    try {
      const migratedTask = database.updateTaskStatus(73, 'payment_ready', {
        extractionTaskId: 'legacy-extraction-task',
        paymentUrl: 'https://payment.example/legacy',
        paymentStatus: 'remaining',
        queuePosition: 6,
        workstationId: 'legacy-workstation',
      });
      database.updateBatchStatus(41, 'failed');

      assert.deepEqual(
        {
          id: migratedTask.id,
          batchId: migratedTask.batchId,
          accessToken: migratedTask.accessToken,
          extractionCdk: migratedTask.extractionCdk,
          paymentCdk: migratedTask.paymentCdk,
          status: migratedTask.status,
          error: migratedTask.error,
          extractionTaskId: migratedTask.extractionTaskId,
          paymentUrl: migratedTask.paymentUrl,
          paymentStatus: migratedTask.paymentStatus,
          queuePosition: migratedTask.queuePosition,
          workstationId: migratedTask.workstationId,
          createdAt: migratedTask.createdAt,
        },
        {
          id: 73,
          batchId: 41,
          accessToken: 'legacy-token',
          extractionCdk: 'legacy-extraction',
          paymentCdk: 'legacy-payment',
          status: 'payment_ready',
          error: null,
          extractionTaskId: 'legacy-extraction-task',
          paymentUrl: 'https://payment.example/legacy',
          paymentStatus: 'remaining',
          queuePosition: 6,
          workstationId: 'legacy-workstation',
          createdAt: '2026-08-02T01:02:03.000Z',
        },
      );
      assert.equal(database.getBatch(41).status, 'failed');
      assert.equal(database.getSetting('helan_client'), 'legacy-cookie');
      database.setSetting('helan_client', 'migrated-cookie');
      assert.equal(database.getSetting('helan_client'), 'migrated-cookie');
    } finally {
      database.close();
    }

    const migratedDatabase = new DatabaseSync(path);
    try {
      assert.equal(
        migratedDatabase.prepare(`
          SELECT COUNT(*) AS count FROM sqlite_master
          WHERE type = 'index' AND name = 'tasks_batch_id_row_number_idx'
        `).get().count,
        1,
      );
      assert.throws(
        () => migratedDatabase.prepare(
          "UPDATE tasks SET status = 'invalid' WHERE id = 73",
        ).run(),
        /constraint/i,
      );
      assert.throws(
        () => migratedDatabase.prepare(
          "UPDATE batches SET status = 'invalid' WHERE id = 41",
        ).run(),
        /constraint/i,
      );
    } finally {
      migratedDatabase.close();
    }
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test('settings migration checks the updated_at default instead of other column defaults', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'local-workbench-'));
  const path = join(directory, 'legacy-settings.sqlite');
  const currentDatabase = createDatabase(path);

  currentDatabase.setSetting('helan_client', 'legacy-cookie');
  currentDatabase.close();

  const legacyDatabase = new DatabaseSync(path);
  legacyDatabase.exec(`
    ALTER TABLE settings RENAME TO settings_current;
    CREATE TABLE settings (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL,
      updated_at TEXT NOT NULL,
      category TEXT NOT NULL DEFAULT 'cookie'
    ) STRICT;
    INSERT INTO settings (key, value, updated_at)
    SELECT key, value, updated_at FROM settings_current;
    DROP TABLE settings_current;
  `);
  legacyDatabase.close();

  try {
    const database = createDatabase(path);
    try {
      database.setSetting('helan_client', 'migrated-cookie');
      assert.equal(database.getSetting('helan_client'), 'migrated-cookie');
    } finally {
      database.close();
    }
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test('createDatabase closes the connection when setup fails at any phase', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'local-workbench-'));
  const migrationPath = join(directory, 'migration.sqlite');
  const legacyDatabase = new DatabaseSync(migrationPath);

  legacyDatabase.exec(`
    CREATE TABLE batches (
      id INTEGER PRIMARY KEY,
      status TEXT NOT NULL CHECK (status IN ('pending')),
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL
    ) STRICT;
    CREATE TABLE tasks (
      id INTEGER PRIMARY KEY,
      batch_id INTEGER NOT NULL REFERENCES batches(id),
      row_number INTEGER NOT NULL,
      access_token TEXT NOT NULL,
      extraction_cdk TEXT NOT NULL,
      payment_cdk TEXT NOT NULL,
      status TEXT NOT NULL CHECK (status IN ('pending')),
      error TEXT,
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL,
      UNIQUE (batch_id, row_number)
    ) STRICT;
    CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL) STRICT;
  `);
  legacyDatabase.close();

  const scenarios = [
    {
      name: 'initialization',
      path: join(directory, 'initialization.sqlite'),
      failure: { failExec: 'PRAGMA foreign_keys = ON' },
    },
    {
      name: 'migration',
      path: migrationPath,
      failure: { failExec: 'ALTER TABLE tasks RENAME' },
    },
    {
      name: 'prepare',
      path: join(directory, 'prepare.sqlite'),
      failure: { failPrepare: 'INSERT INTO batches DEFAULT VALUES' },
    },
  ];

  try {
    for (const scenario of scenarios) {
      const tracker = createTrackedConnectionFactory(scenario.failure);
      let database;

      try {
        assert.throws(
          () => {
            database = createDatabase(scenario.path, {
              connectionFactory: tracker.factory,
            });
          },
          /forced (exec|prepare) failure/,
          scenario.name,
        );
      } finally {
        database?.close();
      }

      assert.equal(tracker.closed, true, scenario.name);
    }
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test('a non-failed task status clears a previously persisted error', async () => {
  await withDatabase(async (database) => {
    const task = database.createBatch([rows[0]]).tasks[0];

    database.updateTaskStatus(task.id, 'failed', {
      error: 'temporary upstream failure',
    });
    const retried = database.updateTaskStatus(task.id, 'extracting');

    assert.equal(retried.status, 'extracting');
    assert.equal(retried.error, null);
  });
});

test('settings persist the helan_client Cookie across database reopen', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'local-workbench-'));
  const path = join(directory, 'workbench.sqlite');
  const firstDatabase = createDatabase(path);

  try {
    assert.equal(firstDatabase.getSetting('helan_client'), null);
    firstDatabase.setSetting('helan_client', 'session-cookie-value');
    firstDatabase.close();

    const reopenedDatabase = createDatabase(path);
    try {
      assert.equal(
        reopenedDatabase.getSetting('helan_client'),
        'session-cookie-value',
      );
    } finally {
      reopenedDatabase.close();
    }
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test('getUnfinishedBatches returns pending and running batches in creation order', async () => {
  await withDatabase(async (database) => {
    const pending = database.createBatch([rows[0]]);
    const running = database.createBatch([rows[1]]);
    const completed = database.createBatch(rows);

    database.updateBatchStatus(running.id, 'running');
    database.updateBatchStatus(completed.id, 'completed');

    assert.deepEqual(
      database.getUnfinishedBatches().map((batch) => ({
        id: batch.id,
        status: batch.status,
      })),
      [
        { id: pending.id, status: 'pending' },
        { id: running.id, status: 'running' },
      ],
    );
  });
});
