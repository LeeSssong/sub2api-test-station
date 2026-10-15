import { describe, expect, it, vi } from 'vitest'

const authStore = vi.hoisted(() => ({ checkAuth: vi.fn(), isAuthenticated: true, isAdmin: false, isSimpleMode: false, hasPendingAuthSession: false }))
const appStore = vi.hoisted(() => ({ siteName: 'Sub2API', backendModeEnabled: false, cachedPublicSettings: null }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/stores/adminCompliance', () => ({ useAdminComplianceStore: () => ({ initialized: true, fetchStatus: vi.fn(), requireAcknowledgement: vi.fn() }) }))
vi.mock('@/composables/useNavigationLoading', () => ({ useNavigationLoadingState: () => ({ startNavigation: vi.fn(), endNavigation: vi.fn(), isLoading: { value: false } }) }))
vi.mock('@/composables/useRoutePrefetch', () => ({ useRoutePrefetch: () => ({ triggerPrefetch: vi.fn(), cancelPendingPrefetch: vi.fn(), resetPrefetchState: vi.fn() }) }))

describe('performance monitor route', () => {
  it.each(['/monitor', '/custom/performance-monitor', '/admin/channels', '/admin/channels/pricing'])(
    'shows not found for retired page %s, including trailing slashes', async (path) => {
      const { default: router } = await import('@/router')
      for (const url of [path, path + '/']) {
        const resolved = router.resolve(url)
        expect(resolved.name).toBe('NotFound')
        expect(resolved.matched.at(-1)?.redirect).toBeUndefined()
      }
    },
  )

  it('does not register the retired scheduler decision log route', async () => {
    const { default: router } = await import('@/router')
    const route = router.getRoutes().find((candidate) => candidate.name === 'AdminSchedulerLogs')
    expect(route).toBeUndefined()
  })
})
