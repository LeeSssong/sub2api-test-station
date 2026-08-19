import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import AccountProfitabilityView from '../AccountProfitabilityView.vue'
import pageSource from '../AccountProfitabilityView.vue?raw'
import zhAdmin from '@/i18n/locales/zh/admin'
import enAdmin from '@/i18n/locales/en/admin'

const getReport = vi.hoisted(() => vi.fn())
const getSelfPurchased = vi.hoisted(() => vi.fn())
const settleSelfPurchased = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({ adminAPI: { accountFinancial: { getReport }, selfPurchasedProfitability: { get: getSelfPurchased, settle: settleSelfPurchased } } }))

const messages: Record<string, string> = {
  'admin.accountProfitability.title': '账号盈利',
  'admin.accountProfitability.description': '原生用量经营指标',
  'admin.accountProfitability.ranges.today': '今日',
  'admin.accountProfitability.ranges.24h': '24 小时',
  'admin.accountProfitability.ranges.7d': '7 天',
  'admin.accountProfitability.ranges.31d': '31 天',
  'admin.accountProfitability.loading': '加载中',
  'admin.accountProfitability.refreshing': '刷新中',
  'admin.accountProfitability.empty': '暂无用量',
  'admin.accountProfitability.loadError': '加载失败',
  'admin.accountProfitability.retry': '重试',
  'admin.accountProfitability.scope.label': '经营维度',
  'admin.accountProfitability.scope.all': '全站',
  'admin.accountProfitability.scope.unassigned': '未归属',
  'admin.accountProfitability.scope.groupSummary': '分组摘要',
  'admin.accountProfitability.scope.accountCount': '{count} 个账号',
  'admin.accountProfitability.summary.requests': '请求数',
  'admin.accountProfitability.summary.tokens': 'Token',
  'admin.accountProfitability.summary.accountCost': '账号计费',
  'admin.accountProfitability.summary.userCost': '用户扣费',
  'admin.accountProfitability.summary.profit': '利润',
  'admin.accountProfitability.summary.margin': '利润率',
  'admin.accountProfitability.summary.operationalCost': '内部运营消耗',
  'admin.accountProfitability.summary.businessCost': '业务消耗',
  'admin.accountProfitability.summary.businessRevenue': '业务营收',
  'admin.accountProfitability.summary.totalCost': '总消耗',
  'admin.accountProfitability.summary.netProfit': '净利润',
  'admin.accountProfitability.summary.externalMargin': '对外毛利率',
  'admin.accountProfitability.summary.includedInTotal': '已包含在总消耗中',
  'admin.accountProfitability.roleHistoryNote': '内部运营按当前管理员角色识别，历史角色变化可能影响历史区间分类',
  'admin.accountProfitability.summary.probeCost': '本站探测花费',
  'admin.accountProfitability.summary.unconsumedBalance': '用户未消费余额',
  'admin.accountProfitability.sort.label': '账号排序',
  'admin.accountProfitability.sort.ascending': '升序',
  'admin.accountProfitability.sort.descending': '降序',
  'admin.accountProfitability.probe.noRecords': '暂无探测记录',
  'admin.accountProfitability.probe.incomplete': '探测用量不完整',
  'admin.accountProfitability.probe.dataError': '探测数据暂不可用',
  'admin.accountProfitability.probe.retry': '重试探测数据',
  'admin.accountProfitability.account.meta': '{platform} · {type}',
  'common.refresh': '刷新',
}

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => (messages[key] ?? key)
      .replace('{count}', String(params?.count ?? ''))
      .replace('{platform}', String(params?.platform ?? ''))
      .replace('{type}', String(params?.type ?? '')),
  }),
}))

type AmountOverrides = Partial<{
  requests: number
  tokens: number
  cost: number
  user_cost: number
  profit: number
  margin: number | null
  operational_cost: number
  business_cost: number
  business_revenue: number
  total_cost: number
  net_profit: number
  external_margin: number | null
  probe_requests: number | null
  probe_tokens: number | null
  probe_cost: number | null
  probe_cost_status: 'confirmed' | 'incomplete' | 'unavailable' | null
}>

const amounts = (overrides: AmountOverrides = {}) => ({
  requests: 2,
  tokens: 10,
  cost: 1.25,
  user_cost: 2.5,
  profit: 1.25,
  margin: 0.5 as number | null,
  operational_cost: 0.25,
  business_cost: 1,
  business_revenue: 2.5,
  total_cost: 1.25,
  net_profit: 1.25,
  external_margin: 0.5 as number | null,
  probe_requests: 1 as number | null,
  probe_tokens: 4 as number | null,
  probe_cost: 0.03 as number | null,
  probe_cost_status: 'confirmed' as 'confirmed' | 'incomplete' | 'unavailable' | null,
  ...overrides,
})

const account = (id: number, name: string, overrides: AmountOverrides = {}) => ({
  id,
  name,
  type: 'api_key',
  platform: 'sub',
  historical: false,
  amounts: amounts(overrides),
})

const nativeReport = () => ({
  generated_at: '2026-08-12T10:00:00Z',
  range: 'today' as const,
  currency: 'USD' as const,
  probe_data_error: false,
  probe_error_code: null,
  summary: amounts({ requests: 6, tokens: 60, cost: 3.75, user_cost: 7.5, business_cost: 3.75, total_cost: 3.75, business_revenue: 7.5, net_profit: 3.75, profit: 3.75 }),
  accounts: [
    account(7, 'Native A', { requests: 3, tokens: 30 }),
    account(8, 'Native B', { requests: 2, tokens: 20 }),
    account(9, 'Native C', { requests: 1, tokens: 10 }),
  ],
  groups: [
    {
      id: 10,
      name: 'Pro',
      unassigned: false,
      historical: false,
      amounts: amounts({ requests: 5, tokens: 50 }),
      accounts: [
        account(7, 'Native A', { requests: 3, tokens: 30 }),
        account(8, 'Native B', { requests: 2, tokens: 20 }),
      ],
    },
    {
      id: 0,
      name: '',
      unassigned: true,
      historical: false,
      amounts: amounts({ requests: 1, tokens: 10 }),
      accounts: [account(9, 'Native C', { requests: 1, tokens: 10 })],
    },
  ],
  user_unconsumed_balance_cny: 90,
})

const mountPage = (options: Record<string, unknown> = {}) => mount(AccountProfitabilityView, {
  global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } },
  ...options,
} as never)

const cardIds = (wrapper: ReturnType<typeof mountPage>) => wrapper
  .findAll('[data-account-id]')
  .map((node) => Number(node.attributes('data-account-id')))

describe('AccountProfitabilityView', () => {
  beforeEach(() => {
    getReport.mockReset().mockResolvedValue(nativeReport())
    settleSelfPurchased.mockReset().mockResolvedValue({ settled: true })
    getSelfPurchased.mockReset().mockResolvedValue({
      start_date: '2026-08-01', end_date: '2026-08-18', generated_at: '2026-08-18T00:00:00Z', currency: 'CNY',
      summary: { procurement_cost_cny: 120, standard_consumed_usd: 30, confirmed_cost_cny: 60, pending_cost_cny: 0, procurement_loss_cny: 60, revenue_cny: 100, net_profit_cny: -20, margin: -0.2, account_count: 1 },
      rows: [{ account_id: 21, name: 'Purchased', platform: 'openai', account_type: 'oauth', status: 'disabled', procurement_cost_cny: 120, estimated_quota_usd: 60, standard_consumed_usd: 30, utilization: 0.5, confirmed_cost_cny: 60, pending_cost_cny: 0, procurement_loss_cny: 60, revenue_cny: 100, net_profit_cny: -20, margin: -0.2, cost_status: 'settled' }],
    })
  })

  it('renders every CNY field and does not replace zero pending cost with loss', async () => {
    const wrapper = mountPage()
    await flushPromises()
    const table = wrapper.get('[data-test="self-purchased-table"]')
    expect(table.text()).toContain('采购成本')
    expect(table.text()).toContain('利用率')
    expect(table.text()).toContain('采购损失')
    const cells = table.findAll('tbody td').map((cell) => cell.text())
    expect(cells.some((cell) => /CN¥0\.00|¥0\.00/.test(cell))).toBe(true)
    expect(table.text()).toContain('50.00%')
    expect(table.text()).toContain('-20.00%')
  })

  it('offers settlement for an expired active procurement version', async () => {
    getSelfPurchased.mockResolvedValueOnce({
      start_date: '2026-08-01', end_date: '2026-08-18', generated_at: '2026-08-18T00:00:00Z', currency: 'CNY',
      summary: { procurement_cost_cny: 120, standard_consumed_usd: 30, confirmed_cost_cny: 60, pending_cost_cny: 60, procurement_loss_cny: 0, revenue_cny: 100, net_profit_cny: 40, margin: 0.4, account_count: 1 },
      rows: [{ account_id: 21, name: 'Expired purchase', platform: 'openai', account_type: 'oauth', status: 'expired', procurement_cost_cny: 120, estimated_quota_usd: 60, standard_consumed_usd: 30, utilization: 0.5, confirmed_cost_cny: 60, pending_cost_cny: 60, procurement_loss_cny: 0, revenue_cny: 100, net_profit_cny: 40, margin: 0.4, cost_status: 'active' }],
    })
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.get('[data-test="settle-21"]').text()).toBe('确认失效')
  })

  it('shows loading before the first response and all-site summary after success', async () => {
    getReport.mockReturnValueOnce(new Promise(() => undefined))
    const loadingPage = mountPage()
    await nextTick()
    expect(loadingPage.get('[data-test="financial-loading"]')).toBeTruthy()
    expect(loadingPage.find('[data-test="summary-cost"]').exists()).toBe(false)
    loadingPage.unmount()

    const readyPage = mountPage()
    await flushPromises()
    expect(readyPage.findAll('[data-test^="summary-"]:not([data-test="summary-grid"])').length).toBe(5)
    expect(readyPage.get('[data-test="summary-business-revenue"]').text()).toContain('$7.50')
  })

  it('renders all-site, group, and account table hierarchy', async () => {
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('[data-test="scope-all"]')).toBeTruthy()
    expect(cardIds(wrapper)).toEqual([7, 8, 9])
    expect(wrapper.get('[data-test="account-financial-table"]')).toBeTruthy()

    await wrapper.get('[data-test="scope-group-10"]').trigger('click')
    expect(wrapper.get('[data-test="group-summary-10"]')).toBeTruthy()
    expect(cardIds(wrapper)).toEqual([7, 8])

    await wrapper.get('[data-test="scope-group-0"]').trigger('click')
    expect(cardIds(wrapper)).toEqual([9])
  })

  it('shows the five approved native result metrics in each account row', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const row = wrapper.get('[data-test="account-row-7"]')
    expect(row.get('[data-metric="operational-cost"]').text()).toContain('$0.25')
    expect(row.get('[data-metric="business-cost"]').text()).toContain('$1.00')
    expect(row.get('[data-metric="business-revenue"]').text()).toContain('$2.50')
    expect(row.get('[data-metric="total-cost"]').text()).toContain('$1.25')
    expect(row.get('[data-metric="net-profit"]').text()).toContain('$1.25')
  })

  it('uses at most two desktop columns and a contained 390px single-column grid', async () => {
    const host = document.createElement('div')
    host.style.width = '390px'
    document.body.appendChild(host)
    const wrapper = mountPage({ attachTo: host })
    await flushPromises()

    const main = wrapper.get('main')
    const grid = wrapper.get('[data-test="account-financial-table-wrap"]')
    expect(main.classes()).toContain('overflow-x-hidden')
    expect(grid.classes()).toContain('overflow-x-auto')
    expect(wrapper.get('[data-test="summary-grid"]').classes()).toContain('grid-cols-2')
    expect(main.element.getBoundingClientRect().width).toBeLessThanOrEqual(390)
    wrapper.unmount()
    host.remove()
  })

  it('places the self-purchased panel below native cards and above scope tabs', async () => {
    const wrapper = mountPage()
    await flushPromises()
    const panel = wrapper.get('[data-test="self-purchased-panel"]')
    const summary = wrapper.get('[data-test="summary-grid"]')
    const scopes = wrapper.get('[data-test="scope-all"]').element.parentElement
    expect(summary.element.compareDocumentPosition(panel.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(panel.element.compareDocumentPosition(scopes!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it.each([
    ['operational_cost', [9, 7, 8], [8, 7, 9]],
    ['business_cost', [7, 8, 9], [9, 8, 7]],
    ['business_revenue', [8, 7, 9], [9, 7, 8]],
    ['total_cost', [7, 9, 8], [8, 9, 7]],
    ['net_profit', [8, 7, 9], [9, 7, 8]],
    ['external_margin', [8, 7, 9], [9, 7, 8]],
  ] as const)('sorts accounts by %s in both directions', async (key, descending, ascending) => {
    const report = nativeReport()
    report.accounts = [
      account(7, 'A', { operational_cost: 1, business_cost: 3, business_revenue: 10, total_cost: 4, net_profit: 6, external_margin: 0.6 }),
      account(8, 'B', { operational_cost: 0.5, business_cost: 1.5, business_revenue: 20, total_cost: 2, net_profit: 18, external_margin: 0.9 }),
      account(9, 'C', { operational_cost: 2, business_cost: 1, business_revenue: 1, total_cost: 3, net_profit: -2, external_margin: -2 }),
    ]
    getReport.mockResolvedValueOnce(report)
    const wrapper = mountPage()
    await flushPromises()

    const button = wrapper.get(`[data-test="sort-${key}"]`)
    if (key !== 'net_profit') await button.trigger('click')
    expect(cardIds(wrapper)).toEqual([...descending])
    await button.trigger('click')
    expect(cardIds(wrapper)).toEqual([...ascending])
  })

  it('keeps sorting state across range, group, and refresh changes', async () => {
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-test="sort-net_profit"]').trigger('click')
    expect(wrapper.get('[data-test="sort-net_profit"]').attributes('aria-sort')).toBe('ascending')

    await wrapper.get('[data-test="scope-group-10"]').trigger('click')
    await wrapper.get('[data-test="range-7d"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="financial-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="sort-net_profit"]').attributes('aria-sort')).toBe('ascending')
  })

  it('formats page money as ordinary two-decimal USD and margins with two decimals', async () => {
    const report = nativeReport()
    report.summary = amounts({ operational_cost: 0.234, business_cost: 1, business_revenue: 2.345, total_cost: 1.234, net_profit: 1.111, external_margin: 0.12345 })
    report.accounts[0].amounts = amounts({ operational_cost: 0.234, business_cost: 1, business_revenue: 2.345, total_cost: 1.234, net_profit: 1.111, external_margin: 0.12345 })
    getReport.mockResolvedValueOnce(report)
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('[data-test="summary-total-cost"]').text()).toContain('$1.23')
    expect(wrapper.get('[data-test="summary-business-revenue"]').text()).toContain('$2.35')
    expect(wrapper.get('[data-test="summary-net-profit"]').text()).toContain('$1.11')
    expect(wrapper.get('[data-test="summary-external-margin"]').text()).toContain('12.35%')
    expect(wrapper.get('[data-test="account-row-7"]').text()).not.toContain('e-')
  })

  it('keeps the native result hierarchy independent of probe status', async () => {
    const failed = nativeReport()
    failed.probe_data_error = true
    failed.probe_error_code = 'probe_aggregate_unavailable'
    failed.summary = amounts({ total_cost: 3.75, net_profit: 3.75, probe_requests: null, probe_tokens: null, probe_cost: null, probe_cost_status: null })
    getReport.mockResolvedValueOnce(failed)
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.get('[data-test="summary-total-cost"]').text()).toContain('$3.75')
    expect(wrapper.get('[data-test="account-row-7"]')).toBeTruthy()
    expect(wrapper.find('[data-test="summary-probe-cost"]').exists()).toBe(false)
  })

  it('keeps existing data visible while refreshing and ignores stale responses', async () => {
    let resolveRefresh!: (value: ReturnType<typeof nativeReport>) => void
    const wrapper = mountPage()
    await flushPromises()
    getReport.mockReturnValueOnce(new Promise((resolve) => { resolveRefresh = resolve }))
    await wrapper.get('[data-test="financial-refresh"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-test="financial-refreshing"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="account-row-7"]').exists()).toBe(true)
    resolveRefresh(nativeReport())
    await flushPromises()

    let resolveOld!: (value: ReturnType<typeof nativeReport>) => void
    let resolveNew!: (value: ReturnType<typeof nativeReport>) => void
    getReport.mockReturnValueOnce(new Promise((resolve) => { resolveOld = resolve }))
      .mockReturnValueOnce(new Promise((resolve) => { resolveNew = resolve }))
    await wrapper.get('[data-test="range-24h"]').trigger('click')
    await wrapper.get('[data-test="range-7d"]').trigger('click')
    resolveNew({ ...nativeReport(), range: '7d', accounts: [account(17, 'Newest')] })
    await flushPromises()
    expect(wrapper.get('[data-test="account-row-17"]').text()).toContain('Newest')
    resolveOld({ ...nativeReport(), range: '24h', accounts: [account(18, 'Stale')] })
    await flushPromises()
    expect(wrapper.find('[data-test="account-row-18"]').exists()).toBe(false)
  })

  it('shows load error and retry without fabricated success cards', async () => {
    getReport.mockRejectedValueOnce(new Error('x'))
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.get('[data-test="financial-load-error"]')).toBeTruthy()
    expect(wrapper.get('[data-test="financial-retry"]')).toBeTruthy()
    expect(wrapper.find('[data-test="summary-business-revenue"]').exists()).toBe(false)
  })

  it('resolves production locale keys and keeps forbidden integrations absent', () => {
    expect(zhAdmin.accountProfitability.summary.requests).toBe('请求数')
    expect(enAdmin.accountProfitability.summary.requests).toBe('Requests')
    expect(zhAdmin.accountProfitability.summary.businessRevenue).toBe('业务营收')
    expect(enAdmin.accountProfitability.summary.businessRevenue).toBe('Business revenue')
    expect(zhAdmin.accountProfitability.summary.probeCost).toBe('本站探测花费')
    expect(enAdmin.accountProfitability.summary.probeCost).toBe('Site probe cost')
    expect(pageSource).not.toMatch(/\/xingqiao|control-plane|tab=cost-exceptions|setTodayOverride|setOAuthCost/)
    expect(pageSource).not.toMatch(/<main[^>]*min-w-|<main[^>]*w-screen/)
  })
})
