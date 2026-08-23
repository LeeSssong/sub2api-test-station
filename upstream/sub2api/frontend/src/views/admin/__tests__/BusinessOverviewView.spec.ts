import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BusinessOverviewView from '../BusinessOverviewView.vue'

const getReport = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({ adminAPI: { businessOverview: { getReport } } }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('vue-chartjs', () => ({ Line: { template: '<div data-test="business-trend-chart" />' } }))

const report = {
  generated_at: '2026-08-24T00:00:00Z', timezone: 'Asia/Shanghai', start_date: '2026-08-24', end_date: '2026-08-24', currency: 'CNY', quota_unit: 'Q', quota_unit_label: '内部记账额度，不是美元', revenue_status: 'pending_split',
  summary: { revenue_status: 'pending_split', revenue_cny: null, upstream_cost_cny: 3, gross_profit_cny: null, gross_margin: null, paid_consumption_q: null, gift_consumption_q: 2, gift_upstream_cost_cny: 3, pending_split_count: 1, pending_cost_count: 0 },
  cash_and_balance: { cash_recharge_cny: null, opening_paid_balance_cny: null, paid_quota_issued_cny: null, paid_consumption_cny: null, closing_paid_balance_cny: null, opening_gift_balance_q: null, closing_gift_balance_q: null, net_settlement_cny: null, balance_reconciliation: { status: 'pending', difference_cny: null, adjustments: [] } },
  trend: [{ date: '2026-08-24', cash_recharge_cny: 1, paid_consumption_cny: 0, net_settlement_cny: 1 }],
  groups: [{ group_id: null, group_name: '未归组', unassigned: true, model_count: 1, request_count: 1, configured_multiplier: null, preset_upstream_multiplier: null, preset_margin: null, preset_status: 'unavailable', effective_upstream_multiplier: null, revenue_cny: null, upstream_cost_cny: 3, gross_profit_cny: null, gross_margin: null, revenue_status: 'pending_split' }],
}

describe('BusinessOverviewView', () => {
  beforeEach(() => { getReport.mockReset(); getReport.mockResolvedValue(report) })

  it('renders fixed CNY result cards and pending state without fabricating zero revenue', async () => {
    const wrapper = mount(BusinessOverviewView)
    await flushPromises()
    expect(wrapper.find('[data-test="business-card-revenue"]').text()).toContain('口径待确认')
    expect(wrapper.find('[data-test="business-card-cost"]').text()).toContain('¥3.00')
    expect(wrapper.findAll('[data-test^="business-card-"]')).toHaveLength(4)
    expect(wrapper.find('[data-test="business-pending"]').exists()).toBe(true)
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
