import assert from 'node:assert/strict';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';

import { createBatchRunner } from '../src/batch-runner.js';
import { createDatabase } from '../src/database.js';
import { UpstreamError } from '../src/upstream-client.js';

async function withDatabase(run) {
  const directory = await mkdtemp(join(tmpdir(), 'batch-runner-'));
  const database = createDatabase(join(directory, 'workbench.sqlite'));

  try {
    await run(database);
  } finally {
    database.close();
    await rm(directory, { recursive: true, force: true });
  }
}

function rows(count, paymentCdk = 'payment-cdk') {
  return Array.from({ length: count }, (_, index) => ({
    accessToken: `access-${index + 1}`,
    extractionCdk: `extract-${index + 1}`,
    paymentCdk,
  }));
}

function extractionTasks(submittedRows) {
  return submittedRows.map((row) => ({
    taskId: `extraction-${row.accessToken}`,
    queuePosition: Number(row.accessToken.slice('access-'.length)),
    workstationId: 'workstation-1',
  }));
}

function paymentSubmissions(urls) {
  return urls.map((url) => ({
    id: `payment-${url.slice(url.lastIndexOf('/') + 1)}`,
    payment_url: url,
  }));
}

test('runs the happy path in 100-row chunks and binds extraction responses by order', async () => {
  await withDatabase(async (database) => {
    const submittedChunks = [];
    const statusChunks = [];
    const changed = [];
    const upstream = {
      async submitExtractions(submittedRows) {
        submittedChunks.push(submittedRows.map((row) => row.accessToken));
        return { tasks: extractionTasks(submittedRows) };
      },
      async getExtraction(taskId) {
        return {
          status: 'succeeded',
          paymentUrl: `https://pay.example/${taskId.slice('extraction-access-'.length)}`,
        };
      },
      async submitPayments(paymentCdk, urls) {
        assert.equal(paymentCdk, 'payment-cdk');
        return { submissions: paymentSubmissions(urls) };
      },
      async getPaymentStatuses(ids) {
        statusChunks.push([...ids]);
        return {
          items: ids.map((id) => ({ id, state: 'completed' })),
        };
      },
    };
    const batch = database.createBatch(rows(101));
    const runner = createBatchRunner({
      database,
      upstream,
      sleep: async () => {},
      onChange: (updatedBatch) => changed.push(updatedBatch.status),
    });

    const result = await runner.start(batch.id);

    assert.deepEqual(submittedChunks.map((chunk) => chunk.length), [100, 1]);
    assert.deepEqual(submittedChunks[1], ['access-101']);
    assert.deepEqual(statusChunks.map((chunk) => chunk.length), [100, 1]);
    assert.equal(result.status, 'completed');
    assert.equal(result.summary.completed, 101);
    assert.equal(result.tasks[100].extractionTaskId, 'extraction-access-101');
    assert.equal(result.tasks[100].paymentTaskId, 'payment-101');
    assert.equal(result.tasks[100].paymentStatus, 'completed');
    assert.ok(changed.includes('running'));
    assert.equal(changed.at(-1), 'completed');
  });
});

test('stops a task when extraction returns plain-text failure', async () => {
  await withDatabase(async (database) => {
    let paymentCalls = 0;
    const batch = database.createBatch(rows(1));
    const runner = createBatchRunner({
      database,
      sleep: async () => {},
      upstream: {
        async submitExtractions(submittedRows) {
          return { tasks: extractionTasks(submittedRows) };
        },
        async getExtraction() {
          return { status: 'failed', error: '兑换码无效' };
        },
        async submitPayments() {
          paymentCalls += 1;
          return { submissions: [] };
        },
        async getPaymentStatuses() {
          return { items: [] };
        },
      },
    });

    const result = await runner.start(batch.id);

    assert.equal(result.status, 'failed');
    assert.equal(result.tasks[0].status, 'failed');
    assert.equal(result.tasks[0].error, '兑换码无效');
    assert.equal(paymentCalls, 0);
  });
});

test('persists accepted 207 submissions and retries only remaining payment URLs', async () => {
  await withDatabase(async (database) => {
    const paymentCalls = [];
    const batch = database.createBatch(rows(2));
    const runner = createBatchRunner({
      database,
      sleep: async () => {},
      upstream: {
        async submitExtractions(submittedRows) {
          return { tasks: extractionTasks(submittedRows) };
        },
        async getExtraction(taskId) {
          return {
            status: 'succeeded',
            paymentUrl: `https://pay.example/${taskId.endsWith('1') ? '1' : '2'}`,
          };
        },
        async submitPayments(paymentCdk, urls) {
          paymentCalls.push([...urls]);
          if (paymentCalls.length === 1) {
            return {
              submissions: paymentSubmissions([urls[0]]),
              remainingPaymentUrls: [urls[1]],
              partial: true,
            };
          }
          assert.equal(
            database.getBatch(batch.id).tasks[0].status,
            'payment_pending',
          );
          return { submissions: paymentSubmissions(urls) };
        },
        async getPaymentStatuses(ids) {
          return { items: ids.map((id) => ({ id, state: 'completed' })) };
        },
      },
    });

    const result = await runner.start(batch.id);

    assert.deepEqual(paymentCalls, [
      ['https://pay.example/1', 'https://pay.example/2'],
      ['https://pay.example/2'],
    ]);
    assert.equal(result.status, 'completed');
    assert.equal(result.summary.completed, 2);
  });
});

test('polls queued and pending payments in chunks until completed', async () => {
  await withDatabase(async (database) => {
    const sleeps = [];
    let polls = 0;
    const extractionPolls = new Map();
    const batch = database.createBatch(rows(2));
    const runner = createBatchRunner({
      database,
      sleep: async (milliseconds) => sleeps.push(milliseconds),
      upstream: {
        async submitExtractions(submittedRows) {
          return { tasks: extractionTasks(submittedRows) };
        },
        async getExtraction(taskId) {
          const attempts = extractionPolls.get(taskId) ?? 0;
          extractionPolls.set(taskId, attempts + 1);
          if (attempts === 0) {
            return { status: 'processing', retryAfterMs: 17 };
          }
          return {
            status: 'succeeded',
            paymentUrl: `https://pay.example/${taskId.endsWith('1') ? '1' : '2'}`,
          };
        },
        async submitPayments(paymentCdk, urls) {
          return { submissions: paymentSubmissions(urls) };
        },
        async getPaymentStatuses(ids) {
          polls += 1;
          const state = polls === 1 ? ['queued', 'pending'] : ['completed', 'completed'];
          return {
            items: ids.map((id, index) => ({ id, state: state[index] })),
          };
        },
      },
    });

    const result = await runner.start(batch.id);

    assert.equal(polls, 2);
    assert.ok(sleeps.includes(17));
    assert.ok(sleeps.some((milliseconds) => milliseconds > 0));
    assert.equal(result.status, 'completed');
  });
});

test('resume restores persisted stages without repeating submitted work', async () => {
  await withDatabase(async (database) => {
    const firstBatch = database.createBatch(rows(3));
    const [extracting, paymentReady, paymentPending] = firstBatch.tasks;
    database.updateTaskStatus(extracting.id, 'extracting', {
      extractionTaskId: 'extraction-resume',
    });
    database.updateTaskStatus(paymentReady.id, 'payment_ready', {
      extractionTaskId: 'extraction-ready',
      paymentUrl: 'https://pay.example/ready',
    });
    database.updateTaskStatus(paymentPending.id, 'payment_pending', {
      extractionTaskId: 'extraction-paid',
      paymentUrl: 'https://pay.example/pending',
      paymentTaskId: 'payment-pending',
      paymentStatus: 'pending',
    });
    database.updateBatchStatus(firstBatch.id, 'running');

    const secondBatch = database.createBatch(rows(1, 'other-payment-cdk'));
    const calls = { extractions: 0, payments: [], statuses: [] };
    const runner = createBatchRunner({
      database,
      sleep: async () => {},
      upstream: {
        async submitExtractions(submittedRows) {
          calls.extractions += 1;
          return { tasks: extractionTasks(submittedRows) };
        },
        async getExtraction(taskId) {
          return {
            status: 'succeeded',
            paymentUrl: taskId === 'extraction-resume'
              ? 'https://pay.example/resume'
              : `https://pay.example/${taskId.slice('extraction-access-'.length)}`,
          };
        },
        async submitPayments(paymentCdk, urls) {
          calls.payments.push({ paymentCdk, urls: [...urls] });
          return { submissions: paymentSubmissions(urls) };
        },
        async getPaymentStatuses(ids) {
          calls.statuses.push([...ids]);
          return { items: ids.map((id) => ({ id, state: 'completed' })) };
        },
      },
    });

    const results = await runner.resume();

    assert.equal(calls.extractions, 1);
    assert.deepEqual(calls.payments, [
      {
        paymentCdk: 'payment-cdk',
        urls: ['https://pay.example/resume', 'https://pay.example/ready'],
      },
      {
        paymentCdk: 'other-payment-cdk',
        urls: ['https://pay.example/1'],
      },
    ]);
    assert.deepEqual(calls.statuses.flat().sort(), [
      'payment-1',
      'payment-pending',
      'payment-ready',
      'payment-resume',
    ]);
    assert.deepEqual(results.map((batch) => batch.status), [
      'completed',
      'completed',
    ]);
    assert.equal(
      database.getTask(paymentPending.id).paymentTaskId,
      'payment-pending',
    );
  });
});

test('does not automatically resubmit an uncertain payment request', async () => {
  await withDatabase(async (database) => {
    let paymentCalls = 0;
    const batch = database.createBatch(rows(1));
    const runner = createBatchRunner({
      database,
      sleep: async () => {},
      upstream: {
        async submitExtractions(submittedRows) {
          return { tasks: extractionTasks(submittedRows) };
        },
        async getExtraction() {
          return { status: 'succeeded', paymentUrl: 'https://pay.example/1' };
        },
        async submitPayments() {
          paymentCalls += 1;
          throw new UpstreamError('connection reset', { uncertain: true });
        },
        async getPaymentStatuses() {
          return { items: [] };
        },
      },
    });

    const result = await runner.start(batch.id);
    await runner.resume();

    assert.equal(paymentCalls, 1);
    assert.equal(result.status, 'failed');
    assert.equal(result.tasks[0].status, 'failed');
    assert.match(result.tasks[0].error, /结果未知需核对/);
  });
});

test('deduplicates concurrent start calls for the same batch', async () => {
  await withDatabase(async (database) => {
    let releaseSubmission;
    let extractionCalls = 0;
    const submissionBlocked = new Promise((resolve) => {
      releaseSubmission = resolve;
    });
    const batch = database.createBatch(rows(1));
    const runner = createBatchRunner({
      database,
      sleep: async () => {},
      upstream: {
        async submitExtractions(submittedRows) {
          extractionCalls += 1;
          await submissionBlocked;
          return { tasks: extractionTasks(submittedRows) };
        },
        async getExtraction() {
          return { status: 'failed', error: 'stop' };
        },
        async submitPayments() {
          return { submissions: [] };
        },
        async getPaymentStatuses() {
          return { items: [] };
        },
      },
    });

    const first = runner.start(batch.id);
    const duplicate = runner.start(batch.id);

    assert.strictEqual(duplicate, first);
    releaseSubmission();
    await first;
    assert.equal(extractionCalls, 1);
  });
});

test('preserves duplicate payment URL occurrences across accepted and remaining results', async () => {
  await withDatabase(async (database) => {
    const duplicateUrl = 'https://pay.example/duplicate';
    const paymentCalls = [];
    const batch = database.createBatch(rows(2));
    const runner = createBatchRunner({
      database,
      sleep: async () => {},
      upstream: {
        async submitExtractions(submittedRows) {
          return { tasks: extractionTasks(submittedRows) };
        },
        async getExtraction() {
          return { status: 'succeeded', paymentUrl: duplicateUrl };
        },
        async submitPayments(paymentCdk, urls) {
          paymentCalls.push([...urls]);
          if (paymentCalls.length === 1) {
            return {
              submissions: [{ id: 'payment-accepted', payment_url: duplicateUrl }],
              remainingPaymentUrls: [duplicateUrl],
              partial: true,
            };
          }
          return {
            submissions: [{ id: 'payment-retried', payment_url: duplicateUrl }],
          };
        },
        async getPaymentStatuses(ids) {
          return { items: ids.map((id) => ({ id, state: 'completed' })) };
        },
      },
    });

    const result = await runner.start(batch.id);

    assert.deepEqual(paymentCalls, [
      [duplicateUrl, duplicateUrl],
      [duplicateUrl],
    ]);
    assert.deepEqual(
      result.tasks.map((task) => task.paymentTaskId),
      ['payment-accepted', 'payment-retried'],
    );
    assert.equal(result.status, 'completed');
  });
});

test('fails an entire extraction chunk when response task count does not match', async () => {
  await withDatabase(async (database) => {
    let extractionPolls = 0;
    const batch = database.createBatch(rows(2));
    const runner = createBatchRunner({
      database,
      sleep: async () => {},
      upstream: {
        async submitExtractions(submittedRows) {
          return { tasks: extractionTasks(submittedRows.slice(0, 1)) };
        },
        async getExtraction() {
          extractionPolls += 1;
          return { status: 'succeeded', paymentUrl: 'https://pay.example/1' };
        },
        async submitPayments() {
          return { submissions: [] };
        },
        async getPaymentStatuses() {
          return { items: [] };
        },
      },
    });

    const result = await runner.start(batch.id);

    assert.equal(extractionPolls, 0);
    assert.equal(result.status, 'failed');
    assert.deepEqual(result.tasks.map((task) => task.status), ['failed', 'failed']);
    assert.deepEqual(
      result.tasks.map((task) => task.extractionTaskId),
      [null, null],
    );
    assert.match(result.tasks[0].error, /数量不匹配/);
  });
});

test('bounds no-progress 207 retries with exponential backoff', async () => {
  await withDatabase(async (database) => {
    const delays = [];
    let paymentCalls = 0;
    const paymentUrl = 'https://pay.example/stuck';
    const batch = database.createBatch(rows(1));
    const runner = createBatchRunner({
      database,
      sleep: async (milliseconds) => delays.push(milliseconds),
      upstream: {
        async submitExtractions(submittedRows) {
          return { tasks: extractionTasks(submittedRows) };
        },
        async getExtraction() {
          return { status: 'succeeded', paymentUrl };
        },
        async submitPayments() {
          paymentCalls += 1;
          if (paymentCalls > 4) {
            throw new Error('payment retry exceeded the bounded attempt count');
          }
          return {
            submissions: [],
            remainingPaymentUrls: [paymentUrl],
            partial: true,
          };
        },
        async getPaymentStatuses() {
          return { items: [] };
        },
      },
    });

    const result = await runner.start(batch.id);

    assert.equal(paymentCalls, 4);
    assert.deepEqual(delays, [250, 500, 1000]);
    assert.equal(result.status, 'failed');
    assert.equal(result.tasks[0].status, 'failed');
    assert.match(result.tasks[0].error, /仍未接受/);
  });
});

test('resume starts later batches while an earlier batch remains pending', async () => {
  await withDatabase(async (database) => {
    const firstBatch = database.createBatch(rows(1));
    const firstTask = firstBatch.tasks[0];
    database.updateTaskStatus(firstTask.id, 'payment_pending', {
      extractionTaskId: 'extraction-stuck',
      paymentUrl: 'https://pay.example/stuck',
      paymentTaskId: 'payment-stuck',
      paymentStatus: 'pending',
    });
    database.updateBatchStatus(firstBatch.id, 'running');
    database.createBatch(rows(1, 'second-cdk'));

    let firstCompleted = false;
    let releaseFirstSleep;
    let markFirstSleeping;
    const firstSleeping = new Promise((resolve) => {
      markFirstSleeping = resolve;
    });
    let secondExtractionCalls = 0;
    const runner = createBatchRunner({
      database,
      sleep: async () => new Promise((resolve) => {
        releaseFirstSleep = resolve;
        markFirstSleeping();
      }),
      upstream: {
        async submitExtractions(submittedRows) {
          secondExtractionCalls += 1;
          return { tasks: extractionTasks(submittedRows) };
        },
        async getExtraction() {
          return { status: 'succeeded', paymentUrl: 'https://pay.example/second' };
        },
        async submitPayments(paymentCdk, urls) {
          return { submissions: [{ id: 'payment-second', payment_url: urls[0] }] };
        },
        async getPaymentStatuses(ids) {
          return {
            items: ids.map((id) => ({
              id,
              state: id === 'payment-stuck' && !firstCompleted
                ? 'pending'
                : 'completed',
            })),
          };
        },
      },
    });

    const resumed = runner.resume();
    await firstSleeping;
    await Promise.resolve();
    const callsBeforeFirstCompleted = secondExtractionCalls;
    firstCompleted = true;
    releaseFirstSleep();
    const results = await resumed;

    assert.equal(callsBeforeFirstCompleted, 1);
    assert.deepEqual(results.map((batch) => batch.status), [
      'completed',
      'completed',
    ]);
  });
});

test('isolates onChange errors from batch execution', async () => {
  await withDatabase(async (database) => {
    let notifications = 0;
    const batch = database.createBatch(rows(1));
    const runner = createBatchRunner({
      database,
      sleep: async () => {},
      onChange() {
        notifications += 1;
        throw new Error('observer failed');
      },
      upstream: {
        async submitExtractions(submittedRows) {
          return { tasks: extractionTasks(submittedRows) };
        },
        async getExtraction() {
          return { status: 'failed', error: 'expected terminal failure' };
        },
        async submitPayments() {
          return { submissions: [] };
        },
        async getPaymentStatuses() {
          return { items: [] };
        },
      },
    });

    const result = await runner.start(batch.id);

    assert.ok(notifications > 1);
    assert.equal(result.status, 'failed');
    assert.equal(result.tasks[0].error, 'expected terminal failure');
  });
});

test('fails a permanently processing extraction after maxPolls', async () => {
  await withDatabase(async (database) => {
    let extractionPolls = 0;
    const batch = database.createBatch(rows(1));
    const runner = createBatchRunner({
      database,
      maxPolls: 2,
      sleep: async () => {},
      upstream: {
        async submitExtractions(submittedRows) {
          return { tasks: extractionTasks(submittedRows) };
        },
        async getExtraction() {
          extractionPolls += 1;
          if (extractionPolls > 2) {
            throw new Error('polling continued past extraction limit');
          }
          return { status: 'processing', retryAfterMs: 0 };
        },
        async submitPayments() {
          return { submissions: [] };
        },
        async getPaymentStatuses() {
          return { items: [] };
        },
      },
    });

    const result = await runner.start(batch.id);

    assert.equal(extractionPolls, 2);
    assert.equal(result.status, 'failed');
    assert.equal(result.tasks[0].status, 'failed');
    assert.match(result.tasks[0].error, /轮询超时/);
  });
});

test('fails a permanently pending payment after maxPolls', async () => {
  await withDatabase(async (database) => {
    let paymentPolls = 0;
    const batch = database.createBatch(rows(1));
    const runner = createBatchRunner({
      database,
      maxPolls: 2,
      sleep: async () => {},
      upstream: {
        async submitExtractions(submittedRows) {
          return { tasks: extractionTasks(submittedRows) };
        },
        async getExtraction() {
          return { status: 'succeeded', paymentUrl: 'https://pay.example/pending' };
        },
        async submitPayments(paymentCdk, urls) {
          return { submissions: [{ id: 'payment-pending', payment_url: urls[0] }] };
        },
        async getPaymentStatuses() {
          paymentPolls += 1;
          if (paymentPolls > 2) {
            throw new Error('polling continued past payment limit');
          }
          return { items: [{ id: 'payment-pending', state: 'pending' }] };
        },
      },
    });

    const result = await runner.start(batch.id);

    assert.equal(paymentPolls, 2);
    assert.equal(result.status, 'failed');
    assert.equal(result.tasks[0].status, 'failed');
    assert.match(result.tasks[0].error, /轮询超时/);
  });
});
