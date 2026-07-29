// Contract tests for the injected admin update dialog
// (infra/sub2api-update-ui/update-ui.js). The script is a browser IIFE, so we
// evaluate it inside jsdom with a stubbed fetch.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const scriptSource = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), '../../../infra/sub2api-update-ui/update-ui.js'),
  'utf8',
)

type FetchStub = (input: unknown, options?: unknown) => Promise<unknown>

declare global {
  interface Window {
    __XingqiaoUpdateUI__?: {
      openConfirmation: () => Promise<HTMLElement>
      pollStatus: () => Promise<unknown>
      pollReadiness: () => Promise<unknown>
      stopPolling: () => void
      isPolling: () => boolean
      isReadinessPolling: () => boolean
    }
  }
}

function jsonResponse(data: unknown) {
  return Promise.resolve({
    ok: true,
    status: 200,
    json: () => Promise.resolve({ code: 0, data }),
    text: () => Promise.resolve(JSON.stringify({ code: 0, data })),
  })
}

let statusData: Record<string, unknown> | null

function stubFetch(): FetchStub {
  return (input: unknown) => {
    const url = typeof input === 'string' ? input : String((input as { url?: string })?.url ?? '')
    if (url.includes('/check-updates')) {
      return jsonResponse({ current_version: '0.1.165', latest_version: '0.1.166' })
    }
    if (url.includes('/host-update/status')) {
      return jsonResponse(statusData)
    }
    if (url.includes('/host-update/readiness')) {
      return jsonResponse({ target_version: '0.1.166', ready: true })
    }
    if (url.includes('/system/update')) {
      return jsonResponse({ operation_id: 'op-ui-1', stage: 'scheduled' })
    }
    return jsonResponse(null)
  }
}

async function openDialogAndSubmit() {
  const ui = window.__XingqiaoUpdateUI__!
  const dialog = await ui.openConfirmation()
  const checkbox = dialog.querySelector<HTMLInputElement>('[name="confirm"]')!
  checkbox.checked = true
  checkbox.dispatchEvent(new Event('change', { bubbles: true }))
  const submit = dialog.querySelector<HTMLButtonElement>('[data-action="submit"]')!
  submit.click()
  await vi.waitFor(() => {
    if (!window.__XingqiaoUpdateUI__!.isPolling()) throw new Error('not yet polling')
  })
  return dialog
}

beforeEach(() => {
  statusData = null
  vi.useFakeTimers({ shouldAdvanceTime: true })
  const storage = new Map<string, string>()
  Object.defineProperty(window, 'localStorage', {
    configurable: true,
    value: {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, String(value)),
      removeItem: (key: string) => storage.delete(key),
    },
  })
  window.localStorage.setItem('auth_token', 'test-token')
  window.fetch = stubFetch() as typeof window.fetch
  // eslint-disable-next-line no-eval
  window.eval(scriptSource)
})

afterEach(() => {
  window.__XingqiaoUpdateUI__?.stopPolling()
  document.getElementById('xingqiao-update-ui-dialog')?.remove()
  document.querySelectorAll('.xq-update-ui-toast').forEach((node) => node.remove())
  vi.useRealTimers()
})

describe('update dialog progress feedback', () => {
  it('switches the submit button to an in-progress label after acceptance', async () => {
    const dialog = await openDialogAndSubmit()
    const submit = dialog.querySelector<HTMLButtonElement>('[data-action="submit"]')!
    expect(submit.textContent).toContain('升级中')
    expect(submit.disabled).toBe(true)
  })

  it('renders the executor step trace as a scrolling log while running', async () => {
    const dialog = await openDialogAndSubmit()
    statusData = {
      operation_id: 'op-ui-1',
      stage: 'running',
      events: ['inspect', 'backup-db', 'health'],
    }
    await window.__XingqiaoUpdateUI__!.pollStatus()
    const log = dialog.querySelector('[data-role="progress-log"]')!
    expect(log).toBeTruthy()
    const text = log.textContent ?? ''
    expect(text).toContain('检查运行环境')
    expect(text).toContain('备份数据库')
    expect(text).toContain('等待健康检查')
  })

  it('shows a toast and auto-closes the dialog when the upgrade promotes', async () => {
    await openDialogAndSubmit()
    statusData = { operation_id: 'op-ui-1', stage: 'promoted', events: ['promoted'] }
    await window.__XingqiaoUpdateUI__!.pollStatus()
    const toast = document.querySelector('.xq-update-ui-toast')
    expect(toast?.textContent).toContain('升级成功')
    expect(document.getElementById('xingqiao-update-ui-dialog')).toBeNull()
    // The toast dismisses itself.
    await vi.advanceTimersByTimeAsync(6000)
    expect(document.querySelector('.xq-update-ui-toast')).toBeNull()
  })

  it('keeps the dialog open on failure so the error stays visible', async () => {
    const dialog = await openDialogAndSubmit()
    statusData = { operation_id: 'op-ui-1', stage: 'failed', events: ['inspect'] }
    await window.__XingqiaoUpdateUI__!.pollStatus()
    expect(document.getElementById('xingqiao-update-ui-dialog')).toBe(dialog)
    const submit = dialog.querySelector<HTMLButtonElement>('[data-action="submit"]')!
    expect(submit.textContent).not.toContain('升级中')
  })
})
