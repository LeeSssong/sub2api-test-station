import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BusinessOverviewView from '../BusinessOverviewView.vue'

const getReport = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({ adminAPI: { businessOverview: { getReport } } }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('vue-chartjs', () => ({ Line: { template: '<div data-test="business-trend-chart" />' } }))

const report = {
  generated_at: '2026-08-24T00:00:00Z', timezone: 'Asia/Shanghai', start_date: '2026-08-24', end_date: '2026-08-24', currency: 'CNY', quota_unit: 'Q', quota_unit_label: '内部记账额度，不是美元', revenue_status: 'confirmed',
  summary: { revenue_status: 'confirmed', revenue_cny: 23.35, upstream_cost_cny: 95.31, gross_profit_cny: -71.96, gross_margin: -71.96 / 23.35, paid_consumption_q: 0, gift_consumption_q: 0, gift_upstream_cost_cny: 0, pending_split_count: 0, pending_cost_count: 0 },
  cash_and_balance: { cash_recharge_cny: 0, opening_paid_balance_cny: 0, paid_quota_issued_cny: 0, paid_consumption_cny: 23.35, closing_paid_balance_cny: 0, opening_gift_balance_q: 0, closing_gift_balance_q: 0, net_settlement_cny: -23.35, balance_reconciliation: { status: 'balanced', difference_cny: 0, adjustments: [] } },
  trend: [{ date: '2026-08-24', cash_recharge_cny: 1, paid_consumption_cny: 0, net_settlement_cny: 1 }],
  groups: [{ group_id: null, group_name: '未归组', unassigned: true, model_count: 1, request_count: 1, configured_multiplier: null, preset_upstream_multiplier: null, preset_margin: null, preset_status: 'unavailable', effective_upstream_multiplier: null, revenue_cny: 23.35, upstream_cost_cny: 95.31, gross_profit_cny: -71.96, gross_margin: -71.96 / 23.35, revenue_status: 'confirmed' }],
}

describe('BusinessOverviewView', () => {
  beforeEach(() => { getReport.mockReset(); getReport.mockResolvedValue(report) })

  it('renders numeric CNY result cards without a pending state', async () => {
    const wrapper = mount(BusinessOverviewView)
    await flushPromises()
    expect(wrapper.find('[data-test="business-card-revenue"]').text()).toContain('¥23.35')
    expect(wrapper.find('[data-test="business-card-cost"]').text()).toContain('¥95.31')
    expect(wrapper.findAll('[data-test^="business-card-"]')).toHaveLength(4)
    expect(wrapper.find('[data-test="business-pending"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('待确认')
    expect(wrapper.text()).toContain('内部记账额度，不是美元')
  })

  it('renders balance, trend, and group sections without upstream details', async () => {
    const wrapper = mount(BusinessOverviewView)
    await flushPromises()
    expect(wrapper.find('[data-test="business-cash-balance"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="business-trend-rows"]').text()).toContain('2026-08-24')
    expect(wrapper.find('[data-test="business-trend-chart"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="business-groups"]').text()).toContain('未归组')
    expect(wrapper.text()).not.toContain('上游名称')
  })
})
