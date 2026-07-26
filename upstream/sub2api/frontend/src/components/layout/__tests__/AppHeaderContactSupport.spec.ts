import { mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import AppHeader from '../AppHeader.vue'

const state = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: { custom_menu_items: [] },
    contactInfo: '',
    docUrl: '',
    showError: vi.fn(),
    showSuccess: vi.fn(),
    toggleMobileSidebar: vi.fn(),
  },
  authStore: {
    isAdmin: true,
    isSimpleMode: false,
    logout: vi.fn(),
    user: {
      id: 1,
      username: 'Admin',
      email: 'admin@example.com',
      role: 'admin',
      balance: 0,
      frozen_balance: 0,
      avatar_url: '',
    },
  },
  onboardingStore: {
    replay: vi.fn(),
  },
  adminSettingsStore: {
    customMenuItems: [],
  },
  router: {
    push: vi.fn(),
  },
  route: {
    meta: {
      titleKey: 'admin.overview.title',
      descriptionKey: 'admin.overview.description',
    },
    name: 'AdminDashboard',
    params: {},
  },
}))

vi.mock('vue-router', () => ({
  useRoute: () => state.route,
  useRouter: () => state.router,
}))

vi.mock('@/stores', () => ({
  useAppStore: () => state.appStore,
  useAuthStore: () => state.authStore,
  useOnboardingStore: () => state.onboardingStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => state.appStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => state.adminSettingsStore,
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

describe('AppHeader contact support entry', () => {
  let wrapper: VueWrapper | undefined

  beforeEach(() => {
    state.appStore.showError.mockReset()
    state.appStore.showSuccess.mockReset()
    document.body.innerHTML = ''
  })

  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    document.body.innerHTML = ''
  })

  it('does not duplicate contact support in the authenticated header', () => {
    wrapper = mount(AppHeader, {
      attachTo: document.body,
      global: {
        stubs: {
          AnnouncementBell: true,
          LocaleSwitcher: true,
          RouterLink: true,
          SubscriptionProgressMini: true,
        },
      },
    })

    expect(wrapper.find('[data-testid="contact-support-trigger"]').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'ContactSupportDialog' }).exists()).toBe(false)
    expect(document.body.textContent).not.toContain('1080152144')
  })
})
