const EXTRACTION_CHUNK_SIZE = 100;
const PAYMENT_CHUNK_SIZE = 500;
const PAYMENT_STATUS_CHUNK_SIZE = 100;
const DEFAULT_POLL_DELAY_MS = 2_000;
const MAX_NO_PROGRESS_PAYMENT_ATTEMPTS = 4;
const PAYMENT_RETRY_BASE_DELAY_MS = 250;
const PAYMENT_RETRY_MAX_DELAY_MS = 2_000;
const DEFAULT_MAX_POLLS = 300;
const UNKNOWN_RESULT_MESSAGE = '结果未知需核对';

function defaultSleep(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

function chunks(items, size) {
  const result = [];

  for (let index = 0; index < items.length; index += size) {
    result.push(items.slice(index, index + size));
  }

  return result;
}

function errorText(error) {
  return error instanceof Error ? error.message : String(error);
}

function submissionId(submission) {
  return submission?.id ?? submission?.taskId ?? submission?.task_id ?? null;
}

function submissionUrl(submission) {
  return submission?.paymentUrl ?? submission?.payment_url ?? null;
}

function extractionId(task) {
  return task?.taskId ?? task?.task_id ?? task?.id ?? null;
}

function statusState(item) {
  return item?.state ?? item?.status ?? 'unknown';
}

export function createBatchRunner({
  database,
  upstream,
  sleep = defaultSleep,
  onChange = () => {},
  maxPolls = DEFAULT_MAX_POLLS,
}) {
  const activeBatches = new Map();
  const pollLimit = Number.isFinite(maxPolls)
    ? Math.max(1, Math.floor(maxPolls))
    : DEFAULT_MAX_POLLS;

  function notify(batchId) {
    const batch = database.getBatch(batchId);
    try {
      void Promise.resolve(onChange(batch)).catch(() => {});
    } catch {}
    return batch;
  }

  function updateTask(taskId, status, details) {
    const task = database.updateTaskStatus(taskId, status, details);
    notify(task.batchId);
    return task;
  }

  function failTasks(tasks, error, uncertain = false) {
    const message = uncertain
      ? `${UNKNOWN_RESULT_MESSAGE}：${errorText(error)}`
      : errorText(error);

    for (const task of tasks) {
      updateTask(task.id, 'failed', { error: message });
    }
  }

  async function submitExtractions(batchId) {
    const pendingTasks = database.getBatch(batchId).tasks.filter(
      (task) => task.status === 'pending',
    );

    for (const taskChunk of chunks(pendingTasks, EXTRACTION_CHUNK_SIZE)) {
      for (const task of taskChunk) {
        updateTask(task.id, 'extraction_queued');
      }

      let response;
      try {
        response = await upstream.submitExtractions(taskChunk);
      } catch (error) {
        failTasks(taskChunk, error, error?.uncertain === true);
        continue;
      }

      const responseTasks = Array.isArray(response?.tasks) ? response.tasks : [];
      if (responseTasks.length !== taskChunk.length) {
        failTasks(
          taskChunk,
          `提链提交响应任务数量不匹配：期望 ${taskChunk.length}，实际 ${responseTasks.length}`,
        );
        continue;
      }

      for (let index = 0; index < taskChunk.length; index += 1) {
        const task = taskChunk[index];
        const submittedTask = responseTasks[index];
        const taskId = extractionId(submittedTask);

        if (!taskId) {
          updateTask(task.id, 'failed', {
            error: '提链提交响应缺少任务 ID',
          });
          continue;
        }

        updateTask(task.id, 'extracting', {
          extractionTaskId: taskId,
          queuePosition: submittedTask.queuePosition
            ?? submittedTask.queue_position
            ?? null,
          workstationId: submittedTask.workstationId
            ?? submittedTask.workstation_id
            ?? null,
        });
      }
    }
  }

  async function pollExtractionTask(task) {
    let polls = 0;

    while (true) {
      if (polls >= pollLimit) {
        updateTask(task.id, 'failed', {
          error: '轮询超时：提链任务',
        });
        return;
      }

      polls += 1;
      let result;
      try {
        result = await upstream.getExtraction(task.extractionTaskId);
      } catch (error) {
        updateTask(task.id, 'failed', { error: errorText(error) });
        return;
      }

      if (result?.status === 'processing') {
        updateTask(task.id, 'extracting');
        await sleep(result.retryAfterMs ?? DEFAULT_POLL_DELAY_MS);
        continue;
      }
      if (result?.status === 'succeeded' && result.paymentUrl) {
        updateTask(task.id, 'payment_ready', {
          paymentUrl: result.paymentUrl,
        });
        return;
      }

      updateTask(task.id, 'failed', {
        error: result?.error || '提链失败',
      });
      return;
    }
  }

  async function finishExtractions(batchId) {
    const queuedWithoutIds = database.getBatch(batchId).tasks.filter(
      (task) => task.status === 'extraction_queued' && !task.extractionTaskId,
    );
    if (queuedWithoutIds.length > 0) {
      failTasks(queuedWithoutIds, '提链提交中断', true);
    }

    const extractingTasks = database.getBatch(batchId).tasks.filter(
      (task) => (
        task.status === 'extracting'
        || task.status === 'extraction_queued'
      ) && task.extractionTaskId,
    );
    for (const task of extractingTasks) {
      await pollExtractionTask(task);
    }
  }

  function groupPaymentTasks(tasks) {
    const groups = new Map();

    for (const task of tasks) {
      const group = groups.get(task.paymentCdk) ?? [];
      group.push(task);
      groups.set(task.paymentCdk, group);
    }

    return groups;
  }

  function persistPaymentResponse(tasks, response) {
    const tasksByUrl = new Map();

    for (const task of tasks) {
      const occurrences = tasksByUrl.get(task.paymentUrl) ?? [];
      occurrences.push(task);
      tasksByUrl.set(task.paymentUrl, occurrences);
    }

    for (const submission of response?.submissions ?? []) {
      const url = submissionUrl(submission);
      const task = tasksByUrl.get(url)?.shift();
      const taskId = submissionId(submission);
      if (!task) {
        continue;
      }
      if (!taskId) {
        updateTask(task.id, 'failed', {
          error: '支付提交响应缺少任务 ID',
        });
        continue;
      }

      updateTask(task.id, 'payment_pending', {
        paymentTaskId: taskId,
        paymentStatus: statusState(submission) === 'unknown'
          ? 'queued'
          : statusState(submission),
      });
    }

    const remainingTasks = [];
    for (const url of response?.remainingPaymentUrls ?? []) {
      const task = tasksByUrl.get(url)?.shift();
      if (!task) {
        continue;
      }

      remainingTasks.push(task);
      updateTask(task.id, 'payment_ready', { paymentStatus: 'remaining' });
    }

    for (const occurrences of tasksByUrl.values()) {
      for (const task of occurrences) {
        updateTask(task.id, 'failed', {
          error: '支付提交响应缺少任务 ID',
        });
      }
    }

    return remainingTasks;
  }

  async function submitPaymentChunk(paymentCdk, initialTasks) {
    let remainingTasks = initialTasks;
    let noProgressAttempts = 0;

    while (remainingTasks.length > 0) {
      const submittedCount = remainingTasks.length;
      for (const task of remainingTasks) {
        updateTask(task.id, 'payment_submitting');
      }

      let response;
      try {
        response = await upstream.submitPayments(
          paymentCdk,
          remainingTasks.map((task) => task.paymentUrl),
        );
      } catch (error) {
        failTasks(remainingTasks, error, error?.uncertain === true);
        return;
      }

      remainingTasks = persistPaymentResponse(remainingTasks, response);
      if (remainingTasks.length === submittedCount) {
        noProgressAttempts += 1;
        if (noProgressAttempts >= MAX_NO_PROGRESS_PAYMENT_ATTEMPTS) {
          failTasks(
            remainingTasks,
            `支付提交连续 ${MAX_NO_PROGRESS_PAYMENT_ATTEMPTS} 次仍未接受`,
          );
          return;
        }

        await sleep(Math.min(
          PAYMENT_RETRY_BASE_DELAY_MS * (2 ** (noProgressAttempts - 1)),
          PAYMENT_RETRY_MAX_DELAY_MS,
        ));
      } else {
        noProgressAttempts = 0;
      }
    }
  }

  async function submitPayments(batchId) {
    const interrupted = database.getBatch(batchId).tasks.filter(
      (task) => task.status === 'payment_submitting',
    );
    if (interrupted.length > 0) {
      failTasks(interrupted, '支付提交中断', true);
    }

    const readyTasks = database.getBatch(batchId).tasks.filter(
      (task) => task.status === 'payment_ready',
    );
    for (const [paymentCdk, tasks] of groupPaymentTasks(readyTasks)) {
      for (const taskChunk of chunks(tasks, PAYMENT_CHUNK_SIZE)) {
        await submitPaymentChunk(paymentCdk, taskChunk);
      }
    }
  }

  async function pollPayments(batchId) {
    const pollCounts = new Map();

    while (true) {
      const pendingTasks = database.getBatch(batchId).tasks.filter(
        (task) => task.status === 'payment_pending',
      );
      if (pendingTasks.length === 0) {
        return;
      }

      let stillPending = false;
      for (const taskChunk of chunks(pendingTasks, PAYMENT_STATUS_CHUNK_SIZE)) {
        const pollableTasks = [];
        for (const task of taskChunk) {
          const polls = pollCounts.get(task.id) ?? 0;
          if (polls >= pollLimit) {
            updateTask(task.id, 'failed', {
              error: '轮询超时：支付任务',
            });
          } else {
            pollCounts.set(task.id, polls + 1);
            pollableTasks.push(task);
          }
        }

        if (pollableTasks.length === 0) {
          continue;
        }

        let response;
        try {
          response = await upstream.getPaymentStatuses(
            pollableTasks.map((task) => task.paymentTaskId),
          );
        } catch (error) {
          failTasks(pollableTasks, error);
          continue;
        }

        const itemsById = new Map(
          (response?.items ?? []).map((item) => [submissionId(item), item]),
        );
        for (const task of pollableTasks) {
          const item = itemsById.get(task.paymentTaskId);
          const state = item ? statusState(item) : 'unknown';

          if (state === 'completed') {
            updateTask(task.id, 'completed', { paymentStatus: state });
          } else if (state === 'queued' || state === 'pending') {
            stillPending = true;
            updateTask(task.id, 'payment_pending', { paymentStatus: state });
          } else {
            updateTask(task.id, 'failed', {
              paymentStatus: state,
              error: `支付任务终态失败：${state}`,
            });
          }
        }
      }

      if (stillPending) {
        await sleep(DEFAULT_POLL_DELAY_MS);
      }
    }
  }

  async function run(batchId) {
    const batch = database.getBatch(batchId);
    if (!batch) {
      throw new Error(`Batch not found: ${batchId}`);
    }
    if (batch.status === 'completed' || batch.status === 'failed') {
      return batch;
    }

    database.updateBatchStatus(batchId, 'running');
    notify(batchId);
    await submitExtractions(batchId);
    await finishExtractions(batchId);
    await submitPayments(batchId);
    await pollPayments(batchId);

    const finalBatch = database.getBatch(batchId);
    const finalStatus = finalBatch.tasks.some((task) => task.status === 'failed')
      ? 'failed'
      : 'completed';
    database.updateBatchStatus(batchId, finalStatus);
    return notify(batchId);
  }

  function start(batchId) {
    if (activeBatches.has(batchId)) {
      return activeBatches.get(batchId);
    }

    const promise = run(batchId).finally(() => {
      activeBatches.delete(batchId);
    });
    activeBatches.set(batchId, promise);
    return promise;
  }

  async function resume() {
    return Promise.all(
      database.getUnfinishedBatches().map((batch) => start(batch.id)),
    );
  }

  return Object.freeze({ start, resume });
}
