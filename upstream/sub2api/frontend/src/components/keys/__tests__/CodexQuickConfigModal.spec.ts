import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { saveAs } from 'file-saver'
import CodexQuickConfigModal from '../CodexQuickConfigModal.vue'

const fetchCodexModelsManifest = vi.hoisted(() => vi.fn())

vi.mock('@/api/codex', () => ({ fetchCodexModelsManifest }))
vi.mock('file-saver', () => ({ saveAs: vi.fn() }))
vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: { template: '<div><slot /><slot name="footer" /></div>' }
}))
vi.mock('@/components/icons/Icon.vue', () => ({ default: { template: '<span />' } }))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => params
      ? `${key}:${JSON.stringify(params)}`
      : key
  })
}))

function mountModal() {
  return mount(CodexQuickConfigModal, {
    props: {
      show: true,
      apiKey: 'sk-test',
      baseUrl: 'https://example.com',
      platform: 'openai'
    }
  })
}

function decodeBase64Utf8(value: string): string {
  const bytes = atob(value).split('').map((character) => character.charCodeAt(0))
  return new TextDecoder().decode(new Uint8Array(bytes))
}

describe('CodexQuickConfigModal', () => {
  beforeEach(() => {
    fetchCodexModelsManifest.mockReset()
    fetchCodexModelsManifest.mockResolvedValue({ content: '{"models":[{"slug":"test-model"}]}', modelCount: 1 })
    vi.mocked(saveAs).mockClear()
    Object.defineProperty(window.navigator, 'userAgent', { configurable: true, value: 'Mozilla/5.0' })
  })

  it('automatically includes the catalog in a usable script and switches platforms', async () => {
    const wrapper = mountModal()
    await flushPromises()
    expect(wrapper.get('[data-testid="quick-config-copy"]').attributes('disabled')).toBeUndefined()
    const unixPayload = wrapper.get('pre').text().match(/PAYLOAD='([^']+)'/)?.[1]
    expect(wrapper.find('input[type="checkbox"]').exists()).toBe(false)
    expect(wrapper.get('pre').text()).toContain('CATALOG_PAYLOAD=')
    expect(fetchCodexModelsManifest).toHaveBeenCalledTimes(1)
    expect(unixPayload).toBeDefined()
    expect(decodeBase64Utf8(unixPayload!)).toContain('experimental_bearer_token')

    await wrapper.get('[data-testid="quick-config-windows-tab"]').trigger('click')
    expect(fetchCodexModelsManifest).toHaveBeenCalledTimes(1)
    expect(wrapper.get('pre').text()).toContain('@echo off')
    expect(wrapper.get('[data-testid="quick-config-windows-tab"]').attributes('aria-selected')).toBe('true')
  })

  it('shows a clear success message after copying or downloading', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) }
    })
    const wrapper = mountModal()
    await flushPromises()

    await wrapper.get('[data-testid="quick-config-copy"]').trigger('click')
    expect(wrapper.get('[data-testid="quick-config-action-status"]').text()).toContain('copySuccessMessage')

    await wrapper.get('[data-testid="quick-config-download"]').trigger('click')
    expect(wrapper.get('[data-testid="quick-config-action-status"]').text()).toContain('downloadSuccessMessage')
  })

  it('downloads an ASCII Windows CMD file with CRLF and system PowerShell bootstrap', async () => {
    const wrapper = mountModal()
    await flushPromises()
    await wrapper.get('[data-testid="quick-config-windows-tab"]').trigger('click')
    await wrapper.get('[data-testid="quick-config-download"]').trigger('click')
    const [blob, fileName] = vi.mocked(saveAs).mock.calls[0]
    expect(fileName).toBe('sub2api-codex-config.cmd')
    const content = await new Promise<string>((resolve) => {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result))
      reader.readAsText(blob as Blob)
    })
    expect(content.startsWith('@echo off\r\n')).toBe(true)
    expect([...content].every((character) => character.charCodeAt(0) <= 0x7f)).toBe(true)
    expect(content).toContain('%SystemRoot%\\System32\\WindowsPowerShell')
  })

  it('waits for the model catalog before enabling copy and download', async () => {
    let resolveRequest!: (value: { content: string; modelCount: number }) => void
    fetchCodexModelsManifest.mockReturnValue(new Promise((resolve) => { resolveRequest = resolve }))
    const wrapper = mountModal()
    await flushPromises()

    await flushPromises()
    expect(wrapper.get('[data-testid="quick-config-catalog-loading"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="quick-config-copy"]').attributes('disabled')).toBeDefined()

    resolveRequest({ content: '{"models":[]}', modelCount: 0 })
    await flushPromises()
    expect(wrapper.get('[data-testid="quick-config-catalog-ready"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="quick-config-download"]').attributes('disabled')).toBeUndefined()
  })

  it('shows a recoverable catalog error', async () => {
    fetchCodexModelsManifest.mockRejectedValue(new Error('unavailable'))
    const wrapper = mountModal()
    await flushPromises()
    await flushPromises()
    expect(wrapper.get('[data-testid="quick-config-catalog-error"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="quick-config-copy"]').attributes('disabled')).toBeDefined()
    fetchCodexModelsManifest.mockResolvedValue({ content: '{"models":[]}', modelCount: 0 })
    await wrapper.get('[data-testid="quick-config-catalog-retry"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="quick-config-download"]').attributes('disabled')).toBeUndefined()
  })

  it('ignores a stale catalog when the key changes and cancels on close', async () => {
    let resolveOld!: (value: { content: string; modelCount: number }) => void
    fetchCodexModelsManifest.mockReturnValueOnce(new Promise((resolve) => { resolveOld = resolve }))
    const wrapper = mountModal()
    await wrapper.setProps({ apiKey: 'sk-new' })
    await flushPromises()
    resolveOld({ content: '{"models":[{"slug":"old-model"}]}', modelCount: 1 })
    await flushPromises()
    expect(fetchCodexModelsManifest).toHaveBeenLastCalledWith('https://example.com', 'sk-new', expect.any(AbortSignal))
    expect(wrapper.get('pre').text()).not.toContain(btoa('{"models":[{"slug":"old-model"}]}'))
    const currentSignal = fetchCodexModelsManifest.mock.calls.at(-1)![2] as AbortSignal
    await wrapper.setProps({ show: false })
    expect(currentSignal.aborted).toBe(false)
    await wrapper.setProps({ show: true })
    fetchCodexModelsManifest.mockReturnValueOnce(new Promise(() => {}))
    await wrapper.setProps({ apiKey: 'sk-pending' })
    const pendingSignal = fetchCodexModelsManifest.mock.calls.at(-1)![2] as AbortSignal
    await wrapper.setProps({ show: false })
    expect(pendingSignal.aborted).toBe(true)
    expect(wrapper.get('[data-testid="quick-config-download"]').attributes('disabled')).toBeDefined()
  })
})
