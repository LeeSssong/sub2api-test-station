import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { afterEach, test } from 'node:test'

const require = createRequire(import.meta.url)
const { JSDOM } = require('../../homepage/node_modules/jsdom')
const UI_SCRIPT = new URL('../../infra/sub2api-update-ui/update-ui.js', import.meta.url)
const UI_HTML = new URL('../../infra/sub2api-update-ui/index.html', import.meta.url)
const openBrowsers = []

const flush = () => new Promise((resolve) => setTimeout(resolve, 0))

function response(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

async function createBrowser({ fetchImpl } = {}) {
  const dom = new JSDOM(
    '<!doctype html><html><body><button id="official-update">Update Now</button><button id="other">Refresh</button></body></html>',
    { url: 'https://api.xingqiaolab.top/admin/system', runScripts: 'outside-only' },
  )
  const { window } = dom
  window.Date.now = () => new Date('2026-07-20T00:00:00Z').getTime()
  const requests = []
  const defaultFetch = async (input, init = {}) => {
    const url = typeof input === 'string' ? input : input.url
    const method = (init.method || input.method || 'GET').toUpperCase()
    requests.push({ url, method, body: init.body, headers: init.headers })
    if (url.endsWith('/api/v1/admin/system/check-updates')) {
      return response({ code: 0, data: { current_version: '1.2.2', latest_version: '1.2.3' } })
    }
    if (url.endsWith('/api/v1/admin/system/host-update/status')) {
      return response({ code: 0, data: null })
    }
    if (url.endsWith('/api/v1/admin/system/host-update/readiness?target_version=1.2.3')) {
      return response({ code: 0, data: { target_version: '1.2.3', ready: true } })
    }
    if (url.endsWith('/api/v1/admin/system/update')) {
      return response({ code: 0, data: { operation_id: 'op-now', stage: 'accepted' } })
    }
    if (url.endsWith('/api/v1/admin/system/host-update/schedule')) {
      return response({ code: 0, data: { cancelled: true } })
    }
    throw new Error(`unexpected request: ${method} ${url}`)
  }
  window.fetch = fetchImpl ? fetchImpl(requests, defaultFetch) : defaultFetch
  window.localStorage.setItem('auth_token', 'admin-token')
  const script = await readFile(UI_SCRIPT, 'utf8')
  window.eval(script)
  await flush()
  const browser = { dom, window, requests, ui: window.__XingqiaoUpdateUI__ }
  openBrowsers.push(browser)
  return browser
}

afterEach(() => {
  while (openBrowsers.length) {
    const browser = openBrowsers.pop()
    browser.ui.stopPolling()
    browser.dom.window.close()
  }
})

function scheduledFetch(requests, fallback) {
  return async (input, init = {}) => {
    const url = typeof input === 'string' ? input : input.url
    if (url.endsWith('/host-update/status')) {
      requests.push({ url, method: 'GET', body: init.body, headers: init.headers })
      return response({ code: 0, data: { operation_id: 'old-op', stage: 'scheduled', target_version: '1.2.3', scheduled_at: '2026-07-26T04:00:00Z' } })
    }
    if (url.endsWith('/host-update/schedule') && (init.method || 'GET') === 'DELETE') {
      requests.push({ url, method: 'DELETE', body: init.body, headers: init.headers })
      return response({ code: 0, data: { cancelled: true } })
    }
    return fallback(input, init)
  }
}

function runningFetch(requests, fallback) {
  return async (input, init = {}) => {
    const url = typeof input === 'string' ? input : input.url
    if (url.endsWith('/host-update/status')) {
      requests.push({ url, method: 'GET', body: init.body, headers: init.headers })
      return response({
        code: 0,
        data: {
          operation_id: 'running-op',
          stage: 'running',
          target_version: '1.2.3',
          events: ['inspect', 'recreate-sub2api'],
        },
      })
    }
    return fallback(input, init)
  }
}

function transientAuthFetch(requests, fallback) {
  let updateAccepted = false
  return async (input, init = {}) => {
    const url = typeof input === 'string' ? input : input.url
    const method = (init.method || 'GET').toUpperCase()
    if (url.endsWith('/api/v1/admin/system/update')) {
      updateAccepted = true
      requests.push({ url, method, body: init.body, headers: init.headers })
      return response({ code: 0, data: { operation_id: 'op-now', stage: 'accepted' } })
    }
    if (url.endsWith('/host-update/status') && updateAccepted) {
      requests.push({ url, method, body: init.body, headers: init.headers })
      return response({ code: 'UPDATE_AUTH_REQUIRED' }, 401)
    }
    return fallback(input, init)
  }
}

function runningWithInfoFailureFetch(requests, fallback) {
  return async (input, init = {}) => {
    const url = typeof input === 'string' ? input : input.url
    if (url.endsWith('/check-updates')) {
      requests.push({ url, method: 'GET', body: init.body, headers: init.headers })
      return response({ code: 'TEMPORARILY_UNAVAILABLE' }, 503)
    }
    return runningFetch(requests, fallback)(input, init)
  }
}

function transientNetworkFetch(requests, fallback) {
  let updateAccepted = false
  return async (input, init = {}) => {
    const url = typeof input === 'string' ? input : input.url
    const method = (init.method || 'GET').toUpperCase()
    if (url.endsWith('/api/v1/admin/system/update')) {
      updateAccepted = true
      requests.push({ url, method, body: init.body, headers: init.headers })
      return response({ code: 0, data: { operation_id: 'op-now', stage: 'accepted' } })
    }
    if (url.endsWith('/host-update/status') && updateAccepted) {
      requests.push({ url, method, body: init.body, headers: init.headers })
      throw new TypeError('Failed to fetch')
    }
    return fallback(input, init)
  }
}

function preSubmitAuthFailureFetch(requests, fallback) {
  return async (input, init = {}) => {
    const url = typeof input === 'string' ? input : input.url
    if (url.endsWith('/host-update/status')) {
      requests.push({ url, method: 'GET', body: init.body, headers: init.headers })
      return response({ code: 'UPDATE_AUTH_REQUIRED' }, 401)
    }
    return fallback(input, init)
  }
}

test('captures the localized official update button before its Vue handler', async () => {
  const browser = await createBrowser()
  let officialHandlerCalled = false
  browser.window.document.addEventListener('click', () => {
    officialHandlerCalled = true
  })

  browser.window.document.querySelector('#official-update').click()
  await flush()

  assert.equal(officialHandlerCalled, false)
  assert.equal(browser.window.document.querySelector('[role="dialog"]') !== null, true)
  assert.match(browser.window.document.body.textContent, /1\.2\.2/)
  assert.match(browser.window.document.body.textContent, /1\.2\.3/)
  assert.equal(browser.window.document.querySelector('[name="mode"][value="now"]').checked, true)
})

test('requires explicit confirmation and converts Beijing time to UTC RFC3339', async () => {
  const browser = await createBrowser()
  await browser.ui.openConfirmation()
  const dialog = browser.window.document.querySelector('[role="dialog"]')
  const submit = dialog.querySelector('[data-action="submit"]')
  assert.equal(submit.disabled, true)

  dialog.querySelector('[name="mode"][value="schedule"]').click()
  const input = dialog.querySelector('input[type="datetime-local"]')
  input.value = '2026-07-26T12:34'
  dialog.querySelector('[name="confirm"]').click()
  await submit.click()
  await flush()

  const updateRequest = browser.requests.find((request) => request.url.endsWith('/api/v1/admin/system/update'))
  assert.ok(updateRequest)
  assert.deepEqual(JSON.parse(updateRequest.body), {
    mode: 'schedule',
    target_version: '1.2.3',
    scheduled_at: '2026-07-26T04:34:00.000Z',
  })
  browser.ui.stopPolling()
})

test('replaces an existing schedule with a JSON DELETE request', async () => {
  const browser = await createBrowser({ fetchImpl: scheduledFetch })
  await browser.ui.openConfirmation()
  const dialog = browser.window.document.querySelector('[role="dialog"]')
  assert.match(dialog.textContent, /已有定时升级/)
  assert.ok(dialog.querySelector('[data-action="replace"]'))
  assert.ok(dialog.querySelector('[data-action="cancel-schedule"]'))

  dialog.querySelector('[data-action="replace"]').click()
  assert.equal(dialog.querySelector('[data-action="replace"]').hidden, true)
  assert.equal(dialog.querySelector('[data-action="submit"]').disabled, true)
  dialog.querySelector('input[type="datetime-local"]').value = '2026-07-26T13:00'
  dialog.querySelector('[name="confirm"]').click()
  dialog.querySelector('[data-action="submit"]').click()
  await flush()
  browser.ui.stopPolling()

  const replacementDelete = browser.requests.find(
    (request) => request.url.endsWith('/host-update/schedule') && request.method === 'DELETE',
  )
  assert.ok(replacementDelete)
  assert.equal(replacementDelete.headers['Content-Type'], 'application/json')
})

test('cancels an existing schedule with a JSON DELETE request', async () => {
  const browser = await createBrowser({ fetchImpl: scheduledFetch })
  await browser.ui.openConfirmation()
  const dialog = browser.window.document.querySelector('[role="dialog"]')
  dialog.querySelector('[data-action="cancel-schedule"]').click()
  await flush()
  const cancelRequest = browser.requests.find(
    (request) => request.url.endsWith('/host-update/schedule') && request.method === 'DELETE',
  )
  assert.ok(cancelRequest)
  assert.equal(cancelRequest.headers['Content-Type'], 'application/json')
})

test('starts status polling after an accepted operation', async () => {
  const browser = await createBrowser()
  await browser.ui.openConfirmation()
  const dialog = browser.window.document.querySelector('[role="dialog"]')
  dialog.querySelector('[name="confirm"]').click()
  await dialog.querySelector('[data-action="submit"]').click()
  await flush()

  assert.equal(browser.ui.isPolling(), true)
  await browser.ui.pollStatus()
  assert.equal(browser.requests.some((request) => request.url.endsWith('/host-update/status')), true)
  browser.ui.stopPolling()
})

test('resumes a running upgrade after the admin page reloads', async () => {
  const browser = await createBrowser({ fetchImpl: runningFetch })
  await browser.ui.openConfirmation()
  const dialog = browser.window.document.querySelector('[role="dialog"]')

  assert.equal(dialog.querySelector('[name="mode"][value="now"]').disabled, true)
  assert.equal(dialog.querySelector('[data-role="submit-label"]').textContent, '升级中…')
  assert.match(dialog.querySelector('[data-role="message"]').textContent, /升级正在执行/)
  assert.match(dialog.querySelector('[data-role="progress-log"]').textContent, /检查运行环境/)
  dialog.querySelector('[data-action="submit"]').click()
  await flush()
  assert.equal(browser.requests.some((request) => request.url.endsWith('/api/v1/admin/system/update')), false)
  assert.equal(browser.ui.isPolling(), true)
  browser.ui.stopPolling()
})

test('resumes a running upgrade when version lookup is temporarily unavailable', async () => {
  const browser = await createBrowser({ fetchImpl: runningWithInfoFailureFetch })
  await browser.ui.openConfirmation()
  const dialog = browser.window.document.querySelector('[role="dialog"]')

  assert.equal(dialog.querySelector('[name="mode"][value="now"]').disabled, true)
  assert.equal(dialog.querySelector('[data-role="submit-label"]').textContent, '升级中…')
  assert.equal(browser.ui.isPolling(), true)
  browser.ui.stopPolling()
})

test('keeps waiting through transient authentication loss after update acceptance', async () => {
  const browser = await createBrowser({ fetchImpl: transientAuthFetch })
  await browser.ui.openConfirmation()
  const dialog = browser.window.document.querySelector('[role="dialog"]')
  dialog.querySelector('[name="confirm"]').click()
  dialog.querySelector('[data-action="submit"]').click()
  await flush()

  await browser.ui.pollStatus()

  const message = dialog.querySelector('[data-role="message"]').textContent
  assert.match(message, /应用容器正在重启/)
  assert.doesNotMatch(message, /更新服务不可用/)
  assert.equal(browser.ui.isPolling(), true)
  browser.ui.stopPolling()
})

test('keeps waiting through a network disconnect after update acceptance', async () => {
  const browser = await createBrowser({ fetchImpl: transientNetworkFetch })
  await browser.ui.openConfirmation()
  const dialog = browser.window.document.querySelector('[role="dialog"]')
  dialog.querySelector('[name="confirm"]').click()
  dialog.querySelector('[data-action="submit"]').click()
  await flush()

  await browser.ui.pollStatus()

  assert.match(dialog.querySelector('[data-role="message"]').textContent, /应用容器正在重启/)
  assert.equal(browser.ui.isPolling(), true)
  browser.ui.stopPolling()
})

test('shows authentication failure before an upgrade has started', async () => {
  const browser = await createBrowser({ fetchImpl: preSubmitAuthFailureFetch })
  await browser.ui.openConfirmation()
  const message = browser.window.document.querySelector('[data-role="message"]')

  assert.equal(message.dataset.tone, 'error')
  assert.doesNotMatch(message.textContent, /应用容器正在重启/)
  assert.equal(browser.ui.isPolling(), false)
})

test('does not intercept unrelated buttons and fail-closes direct update fetches', async () => {
  const browser = await createBrowser()
  let unrelatedCalled = false
  browser.window.document.addEventListener('click', (event) => {
    if (event.target.id === 'other') unrelatedCalled = true
  })
  browser.window.document.querySelector('#other').click()
  assert.equal(unrelatedCalled, true)

  const directResponse = await browser.window.fetch('/api/v1/admin/system/update', { method: 'POST', body: '{}' })
  await flush()
  assert.equal(directResponse.status, 409)
  assert.equal(browser.requests.some((request) => request.url.endsWith('/api/v1/admin/system/update')), false)
  assert.equal(browser.window.document.querySelector('[role="dialog"]') !== null, true)
})

test('keeps submit disabled until the qualified candidate becomes ready and stops polling on close', async () => {
  let ready = false
  const browser = await createBrowser({
    fetchImpl: (requests, fallback) => async (input, init = {}) => {
      const url = typeof input === 'string' ? input : input.url
      if (url.endsWith('/host-update/readiness?target_version=1.2.3')) {
        requests.push({ url, method: 'GET', body: init.body, headers: init.headers })
        return response({
          code: 0,
          data: {
            target_version: '1.2.3',
            ready,
            ...(ready ? {} : { reason: 'candidate_not_ready' }),
          },
        })
      }
      return fallback(input, init)
    },
  })
  await browser.ui.openConfirmation()
  const dialog = browser.window.document.querySelector('[role="dialog"]')
  const submit = dialog.querySelector('[data-action="submit"]')
  dialog.querySelector('[name="confirm"]').click()

  assert.match(dialog.querySelector('[data-role="message"]').textContent, /候选版本正在准备，暂不可升级/)
  assert.equal(dialog.querySelector('[data-role="message"]').dataset.tone, 'warning')
  assert.equal(submit.disabled, true)
  assert.equal(browser.ui.isReadinessPolling(), true)
  submit.click()
  await flush()
  assert.equal(browser.requests.some((request) => request.url.endsWith('/api/v1/admin/system/update')), false)

  ready = true
  await browser.ui.pollReadiness()
  assert.equal(submit.disabled, false)
  dialog.querySelector('[data-action="close"]').click()
  assert.equal(browser.ui.isReadinessPolling(), false)
})

test('reloads the official target after readiness reports UPDATE_TARGET_CHANGED', async () => {
  let target = '1.2.3'
  const browser = await createBrowser({
    fetchImpl: (requests, fallback) => async (input, init = {}) => {
      const url = typeof input === 'string' ? input : input.url
      const method = (init.method || input.method || 'GET').toUpperCase()
      if (url.endsWith('/check-updates')) {
        requests.push({ url, method, body: init.body, headers: init.headers })
        return response({ code: 0, data: { current_version: '1.2.2', latest_version: target } })
      }
      if (url.endsWith('/host-update/readiness?target_version=1.2.3')) {
        requests.push({ url, method, body: init.body, headers: init.headers })
        target = '1.2.4'
        return response({ code: 'UPDATE_TARGET_CHANGED' }, 409)
      }
      if (url.endsWith('/host-update/readiness?target_version=1.2.4')) {
        requests.push({ url, method, body: init.body, headers: init.headers })
        return response({ code: 0, data: { target_version: '1.2.4', ready: false, reason: 'candidate_not_ready' } })
      }
      return fallback(input, init)
    },
  })

  const dialog = await browser.ui.openConfirmation()
  dialog.querySelector('[name="confirm"]').click()

  const readinessTargets = browser.requests
    .filter((request) => request.url.includes('/host-update/readiness'))
    .map((request) => new URL(request.url, browser.window.location.href).searchParams.get('target_version'))
  assert.deepEqual(readinessTargets, ['1.2.3', '1.2.4'])
  assert.match(dialog.textContent, /1\.2\.4/)
  assert.equal(dialog.querySelector('[data-action="submit"]').disabled, true)
  assert.equal(browser.requests.some((request) => request.method === 'POST' && request.url.endsWith('/system/update')), false)
})

test('fails closed when refreshed update info still returns the changed target', async () => {
  let readinessCalls = 0
  const browser = await createBrowser({
    fetchImpl: (requests, fallback) => async (input, init = {}) => {
      const url = typeof input === 'string' ? input : input.url
      const method = (init.method || input.method || 'GET').toUpperCase()
      if (url.endsWith('/check-updates')) {
        requests.push({ url, method, body: init.body, headers: init.headers })
        return response({ code: 0, data: { current_version: '1.2.2', latest_version: '1.2.3' } })
      }
      if (url.endsWith('/host-update/readiness?target_version=1.2.3')) {
        requests.push({ url, method, body: init.body, headers: init.headers })
        readinessCalls += 1
        if (readinessCalls === 1) return response({ code: 'UPDATE_TARGET_CHANGED' }, 409)
        return response({ code: 0, data: { target_version: '1.2.3', ready: true } })
      }
      return fallback(input, init)
    },
  })

  const dialog = await browser.ui.openConfirmation()
  dialog.querySelector('[name="confirm"]').click()

  assert.equal(readinessCalls, 1)
  assert.equal(dialog.querySelector('[data-action="submit"]').disabled, true)
  assert.equal(dialog.querySelector('[data-role="message"]').dataset.tone, 'error')
  assert.match(dialog.querySelector('[data-role="message"]').textContent, /更新目标已变更/)
})

test('fails closed and refreshes readiness when POST reports UPDATE_TARGET_CHANGED', async () => {
  let target = '1.2.3'
  const browser = await createBrowser({
    fetchImpl: (requests, fallback) => async (input, init = {}) => {
      const url = typeof input === 'string' ? input : input.url
      const method = (init.method || input.method || 'GET').toUpperCase()
      if (url.endsWith('/check-updates')) {
        requests.push({ url, method, body: init.body, headers: init.headers })
        return response({ code: 0, data: { current_version: '1.2.2', latest_version: target } })
      }
      if (url.endsWith('/host-update/readiness?target_version=1.2.3')) {
        requests.push({ url, method, body: init.body, headers: init.headers })
        return response({ code: 0, data: { target_version: '1.2.3', ready: true } })
      }
      if (url.endsWith('/api/v1/admin/system/update')) {
        requests.push({ url, method, body: init.body, headers: init.headers })
        target = '1.2.4'
        return response({ code: 'UPDATE_TARGET_CHANGED' }, 409)
      }
      if (url.endsWith('/host-update/readiness?target_version=1.2.4')) {
        requests.push({ url, method, body: init.body, headers: init.headers })
        return response({ code: 0, data: { target_version: '1.2.4', ready: false, reason: 'candidate_not_ready' } })
      }
      return fallback(input, init)
    },
  })

  const dialog = await browser.ui.openConfirmation()
  dialog.querySelector('[name="confirm"]').click()
  dialog.querySelector('[data-action="submit"]').click()
  await flush()

  const updates = browser.requests.filter((request) => request.method === 'POST' && request.url.endsWith('/system/update'))
  assert.deepEqual(updates.map((request) => JSON.parse(request.body).target_version), ['1.2.3'])
  assert.equal(dialog.querySelector('[data-action="submit"]').disabled, true)
  assert.equal(browser.requests.some((request) => request.url.endsWith('/host-update/readiness?target_version=1.2.4')), true)
})

test('ignores readiness responses from a closed dialog when a new dialog opens', async () => {
  const readinessResolvers = []
  let readinessCalls = 0
  const browser = await createBrowser({
    fetchImpl: (requests, fallback) => async (input, init = {}) => {
      const url = typeof input === 'string' ? input : input.url
      if (url.endsWith('/host-update/readiness?target_version=1.2.3')) {
        requests.push({ url, method: 'GET', body: init.body, headers: init.headers })
        readinessCalls += 1
        if (readinessCalls === 1) {
          return response({
            code: 0,
            data: { target_version: '1.2.3', ready: false, reason: 'candidate_not_ready' },
          })
        }
        return new Promise((resolve) => readinessResolvers.push(resolve))
      }
      return fallback(input, init)
    },
  })

  await browser.ui.openConfirmation()
  const stalePoll = browser.ui.pollReadiness()
  await flush()
  const firstDialog = browser.window.document.querySelector('[role="dialog"]')
  assert.ok(firstDialog)
  assert.equal(readinessResolvers.length, 1)
  firstDialog.querySelector('[data-action="close"]').click()

  const secondOpen = browser.ui.openConfirmation()
  await flush()
  const secondDialog = browser.window.document.querySelector('[role="dialog"]')
  assert.ok(secondDialog)
  assert.equal(readinessResolvers.length, 2)
  readinessResolvers[0](response({ code: 0, data: { target_version: '1.2.3', ready: true } }))
  await stalePoll
  secondDialog.querySelector('[name="confirm"]').click()
  assert.equal(secondDialog.querySelector('[data-action="submit"]').disabled, true)

  readinessResolvers[1](response({
    code: 0,
    data: { target_version: '1.2.3', ready: false, reason: 'candidate_not_ready' },
  }))
  await secondOpen
})

test('renders candidate-not-ready POST errors as preparation state', async () => {
  const browser = await createBrowser({
    fetchImpl: (requests, fallback) => async (input, init = {}) => {
      const url = typeof input === 'string' ? input : input.url
      if (url.endsWith('/api/v1/admin/system/update')) {
        requests.push({ url, method: 'POST', body: init.body, headers: init.headers })
        return response({ code: 'UPDATE_CANDIDATE_NOT_READY' }, 409)
      }
      return fallback(input, init)
    },
  })
  await browser.ui.openConfirmation()
  const dialog = browser.window.document.querySelector('[role="dialog"]')
  dialog.querySelector('[name="confirm"]').click()
  dialog.querySelector('[data-action="submit"]').click()
  await flush()

  assert.match(dialog.querySelector('[data-role="message"]').textContent, /候选版本正在准备，暂不可升级/)
  assert.doesNotMatch(dialog.querySelector('[data-role="message"]').textContent, /更新服务不可用/)
  assert.equal(dialog.querySelector('[data-role="message"]').dataset.tone, 'warning')
})

test('serves a template shell with external UI assets only', async () => {
  const html = await readFile(UI_HTML, 'utf8')
  assert.match(html, /\{\{\s*httpInclude\s+"\/__sub2api-official-index"\s*\}\}/)
  assert.match(html, /href="\/xingqiao-update-ui\.css\?v=20260726-1"/)
  assert.match(html, /src="\/xingqiao-update-ui\.js\?v=20260729-1"/)
  assert.doesNotMatch(html, /<script[^>]*>[^<]+<\/script>/)
})
