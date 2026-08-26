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
  it('registers an authenticated native page route', async () => {
    const { default: router } = await import('@/router')
    const route = router.getRoutes().find((candidate) => candidate.name === 'PerformanceMonitor')
    expect(route?.path).toBe('/custom/performance-monitor')
    expect(route?.meta).toMatchObject({ requiresAuth: true, requiresAdmin: false, titleKey: 'nav.performanceMonitor' })
    expect(route?.components?.default ?? route?.component).toBeDefined()
  })

  it('registers the scheduler workbench as an administrator-only route', async () => {
    const { default: router } = await import('@/router')
    const route = router.getRoutes().find((candidate) => candidate.name === 'AdminSchedulerSettings')

    expect(route?.path).toBe('/admin/scheduler-settings')
    expect(route?.meta).toMatchObject({
      requiresAuth: true,
      requiresAdmin: true,
      titleKey: 'admin.schedulerSettings.title',
    })
    expect(route?.components?.default ?? route?.component).toBeDefined()
  })
})
