import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import EndpointDistributionChart from '../EndpointDistributionChart.vue'
import ModelDistributionChart from '../ModelDistributionChart.vue'
import UserBreakdownSubTable from '../UserBreakdownSubTable.vue'

vi.mock('vue-i18n', async () => ({
  ...(await vi.importActual<typeof import('vue-i18n')>('vue-i18n')),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('vue-chartjs', () => ({ Doughnut: { props: ['data'], template: '<div class="chart-data">{{ JSON.stringify(data) }}</div>' } }))

describe('distribution cost display', () => {
  it('keeps endpoint chart values raw and renders its table and tooltip costs with two decimals', () => {
    const wrapper = mount(EndpointDistributionChart, {
      props: { endpointStats: [{ endpoint: '/v1/test', requests: 1, total_tokens: 100, actual_cost: 0.001, cost: 1234.5 }], metric: 'actual_cost', enableBreakdown: false },
      global: { stubs: { LoadingSpinner: true } },
    })
    expect(wrapper.find('tbody tr').text()).toContain('$0.00')
    expect(wrapper.find('tbody tr').text()).toContain('$1234.50')
    expect(JSON.parse(wrapper.find('.chart-data').text()).datasets[0].data).toEqual([0.001])
    const options = (wrapper.vm as any).$?.setupState.doughnutOptions
    expect(options.plugins.tooltip.callbacks.label({ label: '/v1/test', raw: 0.001, dataset: { data: [0.001] } })).toBe('/v1/test: $0.00 (100.0%)')
  })

  it('shows negative and tiny model costs including ranking tooltip as fixed decimals', () => {
    const wrapper = mount(ModelDistributionChart, {
      props: { modelStats: [{ model: 'm', requests: 1, total_tokens: 100, input_tokens: 100, output_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0, actual_cost: -1.2, account_cost: 0.001, cost: 1234.5 }] },
      global: { stubs: { LoadingSpinner: true } },
    })
    expect(wrapper.find('tbody tr').text()).toContain('$-1.20')
    expect(wrapper.find('tbody tr').text()).toContain('$0.00')
    expect(wrapper.find('tbody tr').text()).toContain('$1234.50')
  })

  it('shows breakdown costs with two decimals while leaving request and token counts untouched', () => {
    const wrapper = mount(UserBreakdownSubTable, {
      props: { items: [{ user_id: 1, email: 'a@b.c', requests: 3, total_tokens: 100, actual_cost: -1.2, account_cost: 0.001, cost: 1234.5 }] },
      global: { stubs: { LoadingSpinner: true } },
    })
    expect(wrapper.text()).toContain('$-1.20')
    expect(wrapper.text()).toContain('$0.00')
    expect(wrapper.text()).toContain('$1234.50')
    expect(wrapper.text()).toContain('100')
  })
})
