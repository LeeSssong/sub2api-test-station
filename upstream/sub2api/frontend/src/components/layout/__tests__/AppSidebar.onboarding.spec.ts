import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useAdminSettingsStore } from '@/stores/adminSettings'
import { useOnboardingStore } from '@/stores/onboarding'
import { getAdminSteps, getUserSteps } from '@/components/Guide/steps'
import zh from '@/i18n/locales/zh/misc'
import en from '@/i18n/locales/en/misc'
import AppSidebar from '../AppSidebar.vue'

vi.mock('vue-i18n', async (original) => ({ ...await original<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: false, refreshBatchImageAccess: async () => {} }),
}))

let wrapper: VueWrapper | undefined
afterEach(() => { wrapper?.unmount(); vi.useRealTimers(); vi.restoreAllMocks() })

async function setup(role: 'user' | 'admin', path: string, simple = false) {
  const pinia = createPinia()
  const app = useAppStore(pinia)
  const auth = useAuthStore(pinia)
  auth.$patch({ user: { id: 7, role, username: 'test', balance: 10 } })
  vi.spyOn(auth, 'isSimpleMode', 'get').mockReturnValue(simple)
  vi.spyOn(useAdminSettingsStore(pinia), 'fetch').mockResolvedValue()
  app.$patch({ publicSettingsLoaded: true, cachedPublicSettings: { custom_menu_items: [] } })
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/keys', redirect: to => ({ path: '/dashboard', query: to.query, hash: '#my-keys' }) },
    { path: '/:pathMatch(.*)*', component: { template: '<div />' } },
  ] })
  await router.push(path)
  await router.isReady()
  const guide = useOnboardingStore(pinia)
  const steps = role === 'admin' ? getAdminSteps(key => key, simple) : getUserSteps(key => key)
  let index = steps.findIndex(step => step.element === '[data-tour="sidebar-my-keys"]')
  const advance = vi.fn(async () => { index++ })
  guide.setControlMethods({ isCurrentStep: selector => steps[index]?.element === selector, nextStep: advance })
  wrapper = mount(AppSidebar, { global: { plugins: [pinia, router], stubs: { VersionBadge: true, LocaleSwitcher: true, ContactSupportDialog: true } } })
  return { sidebar: wrapper, router, app, guide, advance, active: () => steps[index]?.element }
}

describe('existing key onboarding entry compatibility', () => {
  it.each(['user', 'admin'] as const)('uses the visible AI tools entry on the user surface for %s', async role => {
    const { sidebar, router, advance, active } = await setup(role, '/usage')
    const target = sidebar.get('nav [data-tour="sidebar-my-keys"]')
    expect(target.isVisible()).toBe(true)
    expect(target.attributes('href')).toBe('/dashboard')
    expect(target.attributes('aria-label')).toBe('AI 工具')
    expect(sidebar.findAll('nav [data-tour="sidebar-my-keys"]')).toHaveLength(1)
    await target.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/dashboard')
    expect(advance).toHaveBeenCalledTimes(1)
    expect(advance).toHaveBeenCalledWith(500)
    expect(active()).toBe('[data-tour="keys-create-btn"]')
  })

  it('closes the mobile sidebar while advancing to the existing create-key step', async () => {
    const { sidebar, app, advance, active } = await setup('user', '/usage')
    vi.useFakeTimers()
    app.setMobileOpen(true)
    await sidebar.get('nav a[href="/dashboard"]').trigger('click')
    expect(advance).toHaveBeenCalledTimes(1)
    expect(advance).toHaveBeenCalledWith(500)
    await vi.advanceTimersByTimeAsync(150)
    expect(app.mobileOpen).toBe(false)
    expect(active()).toBe('[data-tour="keys-create-btn"]')
  })

  it.each([false, true])('preserves the admin /keys target and redirect (simple=%s)', async simple => {
    const { sidebar, router, advance, active } = await setup('admin', '/admin/dashboard', simple)
    const target = sidebar.get('nav [data-tour="sidebar-my-keys"]')
    expect(target.isVisible()).toBe(true)
    expect(target.attributes('href')).toBe('/keys')
    expect(sidebar.findAll('nav [data-tour="sidebar-my-keys"]')).toHaveLength(1)
    await target.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe('/dashboard#my-keys')
    expect(advance).toHaveBeenCalledTimes(1)
    expect(advance).toHaveBeenCalledWith(500)
    expect(active()).toBe('[data-tour="keys-create-btn"]')
  })

  it('does not advance the key tour when another menu entry is clicked', async () => {
    const { sidebar, advance } = await setup('user', '/dashboard')
    await sidebar.get('nav a[href="/usage"]').trigger('click')
    expect(advance).not.toHaveBeenCalled()
  })

  it('describes the merged user destination in both supported locales', () => {
    expect(zh.onboarding.user.keyManage.description).toContain('AI 工具')
    expect(zh.onboarding.user.keyManage.description).toContain('下方')
    expect(zh.onboarding.user.keyManage.description).toContain('我的密钥')
    expect(en.onboarding.user.keyManage.description).toContain('AI Tools')
    expect(en.onboarding.user.keyManage.description).toContain('My Keys')
    expect(en.onboarding.user.keyManage.description).toContain('below')
  })
})
