import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountProfitabilityView from '../AccountProfitabilityView.vue'
import pageSource from '../AccountProfitabilityView.vue?raw'
const { getReport, setTodayOverride, setOAuthCost, push } = vi.hoisted(() => ({ getReport: vi.fn(), setTodayOverride: vi.fn(), setOAuthCost: vi.fn(), push: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accountFinancial: { getReport, setTodayOverride, setOAuthCost } } }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))
const messages: Record<string, string> = {
  'admin.accountProfitability.title': '账号盈利',
  'admin.accountProfitability.description': '按时间范围查看每个账号的实际收入、支出、盈利与利润率。',
  'admin.accountProfitability.ranges.today': '今日',
  'admin.accountProfitability.ranges.24h': '24 小时',
  'admin.accountProfitability.ranges.7d': '7 天',
  'admin.accountProfitability.ranges.31d': '31 天',
  'admin.accountProfitability.summary.revenue': '收入',
  'admin.accountProfitability.summary.expense': '支出',
  'admin.accountProfitability.summary.profit': '盈利',
  'admin.accountProfitability.summary.margin': '利润率',
  'admin.accountProfitability.summary.exceptions': '异常流水',
  'admin.accountProfitability.summary.unconsumedBalance': '用户未消费余额',
  'admin.accountProfitability.columns.account': '账号',
  'admin.accountProfitability.columns.revenue': '收入',
  'admin.accountProfitability.columns.expense': '支出',
  'admin.accountProfitability.columns.profit': '盈利',
  'admin.accountProfitability.columns.margin': '利润率',
  'admin.accountProfitability.columns.exceptions': '异常',
  'admin.accountProfitability.columns.actions': '今日覆盖',
  'common.refresh': '刷新',
}

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
  }),
}))
const report = () => ({ generated_at: '2026-08-12T10:00:00Z', range: 'today', summary: { revenue: 120, cost: 70, profit: 50, margin: .4167, exception_count: 3, affected_revenue: 20 }, accounts: [{ id: 7, name: 'OAuth', type: 'oauth', platform: 'sub', complete: false, amounts: { revenue: 10, cost: 4, profit: 6, margin: .6, exception_count: 3, affected_revenue: 5 }, exception_count: 3, affected_revenue: 5 }], exception_count: 3, affected_revenue: 5, user_unconsumed_balance_cny: 90 })
describe('AccountProfitabilityView', () => {
  it('keeps the page source free of control-plane symbols and xingqiao paths', () => {
    expect(pageSource).not.toMatch(/controlPlaneAPI|ControlPlaneResponse|ReadModelStatus|useReadModelFreshness|resolveTrustedPageDecision|controlPlaneResponse|controlPlaneDegraded|renderSource|unknown|degraded|integrity|\/api\/v1\/xingqiao|\/xingqiao/)
  })

  beforeEach(() => { getReport.mockReset().mockResolvedValue(report()); setTodayOverride.mockReset().mockResolvedValue({}); setOAuthCost.mockReset().mockResolvedValue({}); push.mockReset(); vi.spyOn(global, 'setInterval'); vi.spyOn(global, 'clearInterval') })
  it('renders Chinese range labels and localized table headers without leaking i18n keys', async () => {
    const wrapper = mount(AccountProfitabilityView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } })
    await flushPromises()

    expect(wrapper.get('[data-test="range-today"]').text()).toBe('今日')
    expect(wrapper.get('[data-test="range-24h"]').text()).toBe('24 小时')
    expect(wrapper.get('[data-test="range-7d"]').text()).toBe('7 天')
    expect(wrapper.get('[data-test="range-31d"]').text()).toBe('31 天')

    const headers = wrapper.findAll('th').map((header) => header.text())
    expect(headers).toEqual(['账号', '收入', '支出', '盈利', '利润率', '异常', '今日覆盖'])

    expect(wrapper.text()).not.toContain('admin.accountProfitability.ranges.24h')
    expect(wrapper.text()).not.toContain('admin.accountProfitability.ranges.31d')
    expect(headers).not.toEqual(['Account', 'Revenue', 'Expense', 'Profit', 'Margin', 'Exceptions', 'Today override'])
  })
  it('uses native admin theme classes for summary cards and the account table', async () => {
    const wrapper = mount(AccountProfitabilityView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } },
    })
    await flushPromises()

    const cardKeys = ['revenue', 'expense', 'profit', 'margin', 'exceptions', 'unconsumed-balance']
    for (const key of cardKeys) {
      const classes = wrapper.get(`[data-test="summary-${key}"]`).classes()
      expect(classes).toContain('card')
      expect(classes).toContain('p-4')
      expect(classes).not.toContain('bg-white')
    }

    const tableWrapper = wrapper.get('[data-test="account-financial-table"]')
    expect(tableWrapper.classes()).toContain('table-container')
    expect(tableWrapper.classes()).not.toContain('bg-white')

    const table = tableWrapper.get('table')
    expect(table.classes()).toContain('table')
    expect(table.classes()).not.toContain('min-w-full')
  })
  it('renders six cards, refreshes through getReport only, and keeps the timer', async () => { const wrapper = mount(AccountProfitabilityView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } }); await flushPromises(); expect(getReport).toHaveBeenCalledWith({ range: 'today' }); expect(wrapper.get('[data-test="financial-generated-at"]').text()).toContain('2026'); for (const key of ['revenue','expense','profit','margin','exceptions','unconsumed-balance']) expect(wrapper.find(`[data-test="summary-${key}"]`).exists()).toBe(true); await wrapper.get('[data-test="financial-refresh"]').trigger('click'); await flushPromises(); expect(getReport).toHaveBeenNthCalledWith(2, { range: 'today' }); expect(setTodayOverride).not.toHaveBeenCalled(); expect(setOAuthCost).not.toHaveBeenCalled(); expect(setInterval).toHaveBeenCalledWith(expect.any(Function), 60_000); wrapper.unmount(); expect(clearInterval).toHaveBeenCalled() })
  it('supports read-only ranges and exception jump', async () => { const wrapper = mount(AccountProfitabilityView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } }); await flushPromises(); await wrapper.get('[data-test="range-7d"]').trigger('click'); await flushPromises(); expect(getReport).toHaveBeenLastCalledWith({ range: '7d' }); expect(wrapper.find('[data-test="account-edit-revenue-7"]').exists()).toBe(false); await wrapper.get('[data-test="range-today"]').trigger('click'); await flushPromises(); await wrapper.get('[data-test="account-exceptions-7"]').trigger('click'); expect(push).toHaveBeenCalledWith(expect.objectContaining({ path: '/admin/usage', query: expect.objectContaining({ tab: 'cost-exceptions', range: 'today', account_id: '7' }) })) })
  it('edits today revenue, cost and oauth daily cost with Beijing business date', async () => { const wrapper = mount(AccountProfitabilityView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } }); await flushPromises(); await wrapper.get('[data-test="account-edit-revenue-7"]').setValue('8'); await wrapper.get('[data-test="account-edit-revenue-7"]').trigger('change'); await wrapper.get('[data-test="account-edit-cost-7"]').setValue('5'); await wrapper.get('[data-test="account-edit-cost-7"]').trigger('change'); await wrapper.get('[data-test="account-edit-oauth-cost-7"]').setValue('4'); await wrapper.get('[data-test="account-edit-oauth-cost-7"]').trigger('change'); expect(setTodayOverride).toHaveBeenNthCalledWith(1, 7, expect.objectContaining({ revenue_cny: 8, business_date: expect.stringMatching(/^\d{4}-\d{2}-\d{2}$/) })); expect(setTodayOverride).toHaveBeenCalledWith(7, expect.objectContaining({ cost_cny: 5 })); expect(setOAuthCost).toHaveBeenCalledWith(7, expect.objectContaining({ cost_cny: 4 })) })
  it('shows account financial fields while edits stay today-only', async () => { const wrapper = mount(AccountProfitabilityView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } }); await flushPromises(); expect(wrapper.get('[data-test="account-financial-7"]').text()).toContain('¥10.00'); expect(wrapper.get('[data-test="account-financial-7"]').text()).toContain('¥4.00'); expect(wrapper.get('[data-test="account-financial-7"]').text()).toContain('¥6.00'); expect(wrapper.get('[data-test="account-financial-7"]').text()).toContain('60.0%'); await wrapper.get('[data-test="range-7d"]').trigger('click'); await flushPromises(); expect(wrapper.find('[data-test="account-edit-cost-7"]').exists()).toBe(false) })
})
