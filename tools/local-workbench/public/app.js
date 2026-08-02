import { parseTsvRows } from './row-parser.js';

const state = { batches: [], currentBatch: null, rows: [emptyRow()] };
const elements = {
  rows: document.querySelector('#entry-rows'),
  pasteInput: document.querySelector('#paste-input'),
  error: document.querySelector('#form-error'),
  start: document.querySelector('#start-batch'),
  history: document.querySelector('#batch-history'),
  historyCount: document.querySelector('#history-count'),
  title: document.querySelector('#detail-heading'),
  status: document.querySelector('#batch-status'),
  summary: document.querySelector('#summary'),
  empty: document.querySelector('#empty-detail'),
  tasks: document.querySelector('#task-detail'),
  taskList: document.querySelector('#task-list'),
};

function emptyRow() {
  return { accessToken: '', extractionCdk: '', paymentCdk: '' };
}

function statusText(status) {
  return ({ pending: '等待中', running: '运行中', completed: '已完成', failed: '失败' })[status] ?? status;
}

function formatTime(timestamp) {
  return timestamp ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'short', timeStyle: 'short' }).format(new Date(timestamp)) : '';
}

function iconRefresh() {
  window.lucide?.createIcons();
}

function renderRows() {
  elements.rows.replaceChildren(...state.rows.map((row, rowIndex) => {
    const tableRow = document.createElement('tr');
    for (const field of ['accessToken', 'extractionCdk', 'paymentCdk']) {
      const cell = document.createElement('td');
      const input = document.createElement('input');
      input.className = 'cell-input';
      input.value = row[field];
      input.autocomplete = 'off';
      input.setAttribute('aria-label', `${field} 第 ${rowIndex + 1} 行`);
      input.addEventListener('input', () => { state.rows[rowIndex][field] = input.value; });
      cell.append(input);
      tableRow.append(cell);
    }
    const action = document.createElement('td');
    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'icon-button row-delete';
    remove.title = `删除第 ${rowIndex + 1} 行`;
    remove.setAttribute('aria-label', remove.title);
    remove.innerHTML = '<i data-lucide="trash-2"></i>';
    remove.addEventListener('click', () => {
      state.rows.splice(rowIndex, 1);
      if (state.rows.length === 0) state.rows.push(emptyRow());
      renderRows();
    });
    action.append(remove);
    tableRow.append(action);
    return tableRow;
  }));
  iconRefresh();
}

function renderHistory() {
  elements.historyCount.textContent = String(state.batches.length);
  elements.history.replaceChildren(...state.batches.map((batch) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'batch-item';
    button.setAttribute('aria-current', String(batch.id === state.currentBatch?.id));
    button.innerHTML = `<span><strong>批次 #${batch.id}</strong><small>${formatTime(batch.createdAt)}</small></span><span class="status status-${batch.status}">${statusText(batch.status)}</span>`;
    button.addEventListener('click', () => selectBatch(batch.id));
    return button;
  }));
}

function taskPhase(task, phase) {
  if (phase === '提链') {
    return [task.extractionTaskId || '未开始', task.queuePosition ? `队列 ${task.queuePosition}` : task.error].filter(Boolean);
  }
  return [task.paymentUrl || task.paymentTaskId || '未开始', task.paymentStatus].filter(Boolean);
}

function renderCurrentBatch() {
  const batch = state.currentBatch;
  elements.empty.hidden = Boolean(batch);
  elements.tasks.hidden = !batch;
  elements.summary.hidden = !batch;
  if (!batch) return;
  elements.title.textContent = `批次 #${batch.id}`;
  elements.status.textContent = statusText(batch.status);
  elements.status.className = `status status-${batch.status}`;
  elements.summary.replaceChildren(...Object.entries(batch.summary).map(([key, value]) => {
    const item = document.createElement('span');
    item.textContent = `${key}: ${value}`;
    return item;
  }));
  elements.taskList.replaceChildren(...batch.tasks.map((task) => {
    const row = document.createElement('div');
    row.className = 'task-row';
    const extraction = taskPhase(task, '提链');
    const payment = taskPhase(task, '支付');
    row.innerHTML = `<span class="task-id">${task.rowNumber}</span><span><span class="phase">${extraction[0]}</span>${extraction[1] ? `<span class="phase-note">${extraction[1]}</span>` : ''}</span><span><span class="phase">${payment[0]}</span>${payment[1] ? `<span class="phase-note">${payment[1]}</span>` : ''}</span><span class="final-status ${task.status}">${statusText(task.status)}</span>`;
    return row;
  }));
}

async function request(path, options) {
  const response = await fetch(path, options);
  const body = await response.json();
  if (!response.ok) throw body;
  return body;
}

async function loadHistory() {
  const { batches } = await request('/api/batches');
  state.batches = batches;
  renderHistory();
}

async function selectBatch(id) {
  const { batch } = await request(`/api/batches/${id}`);
  state.currentBatch = batch;
  renderHistory();
  renderCurrentBatch();
}

function showError(message = '') {
  elements.error.hidden = !message;
  elements.error.textContent = message;
}

document.querySelector('#add-row').addEventListener('click', () => {
  state.rows.push(emptyRow());
  renderRows();
});

elements.pasteInput.addEventListener('paste', (event) => {
  const text = event.clipboardData?.getData('text/plain') ?? '';
  const { rows, errors } = parseTsvRows(text);
  if (errors.length > 0) {
    event.preventDefault();
    showError(errors.map((item) => `第 ${item.row} 行：${item.message}`).join('；'));
    return;
  }
  if (rows.length > 0) {
    event.preventDefault();
    state.rows = rows;
    elements.pasteInput.value = '';
    showError();
    renderRows();
  }
});

elements.start.addEventListener('click', async () => {
  showError();
  elements.start.disabled = true;
  try {
    const { batch } = await request('/api/batches', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ rows: state.rows }),
    });
    state.rows = [emptyRow()];
    state.currentBatch = batch;
    renderRows();
    await loadHistory();
    renderCurrentBatch();
  } catch (error) {
    showError(error.errors?.map((item) => `第 ${item.row} 行：${item.message}`).join('；') || error.error || '请求失败');
  } finally {
    elements.start.disabled = false;
  }
});

setInterval(async () => {
  try {
    await loadHistory();
    if (state.currentBatch) await selectBatch(state.currentBatch.id);
  } catch {}
}, 3000);

renderRows();
renderCurrentBatch();
loadHistory().catch((error) => showError(error.error || '无法加载批次'));
iconRefresh();
