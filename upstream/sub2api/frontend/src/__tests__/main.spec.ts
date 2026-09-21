import { beforeEach, describe, expect, it, vi } from 'vitest'

const { mount, isReady, initI18n, initFromInjectedConfig, updateFavicon, appStoreState } = vi.hoisted(() => ({
  mount: vi.fn(),
  isReady: vi.fn(),
  initI18n: vi.fn(),
  initFromInjectedConfig: vi.fn(),
  updateFavicon: vi.fn(),
  appStoreState: { siteName: 'Sub2API', siteLogo: null as string | null }
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
    initFromInjectedConfig,
    get siteName() { return appStoreState.siteName },
    get siteLogo() { return appStoreState.siteLogo }
  }))
}))
vi.mock('@/utils/branding', () => ({ updateFavicon }))
vi.mock('@/utils/device', () => ({ isIOSDevice: vi.fn(() => false) }))

describe('frontend bootstrap', () => {
  beforeEach(() => {
    mount.mockClear()
    isReady.mockReset()
    initI18n.mockReset()
    initI18n.mockResolvedValue(undefined)
    isReady.mockRejectedValue(new Error('navigation failed'))
    initFromInjectedConfig.mockReset()
    initFromInjectedConfig.mockReturnValue(false)
    updateFavicon.mockReset()
    appStoreState.siteName = 'Sub2API'
    appStoreState.siteLogo = null
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

  it('does not replace a server-injected favicon before public settings load', async () => {
    const { bootstrap } = await import('../main')

    await bootstrap()

    expect(updateFavicon).not.toHaveBeenCalled()
  })

  it('applies the administrator logo from injected public settings', async () => {
    initFromInjectedConfig.mockReturnValue(true)
    appStoreState.siteLogo = 'data:image/png;base64,ADMIN'
    const { bootstrap } = await import('../main')

    await bootstrap()

    expect(updateFavicon).toHaveBeenCalledWith('data:image/png;base64,ADMIN')
  })
})
