import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import ContactSupportDialog from '../ContactSupportDialog.vue'

const { showError, showSuccess } = vi.hoisted(() => ({
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('@/i18n', () => ({
  i18n: {
    global: {
      t: (key: string) => key,
    },
  },
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

describe('ContactSupportDialog', () => {
  let wrapper: VueWrapper | undefined
  const writeText = vi.fn()

  beforeEach(() => {
    vi.useFakeTimers()
    showError.mockReset()
    showSuccess.mockReset()
    writeText.mockReset()
    writeText.mockResolvedValue(undefined)
    Object.defineProperty(window, 'isSecureContext', {
      configurable: true,
      value: true,
    })
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  function mountDialog() {
    wrapper = mount(ContactSupportDialog, {
      attachTo: document.body,
      props: { show: true },
    })
    return wrapper
  }

  it('shows the QQ group number and local QR image', () => {
    mountDialog()

    expect(document.body.textContent).toContain('1080152144')
    expect(document.body.querySelector('img')?.getAttribute('src')).toBe(
      '/support/qq-group-1080152144.png',
    )
    expect(document.body.querySelector('img')?.getAttribute('alt')).toBe(
      'common.contactSupportDialog.qrAlt',
    )
  })

  it('copies the QQ group number and shows the copied state', async () => {
    mountDialog()

    const copyButton = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="copy-qq-group"]',
    )
    copyButton?.click()
    await flushPromises()

    expect(writeText).toHaveBeenCalledWith('1080152144')
    expect(document.body.textContent).toContain('common.copied')
    expect(showSuccess).toHaveBeenCalledWith('common.copiedToClipboard')
  })

  it('shows a clear failure state when copying fails', async () => {
    writeText.mockRejectedValue(new Error('clipboard unavailable'))
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      value: vi.fn().mockReturnValue(false),
    })
    mountDialog()

    const copyButton = document.body.querySelector<HTMLButtonElement>(
      '[data-testid="copy-qq-group"]',
    )
    copyButton?.click()
    await flushPromises()

    expect(document.body.textContent).toContain('common.copyFailed')
    expect(showError).toHaveBeenCalledWith('common.copyFailed')
  })

  it('emits close from the close button and Escape key', async () => {
    const mounted = mountDialog()

    document.body
      .querySelector<HTMLButtonElement>('[aria-label="Close modal"]')
      ?.click()
    await nextTick()
    expect(mounted.emitted('close')).toHaveLength(1)

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(mounted.emitted('close')).toHaveLength(2)
  })

  it('emits close when the overlay is clicked', async () => {
    const mounted = mountDialog()
    const overlay = document.body.querySelector<HTMLElement>('.modal-overlay')

    overlay?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()

    expect(mounted.emitted('close')).toHaveLength(1)
  })
})
