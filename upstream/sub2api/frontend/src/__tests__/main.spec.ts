import { beforeEach, describe, expect, it, vi } from 'vitest'

const { mount, isReady, initI18n } = vi.hoisted(() => ({
  mount: vi.fn(),
  isReady: vi.fn(),
  initI18n: vi.fn()
}))

vi.mock('vue', () => ({
  createApp: vi.fn(() => ({ use: vi.fn().mockReturnThis(), mount }))
}))
vi.mock('pinia', () => ({ createPinia: vi.fn(() => ({})) }))
vi.mock('../App.vue', () => ({ default: {} }))
vi.mock('../router', () => ({ default: { isReady } }))
vi.mock('../i18n', () => ({ default: {}, initI18n }))
vi.mock('@/stores/app', () => ({
  useAppStore: vi.fn(() => ({
    initFromInjectedConfig: vi.fn(),
    siteName: 'Sub2API',
    siteLogo: null
  }))
}))
vi.mock('@/utils/branding', () => ({ updateFavicon: vi.fn() }))
vi.mock('@/utils/device', () => ({ isIOSDevice: vi.fn(() => false) }))

describe('frontend bootstrap', () => {
  beforeEach(() => {
    mount.mockClear()
    isReady.mockReset()
    initI18n.mockReset()
    initI18n.mockResolvedValue(undefined)
    isReady.mockRejectedValue(new Error('navigation failed'))
    document.body.innerHTML = '<div id="app"></div>'
  })

  it('mounts the app even when initial router navigation rejects', async () => {
    const { bootstrap } = await import('../main')

    await bootstrap()

    expect(mount).toHaveBeenCalledWith('#app')
    expect(isReady).toHaveBeenCalled()
  })

  it('mounts the app even when locale initialization rejects', async () => {
    initI18n.mockRejectedValue(new Error('locale chunk unavailable'))

    const { bootstrap } = await import('../main')

    await bootstrap()

    expect(mount).toHaveBeenCalledWith('#app')
  })
})
