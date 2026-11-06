import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { saveAs } from 'file-saver'
import CodexQuickConfigModal from '../CodexQuickConfigModal.vue'
import zhDashboard from '@/i18n/locales/zh/dashboard'
import enDashboard from '@/i18n/locales/en/dashboard'

const fetchCodexModelsManifest = vi.hoisted(() => vi.fn())

vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('@/i18n', () => ({ i18n: { global: { t: (key: string) => key } } }))
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

beforeEach(() => {
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(() => {})
})

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
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true })
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

  it('allows download while the optional model catalog is loading', async () => {
    let resolveRequest!: (value: { content: string; modelCount: number }) => void
    fetchCodexModelsManifest.mockReturnValue(new Promise((resolve) => { resolveRequest = resolve }))
    const wrapper = mountModal()
    await flushPromises()

    await flushPromises()
    expect(wrapper.get('[data-testid="quick-config-catalog-loading"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="quick-config-copy"]').attributes('disabled')).toBeUndefined()

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
    expect(wrapper.get('[data-testid="quick-config-copy"]').attributes('disabled')).toBeUndefined()
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


it('downloads a plain macOS command even after catalog failure and shows the actual error', async () => {
  fetchCodexModelsManifest.mockRejectedValueOnce(Object.assign(new Error('Insufficient user balance'), {
    status: 403, code: 'billing_error', requestId: 'req-billing'
  }))
  const wrapper = mountModal()
  await flushPromises()
  expect(wrapper.get('[data-testid="quick-config-catalog-error"]').text()).toContain('insufficientBalance')
  expect(wrapper.get('[data-testid="quick-config-download"]').attributes('disabled')).toBeUndefined()
  await wrapper.get('[data-testid="quick-config-download"]').trigger('click')
  const [blob, filename] = vi.mocked(saveAs).mock.calls.at(-1)!
  expect(filename).toBe('xingqiao-codex-config.command')
  expect((blob as Blob).type).toBe('text/plain;charset=utf-8')
})


it('copies sanitized server diagnostics and downloads a basic script for every platform after catalog failure', async () => {
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
  fetchCodexModelsManifest.mockRejectedValue(Object.assign(new Error('Invalid API key sk-test. Bearer private-token'), { status: 401, code: 'invalid_api_key', requestId: 'req-401' }))
  vi.mocked(saveAs).mockClear()
  const wrapper = mountModal(); await flushPromises()
  expect(wrapper.get('[data-testid="quick-config-catalog-error"]').text()).toContain('Invalid API key')
  await wrapper.get('[data-testid="quick-config-copy-error"]').trigger('click')
  const diagnostic = writeText.mock.calls[0][0]
  expect(diagnostic).toContain('HTTP 401')
  expect(diagnostic).toContain('invalid_api_key')
  expect(diagnostic).toContain('req-401')
  expect(diagnostic).not.toContain('sk-test')
  expect(diagnostic).not.toContain('private-token')
  for (const [tab, filename] of [['unix', 'xingqiao-codex-config.command'], ['windows', 'sub2api-codex-config.cmd'], ['linux', 'sub2api-codex-config.sh']]) {
    await wrapper.get('[data-testid="quick-config-' + tab + '-tab"]').trigger('click')
    await wrapper.get('[data-testid="quick-config-download"]').trigger('click')
    const [blob, actualFilename] = vi.mocked(saveAs).mock.calls.at(-1)!
    expect(actualFilename).toBe(filename)
    expect((blob as Blob).type).toBe('text/plain;charset=utf-8')
    const content = await new Promise<string>((resolve) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.readAsText(blob as Blob) })
    expect(content).toMatch(/(?:CATALOG_PAYLOAD|catalogPayload) ?= ?''/)
  }
  wrapper.unmount()
})


it('copies the Mac first-run command on an insecure HTTP site without Clipboard API', async () => {
  Object.defineProperty(window, 'isSecureContext', { configurable: true, value: false })
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined })
  let copiedText = ''
  Object.defineProperty(document, 'execCommand', { configurable: true, value: () => {
    copiedText = document.querySelector('textarea')?.value || ''
    return true
  } })
  fetchCodexModelsManifest.mockResolvedValue({content: '{"models":[]}', modelCount: 0})
  const wrapper = mountModal(); await flushPromises()
  expect(wrapper.get('[data-testid="quick-config-mac-steps"]').findAll('li')).toHaveLength(3)
  const macInstructions = zhDashboard.keys.quickConfigureModal
  expect(macInstructions.macRun).toContain('没有执行权限')
  expect(macInstructions.macRun).toContain('终端')
  expect(macInstructions.macRun).not.toContain('后双击')
  expect(macInstructions.macFirstRun).toContain('隐私与安全性')
  await wrapper.get('[data-testid="quick-config-copy-startup"]').trigger('click')
  expect(copiedText).toContain('/usr/bin/osascript')
  expect(copiedText).toContain('chmod u+x "$xq_script"')
  expect(copiedText).toContain('/bin/bash "$xq_script"')
  expect(wrapper.get('[data-testid="quick-config-copy-startup"]').text()).toContain('copied')
  wrapper.unmount()
  delete (document as Document & { execCommand?: unknown }).execCommand
})


describe('independent macOS tutorial', () => {
  beforeEach(() => {
    Object.defineProperty(navigator, 'userAgent', { configurable: true, value: 'Macintosh' })
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: true })
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText: vi.fn().mockResolvedValue(undefined) } })
    fetchCodexModelsManifest.mockResolvedValue({ content: '{"models":[]}', modelCount: 0 })
  })

  it('offers the approved video and exact three steps, without the repeated Mac instructions/footer', async () => {
    const wrapper = mountModal(); await flushPromises()
    const video = wrapper.get('video')
    expect(video.attributes('controls')).toBeDefined()
    expect(video.attributes('autoplay')).toBeUndefined()
    expect(video.attributes('src')).toMatch(/macos-top-caption-voice\.mp4/)
    expect(wrapper.get('[data-testid="quick-config-mac-steps"]').findAll('li')).toHaveLength(3)
    expect(wrapper.find('[data-testid="quick-config-run-instruction"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('quickConfigureModal.downloadHint')
    expect(wrapper.text()).toContain('quickConfigureModal.runPrerequisite')
    const zh = zhDashboard.keys.quickConfigureModal
    expect([zh.macStep1, zh.macStep2, zh.macStep3]).toEqual([
      '下载脚本后，复制首次启动命令',
      '按 ⌘空格打开「终端」，粘贴命令并回车，再选择刚下载的脚本。',
      '输入 1 连接星桥，看到「配置已保存」后重新打开 Codex'
    ])
    expect(zh.macFirstRun).toBe('若 macOS 提示无法验证开发者，请先在「系统设置 → 隐私与安全性」确认来源，再选择「仍要打开」。')
    expect(enDashboard.keys.quickConfigureModal.macStep2).toContain('Terminal')
    wrapper.unmount()
  })

  it('preserves the actual video, expanded preview, scroll and focus when copying repeatedly', async () => {
    const wrapper = mount(CodexQuickConfigModal, { attachTo: document.body, props: { show: true, apiKey: 'sk-test', baseUrl: 'https://example.com' } })
    await flushPromises()
    const button = wrapper.get('[data-testid="quick-config-copy-startup"]').element as HTMLButtonElement
    const body = wrapper.element as HTMLElement, video = wrapper.get('video').element as HTMLVideoElement
    const details = wrapper.get('details').element as HTMLDetailsElement
    details.open = true; body.scrollTop = 250; video.currentTime = 8; button.focus()
    for (let i = 0; i < 2; i++) {
      await wrapper.get('[data-testid="quick-config-copy-startup"]').trigger('click'); await flushPromises()
      expect(wrapper.get('video').element).toBe(video)
      expect(wrapper.get('[data-testid="quick-config-copy-startup"]').element).toBe(button)
      expect(document.activeElement).toBe(button)
      expect(body.scrollTop).toBe(250)
      expect(video.currentTime).toBe(8)
      expect(details.open).toBe(true)
    }
    expect(navigator.clipboard.writeText).toHaveBeenLastCalledWith(expect.stringContaining('/usr/bin/osascript'))
    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  })

  it('restores the copy button focus after the real HTTP clipboard fallback', async () => {
    Object.defineProperty(window, 'isSecureContext', { configurable: true, value: false })
    const exec = vi.fn(() => true)
    Object.defineProperty(document, 'execCommand', { configurable: true, value: exec })
    const wrapper = mount(CodexQuickConfigModal, { attachTo: document.body, props: { show: true, apiKey: 'sk-test', baseUrl: 'https://example.com' } })
    await flushPromises()
    const button = wrapper.get('[data-testid="quick-config-copy-startup"]').element as HTMLButtonElement
    button.focus(); const video = wrapper.get('video').element
    await wrapper.get('[data-testid="quick-config-copy-startup"]').trigger('click'); await flushPromises()
    expect(exec).toHaveBeenCalledWith('copy')
    expect(document.activeElement).toBe(button)
    expect(wrapper.get('video').element).toBe(video)
    wrapper.unmount()
    delete (document as Document & { execCommand?: unknown }).execCommand
  })

  it('pauses the old video on system switch, close and hidden props; other OS text remains', async () => {
    const wrapper = mountModal(); await flushPromises()
    const video = wrapper.get('video').element as HTMLVideoElement
    const pause = vi.spyOn(video, 'pause')
    await wrapper.get('[data-testid="quick-config-windows-tab"]').trigger('click')
    expect(pause).toHaveBeenCalled()
    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.get('[data-testid="quick-config-run-instruction"]').text()).toContain('windowsRun')
    await wrapper.get('[data-testid="quick-config-linux-tab"]').trigger('click')
    expect(wrapper.get('[data-testid="quick-config-run-instruction"]').text()).toContain('linuxRun')
    await wrapper.get('[data-testid="quick-config-unix-tab"]').trigger('click')
    const fresh = wrapper.get('video').element as HTMLVideoElement
    const freshPause = vi.spyOn(fresh, 'pause')
    await wrapper.get('[data-testid="quick-config-close"]').trigger('click')
    expect(freshPause).toHaveBeenCalled()
    expect(wrapper.emitted('close')).toHaveLength(1)
    freshPause.mockClear(); await wrapper.setProps({ show: false })
    expect(freshPause).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('keeps steps and real copying/download available when both media and optional catalog fail', async () => {
    fetchCodexModelsManifest.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mountModal(); await flushPromises()
    const steps = wrapper.get('[data-testid="quick-config-mac-steps"]').element
    await wrapper.get('video').trigger('error')
    expect(wrapper.get('[data-testid="quick-config-video-error"]').text()).toContain('videoUnavailable')
    expect(wrapper.get('[data-testid="quick-config-mac-steps"]').element).toBe(steps)
    await wrapper.get('[data-testid="quick-config-copy-startup"]').trigger('click'); await flushPromises()
    expect(navigator.clipboard.writeText).toHaveBeenCalled()
    await wrapper.get('[data-testid="quick-config-download"]').trigger('click')
    expect(vi.mocked(saveAs).mock.calls.at(-1)?.[1]).toBe('xingqiao-codex-config.command')
    wrapper.unmount()
  })
})
