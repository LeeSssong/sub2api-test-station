(function () {
  'use strict'

  var UPDATE_LABELS = ['立即更新', 'Update Now']
  var UPDATE_PATH = '/api/v1/admin/system/update'
  var STATUS_PATH = '/api/v1/admin/system/host-update/status'
  var SCHEDULE_PATH = '/api/v1/admin/system/host-update/schedule'
  var READINESS_PATH = '/api/v1/admin/system/host-update/readiness'
  var READINESS_POLL_INTERVAL_MS = 30000
  var POLL_INTERVAL_MS = 3000
  var TERMINAL_STAGES = ['promoted', 'rolled_back', 'failed', 'rollback_failed']
  var TOAST_DURATION_MS = 5000
  var STEP_LABELS = {
    inspect: '检查运行环境',
    'verify-image': '校验目标镜像',
    'backup-db': '备份数据库',
    'backup-counts': '记录数据基线',
    'backup-app-data': '备份应用数据',
    checksum: '校验备份完整性',
    'compose-validate': '校验编排配置',
    'recreate-sub2api': '切换应用容器',
    health: '等待健康检查',
    smoke: '冒烟验证',
    promoted: '升级完成',
    rolled_back: '已回滚到上一版本',
    rollback_failed: '回滚失败',
  }
  var originalFetch = window.fetch.bind(window)
  var state = {
    dialog: null,
    info: null,
    existing: null,
    opening: null,
    pending: false,
    replacing: false,
    upgrading: false,
    pollTimer: null,
    candidateReady: false,
    readinessTarget: '',
    readinessTimer: null,
    readinessGeneration: 0,
  }

  function text(value) {
    return value == null ? '' : String(value).trim()
  }

  function authHeaders(withJSON) {
    var headers = { 'X-Admin-UI-Request': '1' }
    var token = text(window.localStorage && window.localStorage.getItem('auth_token'))
    if (token) headers.Authorization = 'Bearer ' + token
    if (withJSON) headers['Content-Type'] = 'application/json'
    return headers
  }

  function makeError(payload, status) {
    var data = payload && typeof payload === 'object' ? (payload.data || payload) : {}
    var code = text(payload && payload.code || data && data.code)
    var fallback = code === 'UPDATE_CANDIDATE_NOT_READY'
      ? '候选版本正在准备，暂不可升级'
      : '更新服务不可用'
    var error = new Error(text(data.message || payload && payload.message || fallback))
    error.code = code
    error.status = status || 0
    return error
  }

  async function readJSON(response) {
    var payload = {}
    try {
      payload = await response.json()
    } catch (_) {
      payload = {}
    }
    if (!response.ok || payload.code && payload.code !== 0) throw makeError(payload, response.status)
    return payload
  }

  async function apiRequest(path, options) {
    var request = Object.assign({ credentials: 'same-origin' }, options || {})
    request.headers = Object.assign({}, authHeaders(Boolean(request.body)), request.headers || {})
    try {
      return readJSON(await originalFetch(path, request))
    } catch (error) {
      if (typeof error.status !== 'number') error.status = 0
      throw error
    }
  }

  function payloadData(payload) {
    return payload && payload.data && typeof payload.data === 'object' ? payload.data : (payload || {})
  }

  async function getUpdateInfo() {
    var payload = payloadData(await apiRequest('/api/v1/admin/system/check-updates'))
    return {
      current: text(payload.current_version || payload.currentVersion || '未知'),
      target: text(payload.latest_version || payload.latestVersion || '未知'),
    }
  }

  async function getStatus() {
    try {
      var payload = await apiRequest(STATUS_PATH)
      if (payload && payload.data == null) return null
      return payloadData(payload)
    } catch (error) {
      if (error.status === 404 || error.code === 'UPDATE_NO_OPERATION') return null
      throw error
    }
  }

  async function getReadiness(target) {
    return payloadData(await apiRequest(
      READINESS_PATH + '?target_version=' + encodeURIComponent(target),
    ))
  }

  function shanghaiParts(date) {
    var parts = new Intl.DateTimeFormat('en-CA', {
      timeZone: 'Asia/Shanghai',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    }).formatToParts(date)
    var result = {}
    parts.forEach(function (part) {
      if (part.type !== 'literal') result[part.type] = part.value
    })
    return result
  }

  function formatShanghaiInput(date) {
    var parts = shanghaiParts(date)
    return parts.year + '-' + parts.month + '-' + parts.day + 'T' + parts.hour + ':' + parts.minute
  }

  function formatShanghaiDisplay(value) {
    var date = new Date(value)
    if (Number.isNaN(date.getTime())) return '北京时间待定'
    var parts = shanghaiParts(date)
    return parts.year + '年' + parts.month + '月' + parts.day + '日 ' + parts.hour + ':' + parts.minute
  }

  function shanghaiLocalToUTC(value) {
    var match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(text(value))
    if (!match) throw new Error('请选择有效的北京时间')
    var localAsUTC = Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]), Number(match[4]), Number(match[5]))
    var shown = shanghaiParts(new Date(localAsUTC))
    var shownAsUTC = Date.UTC(Number(shown.year), Number(shown.month) - 1, Number(shown.day), Number(shown.hour), Number(shown.minute))
    return new Date(localAsUTC - (shownAsUTC - localAsUTC)).toISOString()
  }

  function isUpdateButton(element) {
    if (!element || !element.closest) return false
    var button = element.closest('button, [role="button"]')
    if (!button || button.disabled || button.getAttribute('aria-disabled') === 'true') return false
    var label = text(button.getAttribute('aria-label') || button.textContent).replace(/\s+/g, ' ')
    return UPDATE_LABELS.indexOf(label) !== -1
  }

  function setText(node, value) {
    node.textContent = text(value)
  }

  function button(label, action, variant) {
    var node = document.createElement('button')
    node.type = 'button'
    node.className = 'xq-update-ui-button'
    node.dataset.action = action
    if (variant) node.dataset.variant = variant
    setText(node, label)
    return node
  }

  function closeDialog() {
    stopPolling()
    stopReadinessPolling()
    if (state.dialog) state.dialog.remove()
    state.dialog = null
    state.info = null
    state.existing = null
    state.replacing = false
    state.upgrading = false
    state.candidateReady = false
    state.readinessTarget = ''
    state.readinessGeneration += 1
  }

  function showToast(message) {
    var toast = document.createElement('div')
    toast.className = 'xq-update-ui-toast'
    toast.setAttribute('role', 'status')
    setText(toast, message)
    document.body.appendChild(toast)
    window.setTimeout(function () {
      toast.remove()
    }, TOAST_DURATION_MS)
  }

  function renderProgressLog(events) {
    if (!state.dialog) return
    var log = state.dialog.querySelector('[data-role="progress-log"]')
    if (!log) return
    log.hidden = !events || !events.length
    if (log.hidden) return
    log.textContent = ''
    events.forEach(function (event, index) {
      var line = document.createElement('p')
      var done = index < events.length - 1
      line.dataset.state = done ? 'done' : 'active'
      setText(line, (done ? '✓ ' : '… ') + (STEP_LABELS[event] || event))
      log.appendChild(line)
    })
    log.scrollTop = log.scrollHeight
  }

  function focusable(dialog) {
    return Array.from(dialog.querySelectorAll('button, input, [href], select, textarea')).filter(function (node) {
      return !node.disabled && !node.hidden && node.offsetParent !== null
    })
  }

  function setMessage(value, tone) {
    if (!state.dialog) return
    var message = state.dialog.querySelector('[data-role="message"]')
    setText(message, value)
    if (tone) message.dataset.tone = tone
    else message.removeAttribute('data-tone')
  }

  function updateSubmitState() {
    if (!state.dialog) return
    var mode = state.dialog.querySelector('[name="mode"]:checked').value
    var confirmed = state.dialog.querySelector('[name="confirm"]').checked
    var input = state.dialog.querySelector('input[type="datetime-local"]')
    var scheduleValid = mode === 'now' || Boolean(input.value && input.value >= input.min && input.value <= input.max)
    var submit = state.dialog.querySelector('[data-action="submit"]')
    var busy = state.pending || state.upgrading
    submit.disabled = busy || !confirmed || !scheduleValid || !state.candidateReady
    input.disabled = mode === 'now' || busy
    state.dialog.querySelectorAll('[name="mode"], [name="confirm"]').forEach(function (control) {
      control.disabled = state.upgrading
    })
    state.dialog.querySelector('[data-role="submit-label"]').textContent = state.upgrading
      ? '升级中…'
      : (mode === 'now' ? '现在升级' : '本次定时升级')
  }

  function renderExistingSchedule(operation) {
    if (!state.dialog) return
    state.existing = operation && operation.stage === 'scheduled' ? operation : null
    var area = state.dialog.querySelector('[data-role="existing"]')
    area.textContent = ''
    if (!state.existing) {
      area.hidden = true
      return
    }
    area.hidden = false
    var message = document.createElement('p')
    setText(message, '已有定时升级：' + formatShanghaiDisplay(state.existing.scheduled_at) + '，目标 ' + (state.existing.target_version || '未知'))
    area.appendChild(message)
    var actions = document.createElement('div')
    actions.className = 'xq-update-ui-inline-actions'
    actions.appendChild(button('替换计划', 'replace'))
    actions.appendChild(button('取消旧计划', 'cancel-schedule'))
    area.appendChild(actions)
  }

  function resumeRunningOperation(operation) {
    if (!operation || operation.stage !== 'running' || !state.dialog) return false
    state.upgrading = true
    updateSubmitState()
    renderProgressLog(operation.events)
    setMessage('升级正在执行，应用容器重启期间请求可能短暂断开。')
    if (state.pollTimer === null) startPolling()
    return true
  }

  function isTransientUpgradeError(error) {
    return state.upgrading && (
      error.status === 0 ||
      error.status === 401 ||
      error.status === 502 ||
      error.status === 503 ||
      error.status === 504 ||
      error.code === 'UPDATE_AUTH_REQUIRED'
    )
  }

  function renderDialog(info, existing) {
    var dialog = document.createElement('div')
    dialog.id = 'xingqiao-update-ui-dialog'
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('aria-modal', 'true')
    dialog.setAttribute('aria-labelledby', 'xingqiao-update-ui-title')
    var backdrop = document.createElement('div')
    backdrop.className = 'xq-update-ui-backdrop'
    backdrop.dataset.action = 'close'
    dialog.appendChild(backdrop)
    var panel = document.createElement('section')
    panel.className = 'xq-update-ui-panel'
    panel.tabIndex = -1
    var header = document.createElement('header')
    header.className = 'xq-update-ui-header'
    var kicker = document.createElement('p')
    kicker.className = 'xq-update-ui-kicker'
    setText(kicker, '星桥受控升级')
    header.appendChild(kicker)
    var title = document.createElement('h2')
    title.id = 'xingqiao-update-ui-title'
    setText(title, '确认官方版本升级')
    header.appendChild(title)
    panel.appendChild(header)
    var body = document.createElement('div')
    body.className = 'xq-update-ui-body'
    var summary = document.createElement('dl')
    summary.className = 'xq-update-ui-summary'
    ;[['当前版本', info.current], ['目标版本', info.target]].forEach(function (item, index) {
      var wrapper = document.createElement('div')
      var label = document.createElement('dt')
      var value = document.createElement('dd')
      setText(label, item[0])
      setText(value, item[1])
      if (index === 1) value.dataset.role = 'target-version'
      wrapper.appendChild(label)
      wrapper.appendChild(value)
      summary.appendChild(wrapper)
    })
    body.appendChild(summary)
    var choices = document.createElement('div')
    choices.className = 'xq-update-ui-choice'
    ;[['now', '现在升级'], ['schedule', '本次定时升级']].forEach(function (item, index) {
      var label = document.createElement('label')
      var radio = document.createElement('input')
      radio.type = 'radio'
      radio.name = 'mode'
      radio.value = item[0]
      radio.checked = index === 0
      label.appendChild(radio)
      var labelText = document.createElement('span')
      setText(labelText, item[1])
      label.appendChild(labelText)
      choices.appendChild(label)
    })
    body.appendChild(choices)
    var schedule = document.createElement('div')
    schedule.className = 'xq-update-ui-schedule'
    var scheduleLabel = document.createElement('label')
    scheduleLabel.htmlFor = 'xingqiao-update-ui-scheduled-at'
    setText(scheduleLabel, '北京时间')
    schedule.appendChild(scheduleLabel)
    var input = document.createElement('input')
    input.id = 'xingqiao-update-ui-scheduled-at'
    input.type = 'datetime-local'
    input.min = formatShanghaiInput(new Date(Date.now() + 2 * 60 * 1000))
    input.max = formatShanghaiInput(new Date(Date.now() + 30 * 24 * 60 * 60 * 1000))
    schedule.appendChild(input)
    body.appendChild(schedule)
    var existing = document.createElement('div')
    existing.className = 'xq-update-ui-existing'
    existing.dataset.role = 'existing'
    existing.hidden = true
    body.appendChild(existing)
    var confirm = document.createElement('label')
    confirm.className = 'xq-update-ui-confirm'
    var checkbox = document.createElement('input')
    checkbox.type = 'checkbox'
    checkbox.name = 'confirm'
    confirm.appendChild(checkbox)
    var confirmText = document.createElement('span')
    setText(confirmText, '我确认由宿主机按此版本执行一次受控升级。')
    confirm.appendChild(confirmText)
    body.appendChild(confirm)
    var progressLog = document.createElement('div')
    progressLog.className = 'xq-update-ui-progress-log'
    progressLog.dataset.role = 'progress-log'
    progressLog.setAttribute('aria-live', 'polite')
    progressLog.hidden = true
    body.appendChild(progressLog)
    var message = document.createElement('p')
    message.className = 'xq-update-ui-message'
    message.dataset.role = 'message'
    message.setAttribute('aria-live', 'polite')
    body.appendChild(message)
    panel.appendChild(body)
    var footer = document.createElement('footer')
    footer.className = 'xq-update-ui-footer'
    footer.appendChild(button('取消', 'close'))
    var submit = button('', 'submit', 'primary')
    var submitLabel = document.createElement('span')
    submitLabel.dataset.role = 'submit-label'
    setText(submitLabel, '现在升级')
    submit.textContent = ''
    submit.appendChild(submitLabel)
    footer.appendChild(submit)
    panel.appendChild(footer)
    dialog.appendChild(panel)
    document.body.appendChild(dialog)
    state.dialog = dialog
    renderExistingSchedule(operationOrNull(existing))
    choices.addEventListener('change', function () {
      schedule.hidden = dialog.querySelector('[name="mode"]:checked').value === 'now'
      updateSubmitState()
    })
    input.addEventListener('input', updateSubmitState)
    checkbox.addEventListener('change', updateSubmitState)
    dialog.addEventListener('click', handleDialogClick)
    dialog.addEventListener('keydown', handleDialogKeydown)
    updateSubmitState()
    var first = focusable(dialog)[0]
    if (first) first.focus()
    return dialog
  }

  function operationOrNull(operation) {
    return operation && operation.stage === 'scheduled' ? operation : null
  }

  function settled(promise) {
    return promise.then(function (value) {
      return { value: value }
    }, function (error) {
      return { error: error }
    })
  }

  async function openConfirmation() {
    if (state.dialog) return state.dialog
    if (state.opening) return state.opening
    state.candidateReady = false
    state.readinessTarget = ''
    state.readinessGeneration += 1
    state.opening = Promise.all([settled(getUpdateInfo()), settled(getStatus())]).then(async function (results) {
      var infoResult = results[0]
      var statusResult = results[1]
      state.info = infoResult.error ? { current: '未知', target: '未知' } : infoResult.value
      var operation = statusResult.error ? null : statusResult.value
      renderDialog(state.info, operation)
      renderExistingSchedule(operation)
      if (resumeRunningOperation(operation)) return state.dialog
      var error = statusResult.error || infoResult.error
      if (error) {
        setMessage(error.message, 'error')
      } else {
        state.readinessTarget = state.info.target
        await pollReadiness()
        startReadinessPolling()
      }
      return state.dialog
    }).finally(function () {
      state.opening = null
    })
    return state.opening
  }

  async function cancelSchedule() {
    if (state.pending) return
    state.pending = true
    updateSubmitState()
    try {
      await apiRequest(SCHEDULE_PATH, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
      })
      setMessage('定时升级已取消。', 'success')
      renderExistingSchedule(null)
    } catch (error) {
      setMessage(error.message, 'error')
    } finally {
      state.pending = false
      updateSubmitState()
    }
  }

  async function submitUpdate() {
    if (!state.dialog || state.pending || state.upgrading || !state.candidateReady) return
    var mode = state.dialog.querySelector('[name="mode"]:checked').value
    var input = state.dialog.querySelector('input[type="datetime-local"]')
    var payload = { mode: mode, target_version: state.info.target }
    if (mode === 'schedule') {
      try {
        payload.scheduled_at = shanghaiLocalToUTC(input.value)
      } catch (error) {
        setMessage(error.message, 'error')
        return
      }
    }
    state.pending = true
    updateSubmitState()
    setMessage('正在提交受控升级请求。')
    try {
      if (mode === 'schedule' && state.replacing) {
        await apiRequest(SCHEDULE_PATH, {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
        })
        state.replacing = false
        renderExistingSchedule(null)
      }
      await apiRequest(UPDATE_PATH, { method: 'POST', body: JSON.stringify(payload) })
      stopReadinessPolling()
      if (mode === 'now') state.upgrading = true
      setMessage(mode === 'now' ? '升级已开始，以下是宿主机执行进度。' : '定时升级已受理。', 'success')
      startPolling()
    } catch (error) {
      if (error.code === 'UPDATE_ALREADY_SCHEDULED') {
        setMessage('已有定时升级，请先明确替换或取消。', 'error')
        renderExistingSchedule(await getStatus().catch(function () { return null }))
      } else if (error.code === 'UPDATE_CANDIDATE_NOT_READY') {
        state.candidateReady = false
        setMessage('候选版本正在准备，暂不可升级', 'warning')
        startReadinessPolling()
      } else if (error.code === 'UPDATE_TARGET_CHANGED') {
        await reloadTargetAfterChange()
      } else {
        setMessage(error.message, 'error')
      }
    } finally {
      state.pending = false
      updateSubmitState()
    }
  }

  async function pollStatus() {
    try {
      var operation = await getStatus()
      if (!operation) {
        stopPolling()
        return null
      }
      resumeRunningOperation(operation)
      if (operation.stage === 'promoted' && state.upgrading) {
        showToast('升级成功，服务已切换到 ' + (operation.target_version || '新版本') + '。')
        closeDialog()
        return operation
      }
      if (state.dialog) {
        if (operation.stage === 'scheduled') renderExistingSchedule(operation)
        renderProgressLog(operation.events)
        var phrase = {
          running: '升级正在执行，当前请求可能短暂断开。',
          promoted: '升级成功完成。',
          rolled_back: '升级失败，已回滚到上一版本。',
          failed: '升级失败，请查看运维状态。',
          rollback_failed: '升级和回滚均未完成，请立即查看运维状态。',
        }[operation.stage]
        if (phrase) setMessage(phrase, TERMINAL_STAGES.indexOf(operation.stage) === -1 ? null : (operation.stage === 'promoted' ? 'success' : 'error'))
      }
      if (TERMINAL_STAGES.indexOf(operation.stage) !== -1) {
        stopPolling()
        if (state.upgrading && operation.stage !== 'promoted') {
          state.upgrading = false
          updateSubmitState()
        }
      }
      return operation
    } catch (error) {
      if (state.dialog && isTransientUpgradeError(error)) {
        setMessage('应用容器正在重启，正在等待升级结果。')
        if (state.pollTimer === null) startPolling()
      } else if (state.dialog) {
        setMessage(error.message, 'error')
      }
      return null
    }
  }

  function startPolling() {
    stopPolling()
    state.pollTimer = window.setInterval(pollStatus, POLL_INTERVAL_MS)
  }

  function stopPolling() {
    if (state.pollTimer !== null) window.clearInterval(state.pollTimer)
    state.pollTimer = null
    stopReadinessPolling()
  }

  async function pollReadiness() {
    if (!state.dialog || !state.readinessTarget || state.upgrading) return null
    var dialog = state.dialog
    var target = state.readinessTarget
    var generation = state.readinessGeneration
    try {
      var readiness = await getReadiness(target)
      if (!isCurrentReadinessRequest(dialog, target, generation)) return null
      if (text(readiness.target_version) !== target) {
        throw new Error('更新服务返回了不匹配的候选版本')
      }
      state.candidateReady = readiness.ready === true
      if (!state.candidateReady) {
        setMessage('候选版本正在准备，暂不可升级', 'warning')
      } else {
        var message = state.dialog.querySelector('[data-role="message"]')
        if (message && message.textContent === '候选版本正在准备，暂不可升级') setMessage('')
      }
    } catch (error) {
      if (!isCurrentReadinessRequest(dialog, target, generation)) return null
      if (error.code === 'UPDATE_TARGET_CHANGED') {
        await reloadTargetAfterChange()
        return null
      }
      state.candidateReady = false
      setMessage(error.message || '更新服务不可用', 'error')
    }
    updateSubmitState()
    return state.candidateReady
  }

  function isCurrentReadinessRequest(dialog, target, generation) {
    return state.dialog === dialog && state.readinessTarget === target && state.readinessGeneration === generation
  }

  async function reloadTargetAfterChange() {
    var dialog = state.dialog
    var previousTarget = state.readinessTarget
    state.candidateReady = false
    state.readinessTarget = ''
    state.readinessGeneration += 1
    stopReadinessPolling()
    updateSubmitState()
    try {
      var info = await getUpdateInfo()
      if (state.dialog !== dialog || state.upgrading) return
      state.info = info
      var targetVersion = state.dialog.querySelector('[data-role="target-version"]')
      if (targetVersion) setText(targetVersion, info.target)
      if (!info.target || info.target === previousTarget) {
        setMessage('更新目标已变更，等待最新版本信息。', 'error')
        updateSubmitState()
        return
      }
      state.readinessTarget = info.target
      await pollReadiness()
      startReadinessPolling()
    } catch (error) {
      if (state.dialog !== dialog) return
      state.candidateReady = false
      setMessage(error.message || '更新服务不可用', 'error')
      updateSubmitState()
    }
  }

  function startReadinessPolling() {
    stopReadinessPolling()
    if (!state.dialog || !state.readinessTarget || state.upgrading) return
    state.readinessTimer = window.setInterval(pollReadiness, READINESS_POLL_INTERVAL_MS)
  }

  function stopReadinessPolling() {
    if (state.readinessTimer !== null) window.clearInterval(state.readinessTimer)
    state.readinessTimer = null
  }

  function replaceSchedule() {
    if (!state.dialog) return
    state.replacing = true
    var replace = state.dialog.querySelector('[data-action="replace"]')
    if (replace) replace.hidden = true
    var mode = state.dialog.querySelector('[name="mode"][value="schedule"]')
    mode.checked = true
    state.dialog.querySelector('.xq-update-ui-schedule').hidden = false
    setMessage('请设置新的北京时间并确认替换旧计划。')
    updateSubmitState()
  }

  function handleDialogClick(event) {
    var action = event.target.closest && event.target.closest('[data-action]')
    if (!action) return
    if (action.dataset.action === 'close') closeDialog()
    if (action.dataset.action === 'submit') submitUpdate()
    if (action.dataset.action === 'replace') replaceSchedule()
    if (action.dataset.action === 'cancel-schedule') cancelSchedule()
  }

  function handleDialogKeydown(event) {
    if (event.key === 'Escape') {
      event.preventDefault()
      closeDialog()
      return
    }
    if (event.key !== 'Tab' || !state.dialog) return
    var nodes = focusable(state.dialog)
    if (!nodes.length) return
    var current = nodes.indexOf(document.activeElement)
    var next = nodes[(current + (event.shiftKey ? -1 : 1) + nodes.length) % nodes.length]
    event.preventDefault()
    next.focus()
  }

  function handleDocumentClick(event) {
    if (!isUpdateButton(event.target)) return
    event.preventDefault()
    event.stopImmediatePropagation()
    openConfirmation()
  }

  function blockedMutationResponse() {
    var body = JSON.stringify({ code: 'UPDATE_CONFIRMATION_REQUIRED', message: '请通过确认窗口提交受控升级' })
    return Promise.resolve({
      ok: false,
      status: 409,
      json: function () { return Promise.resolve(JSON.parse(body)) },
      text: function () { return Promise.resolve(body) },
    })
  }

  window.fetch = function (input, options) {
    var requestURL = typeof input === 'string' ? input : input && input.url
    var pathname = requestURL ? new URL(requestURL, window.location.href).pathname : ''
    var method = String(options && options.method || input && input.method || 'GET').toUpperCase()
    if (method === 'POST' && pathname === UPDATE_PATH) {
      openConfirmation()
      return blockedMutationResponse()
    }
    return originalFetch(input, options)
  }

  document.addEventListener('click', handleDocumentClick, true)

  window.__XingqiaoUpdateUI__ = {
    openConfirmation: openConfirmation,
    pollStatus: pollStatus,
    pollReadiness: pollReadiness,
    startPolling: startPolling,
    stopPolling: stopPolling,
    isPolling: function () { return state.pollTimer !== null },
    isReadinessPolling: function () { return state.readinessTimer !== null },
    convertBeijingTime: shanghaiLocalToUTC,
  }
}())
