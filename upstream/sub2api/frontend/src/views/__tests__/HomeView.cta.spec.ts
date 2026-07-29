import { shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const state = vi.hoisted(() => ({
  auth: {
    isAuthenticated: false,
    isAdmin: false,
    user: null as { email: string } | null,
    checkAuth: vi.fn(),
  },
  app: {
    cachedPublicSettings: null,
    siteName: '星桥 API',
    siteLogo: '',
    siteSubtitle: 'AI API Gateway Platform',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => state.auth,
  useAppStore: () => state.app,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) =>
        ({
          'home.getStarted': '立即开始',
          'home.goToDashboard': '进入控制台',
        })[key] ?? key,
    }),
  }
})

import HomeView from '../HomeView.vue'

function mountHome() {
  return shallowMount(HomeView, {
    global: {
      stubs: {
        Icon: true,
        LocaleSwitcher: true,
        RouterLink: {
          props: ['to'],
          template: '<a :data-to="typeof to === \'string\' ? to : JSON.stringify(to)"><slot /></a>',
        },
      },
    },
  })
}

function heroCTA(wrapper: ReturnType<typeof mountHome>) {
  return wrapper.get('a.btn-primary')
}

describe('HomeView hero CTA', () => {
  beforeEach(() => {
    state.auth.isAuthenticated = false
    state.auth.isAdmin = false
    state.auth.user = null
  })

  it('shows 立即开始 and links guests to login', () => {
    const wrapper = mountHome()
    const cta = heroCTA(wrapper)

    expect(cta?.text()).toContain('立即开始')
    expect(cta?.attributes('data-to')).toBe('/login')
  })

  it.each([
    { isAdmin: false, expectedPath: '/dashboard' },
    { isAdmin: true, expectedPath: '/admin/dashboard' },
  ])('keeps 立即开始 while routing authenticated users correctly', ({ isAdmin, expectedPath }) => {
    state.auth.isAuthenticated = true
    state.auth.isAdmin = isAdmin
    state.auth.user = { email: 'user@example.com' }

    const wrapper = mountHome()
    const cta = heroCTA(wrapper)

    expect(cta?.text()).toContain('立即开始')
    expect(cta?.text()).not.toContain('进入控制台')
    expect(cta?.attributes('data-to')).toBe(expectedPath)
  })
})
