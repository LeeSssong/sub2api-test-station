import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { test } from 'node:test'

const require = createRequire(import.meta.url)
const { JSDOM } = require('../../homepage/node_modules/jsdom')
const UI_SCRIPT = new URL('../../infra/sub2api-update-ui/update-ui.js', import.meta.url)
const UI_HTML = new URL('../../infra/sub2api-update-ui/index.html', import.meta.url)

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
  const requests = []
  const defaultFetch = async (input, init = {}) => {
    const url = typeof input === 'string' ? input : input.url
    const method = (init.method || input.method || 'GET').toUpperCase()
    requests.push({ url, method, body: init.body, headers: init.headers })
    if (url.endsWith('/api/v1/admin/system/check-updates')) {
      return response({ code: 0, data: { current_version: '1.2.2', latest_version: '1.2.3' } })
    }
    if (url.endsWith('/api/v1/admin/system/host-update/status')) {
      return response({ code: 0, data: { operation_id: 'op-status', stage: 'running', target_version: '1.2.3' } })
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
  return { dom, window, requests, ui: window.__XingqiaoUpdateUI__ }
}

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

test('serves a template shell with external UI assets only', async () => {
  const html = await readFile(UI_HTML, 'utf8')
  assert.match(html, /\{\{\s*httpInclude\s+"\/__sub2api-official-index"\s*\}\}/)
  assert.match(html, /href="\/xingqiao-update-ui\.css"/)
  assert.match(html, /src="\/xingqiao-update-ui\.js\?v=20260725-2"/)
  assert.doesNotMatch(html, /<script[^>]*>[^<]+<\/script>/)
})
