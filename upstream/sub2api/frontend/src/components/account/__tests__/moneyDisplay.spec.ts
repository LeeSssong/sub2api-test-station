import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountCapacityCell from '../AccountCapacityCell.vue'
import AccountStatsModal from '../AccountStatsModal.vue'

const { getStats } = vi.hoisted(() => ({ getStats: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getStats } } }))
vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('vue-chartjs', () => ({ Line: { template: '<div />' }, Doughnut: { template: '<div />' } }))

describe('account cost display', () => {
  it('shows the window cost in two decimals and retains raw capacity limits', () => {
    const wrapper = mount(AccountCapacityCell, {
      props: { account: { id: 1, platform: 'anthropic', type: 'oauth', concurrency: 1, current_window_cost: 0.001, window_cost_limit: 1234.5 } as any },
      global: { stubs: { CapacityBadge: { props: ['current', 'max'], template: '<span>{{ current }}/{{ max }}</span>' }, QuotaBadge: true } },
    })
    expect(wrapper.text()).toContain('$0.00/$1234.50')
  })

  it('does not display an unknown window cost as zero', () => {
    const wrapper = mount(AccountCapacityCell, {
      props: { account: { id: 1, platform: 'anthropic', type: 'oauth', concurrency: 1, current_window_cost: null, window_cost_limit: 10 } as any },
      global: { stubs: { CapacityBadge: { props: ['current', 'max', 'tooltip', 'colorClass'], template: '<span>{{ current }}/{{ max }}|{{ tooltip }}|{{ colorClass }}</span>' }, QuotaBadge: true } },
    })
    expect(wrapper.text()).not.toContain('$0.00/$10.00')
    expect(wrapper.text()).toContain('窗口费用暂不可用')
    expect(wrapper.text()).not.toContain('emerald')
  })

  it('renders summary and chart tooltip costs in two decimals and leaves failed loads without zero cost cards', async () => {
    const account = { id: 1, name: 'Test', status: 'active' } as any
    getStats.mockResolvedValueOnce({
      summary: { total_cost: 1234.5, total_user_cost: -1.2, total_standard_cost: 0.001, total_requests: 3, total_tokens: 100, avg_daily_cost: 0, avg_daily_user_cost: 0, avg_daily_requests: 1, avg_daily_tokens: 10, avg_duration_ms: 10, days: 30, actual_days_used: 1, today: null, highest_cost_day: null, highest_request_day: null },
      history: [], models: [], endpoints: [], upstream_endpoints: [],
    })
    const stubs = { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' }, Icon: true, LoadingSpinner: true, ModelDistributionChart: true, EndpointDistributionChart: true, Line: true }
    const wrapper = mount(AccountStatsModal, { props: { show: false, account }, global: { stubs } })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(wrapper.text()).toContain('$1234.50')
    expect(wrapper.text()).toContain('$-1.20')
    expect(wrapper.text()).toContain('$0.00')
    expect(wrapper.text()).toContain('usage.accountBilled—')
    expect(wrapper.text()).toContain('usage.userBilled—')
    const options = (wrapper.vm as any).$?.setupState.lineChartOptions
    expect(options.plugins.tooltip.callbacks.label({ dataset: { label: 'Cost (USD)' }, raw: 0.001 })).toBe('Cost (USD): $0.00')
    expect(options.scales.y.ticks.callback(1234.5)).toBe('$1234.50')
    getStats.mockRejectedValueOnce(new Error('offline'))
    const failed = mount(AccountStatsModal, { props: { show: false, account }, global: { stubs } })
    await failed.setProps({ show: true })
    await flushPromises()
    expect(failed.text()).not.toContain('$0.00')
  })
})
