import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useAdminSettingsStore } from '@/stores/adminSettings'
import AppSidebar from '../AppSidebar.vue'

vi.mock('vue-i18n', async (original) => ({ ...await original<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: false, refreshBatchImageAccess: async () => {} }),
}))

const storefront = {
  id: 'xingqiao-storefront', label: '云猫兑换码充值',
  url: 'https://catfk.com/shop/DLK8SNUJ', visibility: 'user' as const, sort_order: 90,
  icon_svg: '<svg viewBox="0 0 24 24"><rect x="3" y="5" width="18" height="14" /></svg>',
}
let wrapper: VueWrapper | undefined

afterEach(() => { wrapper?.unmount(); vi.restoreAllMocks() })

async function setup(items = [storefront], paymentEnabled = false, role: 'user' | 'admin' = 'user') {
  const pinia = createPinia()
  const app = useAppStore(pinia)
  useAuthStore(pinia).$patch({ user: { id: 7, role, username: 'test', balance: 10 } })
  vi.spyOn(useAdminSettingsStore(pinia), 'fetch').mockResolvedValue()
  app.$patch({ publicSettingsLoaded: true, cachedPublicSettings: { custom_menu_items: items, payment_enabled: paymentEnabled } })
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/:pathMatch(.*)*', component: { template: '<div />' } },
  ] })
  await router.push('/dashboard')
  await router.isReady()
  wrapper = mount(AppSidebar, { global: {
    plugins: [pinia, router],
    stubs: { VersionBadge: true, LocaleSwitcher: true, ContactSupportDialog: true },
  } })
  return { app, router, sidebar: wrapper }
}

describe('regular user storefront menu', () => {
  it.each([false, undefined])('hides support tickets unless explicitly enabled (value=%s)', async (enabled) => {
    const { sidebar, app } = await setup([])
    app.$patch({ cachedPublicSettings: { support_ticket_enabled: enabled } })
    await nextTick()
    expect(sidebar.find('nav a[href="/support-tickets"]').exists()).toBe(false)
  })

  it('hides support tickets while settings are unavailable', async () => {
    const { sidebar, app } = await setup([])
    app.$patch({ publicSettingsLoaded: false, cachedPublicSettings: null })
    await nextTick()
    expect(sidebar.find('nav a[href="/support-tickets"]').exists()).toBe(false)
  })

  it('reactively shows an enabled ticket entry and opens the native page', async () => {
    const { sidebar, app, router } = await setup([])
    app.$patch({ cachedPublicSettings: { support_ticket_enabled: true } })
    await nextTick()
    expect(sidebar.findAll('nav a[href="/support-tickets"]')).toHaveLength(1)
    await sidebar.get('nav a[href="/support-tickets"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/support-tickets')

    app.$patch({ cachedPublicSettings: { support_ticket_enabled: false } })
    await nextTick()
    expect(sidebar.find('nav a[href="/support-tickets"]').exists()).toBe(false)
  })

  it('keeps administrator tickets in the management console and follows the switch', async () => {
    const { sidebar, app, router } = await setup([], false, 'admin')
    app.$patch({ cachedPublicSettings: { support_ticket_enabled: true } })
    await nextTick()
    expect(sidebar.find('nav a[href="/support-tickets"]').exists()).toBe(false)
    await router.push('/admin/dashboard')
    await flushPromises()
    expect(sidebar.find('nav a[href="/support-tickets"]').exists()).toBe(false)
    expect(sidebar.find('nav a[href="/admin/support-tickets"]').exists()).toBe(true)

    app.$patch({ cachedPublicSettings: { support_ticket_enabled: false } })
    await nextTick()
    expect(sidebar.find('nav a[href="/admin/support-tickets"]').exists()).toBe(false)
  })

  it.each(['user', 'admin'] as const)('keeps order history outside the sidebar for %s', async (role) => {
    const { sidebar } = await setup([], false, role)
    expect(sidebar.find('nav a[href="/orders"]').exists()).toBe(false)
    await sidebar.get('[data-testid="user-sidebar-account"]').trigger('click')
    expect(sidebar.find('[role="menu"] a[href="/orders"]').exists()).toBe(false)
  })

  it.each(['user', 'admin'] as const)('follows the affiliate switch and opens the native page for %s', async (role) => {
    const { sidebar, app, router } = await setup([], false, role)
    expect(sidebar.find('nav a[href="/affiliate"]').exists()).toBe(false)

    app.$patch({ cachedPublicSettings: { affiliate_enabled: true } })
    await nextTick()
    const affiliate = sidebar.get('nav a[href="/affiliate"]')
    expect(affiliate.text()).toBe('nav.affiliate')
    expect(affiliate.find('svg').exists()).toBe(true)
    await affiliate.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/affiliate')
    expect(sidebar.get('nav a[href="/affiliate"]').attributes('aria-current')).toBe('page')

    app.$patch({ cachedPublicSettings: { affiliate_enabled: false } })
    await nextTick()
    expect(sidebar.find('nav a[href="/affiliate"]').exists()).toBe(false)

    app.$patch({ cachedPublicSettings: { affiliate_enabled: true } })
    await nextTick()
    expect(sidebar.findAll('nav a[href="/affiliate"]')).toHaveLength(1)
  })

  it('gives administrators the same user navigation and a return to the management console', async () => {
    const { sidebar, app, router } = await setup([storefront], false, 'admin')
    app.$patch({ sidebarCollapsed: true })
    await nextTick()
    expect(sidebar.findAll('nav a').map(link => link.attributes('href'))).toEqual(['/dashboard', '/image-studio', '/usage'])
    expect(sidebar.get('aside').classes()).not.toContain('admin-sidebar')
    expect(sidebar.get('aside').classes()).not.toContain('admin-sidebar-collapsed')
    expect(sidebar.get('.sidebar-brand-title').attributes('href')).toBe('/dashboard')

    await sidebar.get('[data-testid="user-sidebar-account"]').trigger('click')
    expect(sidebar.find('.shell-collapse-action').exists()).toBe(false)
    await sidebar.get('[role="menu"] a[href="/admin/dashboard"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/admin/dashboard')
    expect(sidebar.get('aside').classes()).toContain('admin-sidebar')
    expect(sidebar.get('aside').classes()).toContain('admin-sidebar-collapsed')
    expect(sidebar.findAll('nav a').some(link => link.attributes('href') === '/admin/accounts')).toBe(true)
    app.$patch({ sidebarCollapsed: false })
    await nextTick()
    const personal = sidebar.findAll('.sidebar-section').find(section => section.find('.sidebar-section-title').exists())!
    expect(personal.findAll('a').map(link => link.attributes('href'))).toEqual(['/dashboard', '/image-studio', '/usage', '/keys'])
    await router.push('/keys')
    await flushPromises()
    expect(sidebar.get('aside').classes()).not.toContain('admin-sidebar')
    expect(sidebar.findAll('nav a').map(link => link.attributes('href'))).toEqual(['/dashboard', '/image-studio', '/usage'])
  })

  it.each([false, true])('keeps only the bottom recharge entry when payments are enabled=%s', async (paymentEnabled) => {
    const { sidebar, router } = await setup([storefront], paymentEnabled)
    expect(sidebar.findAll('nav a').map(link => link.attributes('href'))).toEqual([
      '/dashboard', '/image-studio', '/usage',
    ])
    expect(sidebar.find('nav a[href="/redeem"]').exists()).toBe(false)
    expect(sidebar.find('nav a[href="/custom/xingqiao-storefront"]').exists()).toBe(false)
    const link = sidebar.get('[data-testid="user-sidebar-recharge"]')
    expect(link.attributes('href')).toBe(paymentEnabled ? '/purchase' : '/redeem')
    await link.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe(paymentEnabled ? '/purchase' : '/redeem')
  })

  it('updates the configured site name without restoring duplicate menus when settings change', async () => {
    const { sidebar, app } = await setup([])
    app.$patch({ siteName: '星桥测试服' })
    await nextTick()
    expect(sidebar.get('.sidebar-brand-title').text()).toBe('星桥测试服')
    app.$patch({ siteName: '更新后的站点名称' })
    await nextTick()
    expect(sidebar.get('.sidebar-brand-title').text()).toBe('更新后的站点名称')
    for (const visibility of ['user', 'admin'] as const) {
      app.$patch({ cachedPublicSettings: { custom_menu_items: [{ ...storefront, visibility }] } })
      await nextTick()
      expect(sidebar.findAll('nav a').map(link => link.attributes('href'))).toEqual(['/dashboard', '/image-studio', '/usage'])
      expect(sidebar.get('[data-testid="user-sidebar-recharge"]').exists()).toBe(true)
    }
  })

  it('mounts intelligence tests only through configured user menus', async () => {
    const {sidebar,app}=await setup([])
    const item={...storefront,id:'intelligence-test',label:'智商检测',url:'/intelligence-test'}
    app.$patch({cachedPublicSettings:{custom_menu_items:[item],pelican_showcase_enabled:true}})
    await nextTick()
    expect(sidebar.get('a[href="/intelligence-test"]').text()).toBe('智商监测')
    expect(sidebar.get('a[href="/intelligence-test"]').find('svg rect').exists()).toBe(true)
    expect(sidebar.get('a[href="/intelligence-test"]').find('img[src*="undefined"]').exists()).toBe(false)
    expect(sidebar.find('a[href="/pelican-showcase"]').exists()).toBe(false)
    app.$patch({cachedPublicSettings:{custom_menu_items:[{...item,visibility:'admin'}]}})
    await nextTick();expect(sidebar.find('a[href="/intelligence-test"]').exists()).toBe(false)
  })


})
