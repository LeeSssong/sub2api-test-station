import { DatabaseSync } from 'node:sqlite';
import { mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';

const TIMESTAMP_SQL = "strftime('%Y-%m-%dT%H:%M:%fZ', 'now')";
const BATCH_STATUSES = new Set(['pending', 'running', 'completed', 'failed']);
const TASK_STATUSES = new Set([
  'pending',
  'extraction_queued',
  'extracting',
  'extracted',
  'payment_ready',
  'payment_submitting',
  'payment_pending',
  'completed',
  'failed',
]);
const TASK_PROGRESS_COLUMNS = [
  ['extraction_task_id', 'TEXT'],
  ['payment_url', 'TEXT'],
  ['payment_task_id', 'TEXT'],
  ['payment_status', 'TEXT'],
  ['queue_position', 'INTEGER'],
  ['workstation_id', 'TEXT'],
];

function assertStatus(statuses, status, type) {
  if (!statuses.has(status)) {
    throw new RangeError(`Invalid ${type} status: ${status}`);
  }
}

function mapTask(row) {
  if (!row) {
    return null;
  }

  return {
    id: row.id,
    batchId: row.batch_id,
    rowNumber: row.row_number,
    accessToken: row.access_token,
    extractionCdk: row.extraction_cdk,
    paymentCdk: row.payment_cdk,
    status: row.status,
    error: row.error,
    extractionTaskId: row.extraction_task_id,
    paymentUrl: row.payment_url,
    paymentTaskId: row.payment_task_id,
    paymentStatus: row.payment_status,
    queuePosition: row.queue_position,
    workstationId: row.workstation_id,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

function mapBatch(row, tasks, summary) {
  if (!row) {
    return null;
  }

  return {
    id: row.id,
    status: row.status,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
    summary,
    tasks,
  };
}

function createCurrentSchema(connection) {
  connection.exec(`
    CREATE TABLE IF NOT EXISTS batches (
      id INTEGER PRIMARY KEY,
      status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'completed', 'failed')),
      created_at TEXT NOT NULL DEFAULT (${TIMESTAMP_SQL}),
      updated_at TEXT NOT NULL DEFAULT (${TIMESTAMP_SQL})
    ) STRICT;

    CREATE TABLE IF NOT EXISTS tasks (
      id INTEGER PRIMARY KEY,
      batch_id INTEGER NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
      row_number INTEGER NOT NULL,
      access_token TEXT NOT NULL,
      extraction_cdk TEXT NOT NULL,
      payment_cdk TEXT NOT NULL,
      status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending',
        'extraction_queued',
        'extracting',
        'extracted',
        'payment_ready',
        'payment_submitting',
        'payment_pending',
        'completed',
        'failed'
      )),
      error TEXT,
      extraction_task_id TEXT,
      payment_url TEXT,
      payment_task_id TEXT,
      payment_status TEXT,
      queue_position INTEGER,
      workstation_id TEXT,
      created_at TEXT NOT NULL DEFAULT (${TIMESTAMP_SQL}),
      updated_at TEXT NOT NULL DEFAULT (${TIMESTAMP_SQL}),
      UNIQUE (batch_id, row_number)
    ) STRICT;

    CREATE INDEX IF NOT EXISTS tasks_batch_id_row_number_idx
      ON tasks (batch_id, row_number);

    CREATE TABLE IF NOT EXISTS settings (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL,
      updated_at TEXT NOT NULL DEFAULT (${TIMESTAMP_SQL})
    ) STRICT;
  `);
}

function schemaNeedsMigration(connection) {
  const selectTableSql = connection.prepare(`
    SELECT sql FROM sqlite_master
    WHERE type = 'table' AND name = ?
  `);
  const batchSql = selectTableSql.get('batches').sql;
  const taskSql = selectTableSql.get('tasks').sql;
  const taskColumns = new Set(
    connection.prepare('PRAGMA table_info(tasks)').all().map((row) => row.name),
  );
  const updatedAtColumn = connection
    .prepare('PRAGMA table_info(settings)')
    .all()
    .find((row) => row.name === 'updated_at');

  return TASK_PROGRESS_COLUMNS.some(([column]) => !taskColumns.has(column))
    || [...BATCH_STATUSES].some((status) => !batchSql.includes(`'${status}'`))
    || [...TASK_STATUSES].some((status) => !taskSql.includes(`'${status}'`))
    || updatedAtColumn?.dflt_value == null;
}

function migrateSchema(connection) {
  const taskColumns = new Set(
    connection.prepare('PRAGMA table_info(tasks)').all().map((row) => row.name),
  );
  const progressValues = TASK_PROGRESS_COLUMNS
    .map(([column]) => taskColumns.has(column) ? column : 'NULL')
    .join(', ');
  const settingColumns = new Set(
    connection.prepare('PRAGMA table_info(settings)').all().map((row) => row.name),
  );
  const settingUpdatedAt = settingColumns.has('updated_at')
    ? 'updated_at'
    : TIMESTAMP_SQL;

  connection.exec('PRAGMA foreign_keys = OFF; BEGIN IMMEDIATE');

  try {
    connection.exec(`
      ALTER TABLE tasks RENAME TO tasks_legacy;
      ALTER TABLE batches RENAME TO batches_legacy;
      ALTER TABLE settings RENAME TO settings_legacy;
    `);
    createCurrentSchema(connection);
    connection.exec(`
      INSERT INTO batches (id, status, created_at, updated_at)
      SELECT id, status, created_at, updated_at
      FROM batches_legacy;

      INSERT INTO tasks (
        id,
        batch_id,
        row_number,
        access_token,
        extraction_cdk,
        payment_cdk,
        status,
        error,
        extraction_task_id,
        payment_url,
        payment_task_id,
        payment_status,
        queue_position,
        workstation_id,
        created_at,
        updated_at
      )
      SELECT
        id,
        batch_id,
        row_number,
        access_token,
        extraction_cdk,
        payment_cdk,
        status,
        error,
        ${progressValues},
        created_at,
        updated_at
      FROM tasks_legacy;

      INSERT INTO settings (key, value, updated_at)
      SELECT key, value, ${settingUpdatedAt}
      FROM settings_legacy;

      DROP TABLE tasks_legacy;
      DROP TABLE batches_legacy;
      DROP TABLE settings_legacy;

      CREATE INDEX IF NOT EXISTS tasks_batch_id_row_number_idx
        ON tasks (batch_id, row_number);
    `);

    const foreignKeyErrors = connection.prepare('PRAGMA foreign_key_check').all();
    if (foreignKeyErrors.length > 0) {
      throw new Error('Database migration produced invalid foreign keys');
    }

    connection.exec('PRAGMA user_version = 1; COMMIT');
  } catch (error) {
    connection.exec('ROLLBACK');
    throw error;
  } finally {
    connection.exec('PRAGMA foreign_keys = ON');
  }
}

function defaultConnectionFactory(path) {
  return new DatabaseSync(path);
}

export function createDatabase(
  path,
  { connectionFactory = defaultConnectionFactory } = {},
) {
  if (path !== ':memory:') {
    mkdirSync(dirname(resolve(path)), { recursive: true });
  }

  const connection = connectionFactory(path);

  try {
    connection.exec(`
      PRAGMA foreign_keys = ON;
      PRAGMA journal_mode = WAL;
    `);
    createCurrentSchema(connection);

    if (schemaNeedsMigration(connection)) {
      migrateSchema(connection);
    } else {
      connection.exec('PRAGMA user_version = 1');
    }

  const insertBatch = connection.prepare(
    'INSERT INTO batches DEFAULT VALUES',
  );
  const insertTask = connection.prepare(`
    INSERT INTO tasks (
      batch_id,
      row_number,
      access_token,
      extraction_cdk,
      payment_cdk
    ) VALUES (?, ?, ?, ?, ?)
  `);
  const selectBatch = connection.prepare(
    'SELECT * FROM batches WHERE id = ?',
  );
  const selectTask = connection.prepare('SELECT * FROM tasks WHERE id = ?');
  const selectBatchTasks = connection.prepare(`
    SELECT * FROM tasks
    WHERE batch_id = ?
    ORDER BY row_number ASC
  `);
  const selectBatchSummary = connection.prepare(`
    SELECT status, COUNT(*) AS count
    FROM tasks
    WHERE batch_id = ?
    GROUP BY status
  `);
  const updateTaskStatusStatement = connection.prepare(`
    UPDATE tasks
    SET
      status = ?,
      error = ?,
      extraction_task_id = CASE WHEN ? THEN ? ELSE extraction_task_id END,
      payment_url = CASE WHEN ? THEN ? ELSE payment_url END,
      payment_task_id = CASE WHEN ? THEN ? ELSE payment_task_id END,
      payment_status = CASE WHEN ? THEN ? ELSE payment_status END,
      queue_position = CASE WHEN ? THEN ? ELSE queue_position END,
      workstation_id = CASE WHEN ? THEN ? ELSE workstation_id END,
      updated_at = ${TIMESTAMP_SQL}
    WHERE id = ?
  `);
  const touchBatch = connection.prepare(`
    UPDATE batches
    SET updated_at = ${TIMESTAMP_SQL}
    WHERE id = ?
  `);
  const updateBatchStatusStatement = connection.prepare(`
    UPDATE batches
    SET status = ?, updated_at = ${TIMESTAMP_SQL}
    WHERE id = ?
  `);
  const selectSetting = connection.prepare(
    'SELECT value FROM settings WHERE key = ?',
  );
  const upsertSetting = connection.prepare(`
    INSERT INTO settings (key, value)
    VALUES (?, ?)
    ON CONFLICT (key) DO UPDATE SET
      value = excluded.value,
      updated_at = ${TIMESTAMP_SQL}
  `);
  const selectUnfinishedBatches = connection.prepare(`
    SELECT * FROM batches
    WHERE status IN ('pending', 'running')
    ORDER BY id ASC
  `);

  function getBatchSummary(batchId) {
    const summary = { total: 0, pending: 0 };

    for (const row of selectBatchSummary.all(batchId)) {
      summary[row.status] = row.count;
      summary.total += row.count;
    }

    return summary;
  }

  function getBatch(batchId) {
    const row = selectBatch.get(batchId);

    if (!row) {
      return null;
    }

    const tasks = selectBatchTasks.all(batchId).map(mapTask);
    return mapBatch(row, tasks, getBatchSummary(batchId));
  }

  function createBatch(rows) {
    connection.exec('BEGIN IMMEDIATE');

    try {
      const result = insertBatch.run();
      const batchId = Number(result.lastInsertRowid);

      rows.forEach((row, index) => {
        insertTask.run(
          batchId,
          index + 1,
          row.accessToken,
          row.extractionCdk,
          row.paymentCdk,
        );
      });

      connection.exec('COMMIT');
      return getBatch(batchId);
    } catch (error) {
      connection.exec('ROLLBACK');
      throw error;
    }
  }

  function updateTaskStatus(taskId, status, details = {}) {
    assertStatus(TASK_STATUSES, status, 'task');
    connection.exec('BEGIN IMMEDIATE');

    try {
      const currentTask = selectTask.get(taskId);
      if (!currentTask) {
        connection.exec('ROLLBACK');
        return null;
      }

      const has = (field) => Object.hasOwn(details, field);
      const error = status === 'failed'
        ? (has('error') ? details.error : currentTask.error)
        : null;

      updateTaskStatusStatement.run(
        status,
        error,
        Number(has('extractionTaskId')),
        details.extractionTaskId ?? null,
        Number(has('paymentUrl')),
        details.paymentUrl ?? null,
        Number(has('paymentTaskId')),
        details.paymentTaskId ?? null,
        Number(has('paymentStatus')),
        details.paymentStatus ?? null,
        Number(has('queuePosition')),
        details.queuePosition ?? null,
        Number(has('workstationId')),
        details.workstationId ?? null,
        taskId,
      );
      touchBatch.run(currentTask.batch_id);
      connection.exec('COMMIT');
      return mapTask(selectTask.get(taskId));
    } catch (error) {
      connection.exec('ROLLBACK');
      throw error;
    }
  }

  function updateBatchStatus(batchId, status) {
    assertStatus(BATCH_STATUSES, status, 'batch');
    const result = updateBatchStatusStatement.run(status, batchId);
    return result.changes === 0 ? null : getBatch(batchId);
  }

    return {
      createBatch,
      getBatch,
      getTask(taskId) {
        return mapTask(selectTask.get(taskId));
      },
      getBatchSummary,
      updateTaskStatus,
      updateBatchStatus,
      getSetting(key) {
        return selectSetting.get(key)?.value ?? null;
      },
      setSetting(key, value) {
        upsertSetting.run(key, value);
        return value;
      },
      getUnfinishedBatches() {
        return selectUnfinishedBatches
          .all()
          .map((row) => getBatch(row.id));
      },
      close() {
        connection.close();
      },
    };
  } catch (error) {
    try {
      connection.close();
    } catch {
      // Preserve the setup error that prevented the repository from opening.
    }
    throw error;
  }
}
