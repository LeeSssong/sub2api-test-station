import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AccountStatsModal from '../AccountStatsModal.vue'

const { getStats } = vi.hoisted(() => ({ getStats: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getStats } } }))
vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('vue-chartjs', () => ({ Line: { template: '<div />' } }))

describe('admin account statistics money display', () => {
  it('uses fixed two-decimal amounts for small and large costs including chart labels', async () => {
    getStats.mockResolvedValueOnce({
      summary: {
        total_cost: 1234.5, total_user_cost: 0.001, total_standard_cost: 0,
        total_requests: 1, total_tokens: 1, avg_daily_cost: 0.005,
        avg_daily_user_cost: 0, avg_daily_requests: 1, avg_daily_tokens: 1,
        avg_duration_ms: 1, actual_days_used: 1, today: null,
        highest_cost_day: null, highest_request_day: null,
      },
      history: [], models: [], endpoints: [], upstream_endpoints: [],
    })
    const wrapper = mount(AccountStatsModal, {
      props: { show: false, account: { id: 1, name: 'test', status: 'active' } as any },
      global: { stubs: {
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
        Icon: true, LoadingSpinner: true, ModelDistributionChart: true,
        EndpointDistributionChart: true, Line: true,
      } },
    })
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('$1234.50')
    expect(wrapper.text()).toContain('$0.00')
    expect(wrapper.text()).toContain('$0.01')
    const options = (wrapper.vm as any).$?.setupState.lineChartOptions
    expect(options.scales.y.ticks.callback(0.001)).toBe('$0.00')
    wrapper.unmount()
  })
})
