import { beforeEach, describe, expect, it, vi } from 'vitest'

const { mount, isReady } = vi.hoisted(() => ({
  mount: vi.fn(),
  isReady: vi.fn()
}))

vi.mock('vue', () => ({
  createApp: vi.fn(() => ({ use: vi.fn().mockReturnThis(), mount }))
}))
vi.mock('pinia', () => ({ createPinia: vi.fn(() => ({})) }))
vi.mock('../App.vue', () => ({ default: {} }))
vi.mock('../router', () => ({ default: { isReady } }))
vi.mock('../i18n', () => ({ default: {}, initI18n: vi.fn().mockResolvedValue(undefined) }))
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
    isReady.mockRejectedValue(new Error('navigation failed'))
    document.body.innerHTML = '<div id="app"></div>'
  })

  it('mounts the app even when initial router navigation rejects', async () => {
    const { bootstrap } = await import('../main')

    await bootstrap()

    expect(mount).toHaveBeenCalledWith('#app')
    expect(isReady).toHaveBeenCalled()
  })
})
