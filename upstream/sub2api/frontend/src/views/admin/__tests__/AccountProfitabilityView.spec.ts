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
const updateProcurementCost = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({ adminAPI: { accountFinancial: { getReport }, accounts: { updateProcurementCost }, selfPurchasedProfitability: { get: getSelfPurchased, settle: settleSelfPurchased } } }))

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
  'admin.accountProfitability.summary.netProfit': '经营利润',
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
    updateProcurementCost.mockReset().mockResolvedValue({ procurement_cost_cny: 88, estimated_usable_quota_usd: 60 })
    getSelfPurchased.mockReset().mockResolvedValue({
      start_date: '2026-08-01', end_date: '2026-08-18', generated_at: '2026-08-18T00:00:00Z', currency: 'CNY',
      summary: { procurement_cost_cny: 120, standard_consumed_usd: 30, confirmed_cost_cny: 60, pending_cost_cny: 0, procurement_loss_cny: 60, revenue_cny: 100, net_profit_cny: -20, margin: -0.2, account_count: 1 },
      rows: [{ account_id: 21, name: 'Purchased', platform: 'openai', account_type: 'oauth', status: 'disabled', procurement_cost_cny: 120, estimated_quota_usd: 60, standard_consumed_usd: 30, utilization: 0.5, confirmed_cost_cny: 60, pending_cost_cny: 0, procurement_loss_cny: 60, revenue_cny: 100, net_profit_cny: -20, margin: -0.2, cost_status: 'settled' }],
    })
  })

  it('defaults to USD and loads CNY on demand with the shared range', async () => {
    const wrapper = mountPage()
    await flushPromises()
    expect(getReport).toHaveBeenCalledWith({ range: 'today' })
    expect(getSelfPurchased).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="view-usd"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('[data-test="self-purchased-panel"]').exists()).toBe(false)

    await wrapper.get('[data-test="view-cny"]').trigger('click')
    await flushPromises()
    expect(getSelfPurchased).toHaveBeenCalledWith({ range: 'today' })
    expect(wrapper.find('[data-test="summary-grid"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="scope-all"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="self-purchased-panel"]')).toBeTruthy()
  })

  it('refreshes and changes range only for the active primary view', async () => {
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-test="view-cny"]').trigger('click')
    await flushPromises()
    getReport.mockClear()
    getSelfPurchased.mockClear()

    await wrapper.get('[data-test="range-7d"]').trigger('click')
    await flushPromises()
    expect(getSelfPurchased).toHaveBeenCalledWith({ range: '7d' })
    expect(getReport).not.toHaveBeenCalled()

    getSelfPurchased.mockClear()
    await wrapper.get('[data-test="financial-refresh"]').trigger('click')
    await flushPromises()
    expect(getSelfPurchased).toHaveBeenCalledWith({ range: '7d' })
    expect(getReport).not.toHaveBeenCalled()
  })

  it('reloads CNY when returning after its loaded range becomes stale', async () => {
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-test="view-cny"]').trigger('click')
    await flushPromises()
    expect(getSelfPurchased).toHaveBeenLastCalledWith({ range: 'today' })

    await wrapper.get('[data-test="view-usd"]').trigger('click')
    await wrapper.get('[data-test="range-7d"]').trigger('click')
    await flushPromises()
    getSelfPurchased.mockClear()

    await wrapper.get('[data-test="view-cny"]').trigger('click')
    await flushPromises()
    expect(getSelfPurchased).toHaveBeenCalledWith({ range: '7d' })
  })

  it('renders every CNY field and does not replace zero pending cost with loss', async () => {
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-test="view-cny"]').trigger('click')
    await flushPromises()
    const cards = wrapper.get('[data-test="self-purchased-card-grid"]')
    expect(cards.text()).toContain('采购成本')
    expect(cards.text()).toContain('利用率')
    expect(cards.text()).toContain('采购损失')
    const cells = cards.findAll('[data-metric]').map((cell) => cell.text())
    expect(cells.some((cell) => /CN¥0\.00|¥0\.00/.test(cell))).toBe(true)
    expect(cards.text()).toContain('50.00%')
    expect(cards.text()).toContain('-20.00%')
  })

  it('offers settlement for an expired active procurement version', async () => {
    getSelfPurchased.mockResolvedValueOnce({
      start_date: '2026-08-01', end_date: '2026-08-18', generated_at: '2026-08-18T00:00:00Z', currency: 'CNY',
      summary: { procurement_cost_cny: 120, standard_consumed_usd: 30, confirmed_cost_cny: 60, pending_cost_cny: 60, procurement_loss_cny: 0, revenue_cny: 100, net_profit_cny: 40, margin: 0.4, account_count: 1 },
      rows: [{ account_id: 21, name: 'Expired purchase', platform: 'openai', account_type: 'oauth', status: 'expired', procurement_cost_cny: 120, estimated_quota_usd: 60, standard_consumed_usd: 30, utilization: 0.5, confirmed_cost_cny: 60, pending_cost_cny: 60, procurement_loss_cny: 0, revenue_cny: 100, net_profit_cny: 40, margin: 0.4, cost_status: 'active' }],
    })
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-test="view-cny"]').trigger('click')
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
    expect(wrapper.get('[data-test="account-card-grid"]')).toBeTruthy()

    await wrapper.get('[data-test="scope-group-10"]').trigger('click')
    expect(wrapper.get('[data-test="group-summary-10"]')).toBeTruthy()
    expect(cardIds(wrapper)).toEqual([7, 8])

    await wrapper.get('[data-test="scope-group-0"]').trigger('click')
    expect(cardIds(wrapper)).toEqual([9])
  })

  it('renders one USD card per account with a fixed metric grid', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const grid = wrapper.get('[data-test="account-card-grid"]')
    expect(grid.classes()).toContain('lg:grid-cols-2')
    expect(grid.findAll('[data-test^="account-card-"]')).toHaveLength(3)
    const card = wrapper.get('[data-test="account-card-7"]')
    expect(card.get('[data-metric="operational-cost"]').text()).toContain('$0.25')
    expect(card.get('[data-metric="business-cost"]').text()).toContain('$1.00')
    expect(card.get('[data-metric="business-revenue"]').text()).toContain('$2.50')
    expect(card.get('[data-metric="total-cost"]').text()).toContain('$1.25')
    expect(card.get('[data-metric="net-profit"]').text()).toContain('$1.25')
    expect(card.get('[data-metric="margin"]').text()).toContain('50.00%')
    expect(wrapper.find('[data-test="account-financial-table"]').exists()).toBe(false)
  })

  it('filters USD cards locally by account identity and status without reloading', async () => {
    const wrapper = mountPage()
    await flushPromises()
    getReport.mockClear()

    await wrapper.get('[data-test="account-search"]').setValue('native b')
    expect(wrapper.findAll('article[data-test^="account-card-"]')).toHaveLength(1)
    expect(wrapper.get('[data-test="account-card-8"]').text()).toContain('Native B')

    await wrapper.get('[data-test="account-search"]').setValue('8')
    expect(wrapper.findAll('article[data-test^="account-card-"]')).toHaveLength(1)
    await wrapper.get('[data-test="account-search"]').setValue('sub')
    expect(wrapper.findAll('article[data-test^="account-card-"]')).toHaveLength(3)
    await wrapper.get('[data-test="account-search"]').setValue('active')
    expect(wrapper.findAll('article[data-test^="account-card-"]')).toHaveLength(3)
    expect(getReport).not.toHaveBeenCalled()

    await wrapper.get('[data-test="account-search"]').setValue('does-not-exist')
    expect(wrapper.find('[data-test="financial-empty"]').exists()).toBe(true)

    await wrapper.get('[data-test="account-search"]').setValue('')
    expect(wrapper.findAll('article[data-test^="account-card-"]')).toHaveLength(3)
  })

  it('renders CNY cards with procurement fields and filters cost status locally', async () => {
    getSelfPurchased.mockResolvedValueOnce({
      start_date: '2026-08-01', end_date: '2026-08-18', generated_at: '2026-08-18T00:00:00Z', currency: 'CNY',
      summary: { procurement_cost_cny: 120, standard_consumed_usd: 30, confirmed_cost_cny: 60, pending_cost_cny: 60, procurement_loss_cny: 0, revenue_cny: 100, net_profit_cny: 40, margin: 0.4, account_count: 2 },
      rows: [
        { account_id: 21, name: 'Purchased', platform: 'openai', account_type: 'oauth', status: 'disabled', procurement_cost_cny: 120, estimated_quota_usd: 60, standard_consumed_usd: 30, utilization: 0.5, confirmed_cost_cny: 60, pending_cost_cny: 60, procurement_loss_cny: 0, revenue_cny: 100, net_profit_cny: 40, margin: 0.4, cost_status: 'active' },
        { account_id: 22, name: 'Pending', platform: 'anthropic', account_type: 'oauth', status: 'active', procurement_cost_cny: null, estimated_quota_usd: null, standard_consumed_usd: 0, utilization: null, confirmed_cost_cny: 0, pending_cost_cny: 0, procurement_loss_cny: 0, revenue_cny: 0, net_profit_cny: null, margin: null, cost_status: 'cost_pending' },
      ],
    })
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-test="view-cny"]').trigger('click')
    await flushPromises()

    const grid = wrapper.get('[data-test="self-purchased-card-grid"]')
    expect(grid.classes()).toContain('lg:grid-cols-2')
    expect(grid.findAll('[data-test^="self-purchased-card-"]')).toHaveLength(2)
    const card = wrapper.get('[data-test="self-purchased-card-21"]')
    expect(card.get('[data-metric="procurement-cost"]').text()).toContain('120.00')
    expect(card.get('[data-metric="estimated-quota"]').text()).toContain('60.00 USD')
    expect(card.get('[data-metric="standard-consumed"]').text()).toContain('30.00 USD')
    expect(card.get('[data-metric="utilization"]').text()).toContain('50.00%')
    expect(card.get('[data-metric="confirmed-cost"]').text()).toContain('60.00')
    expect(card.get('[data-metric="pending-cost"]').text()).toContain('60.00')
    expect(card.get('[data-metric="procurement-loss"]').text()).toContain('0.00')
    expect(card.get('[data-metric="revenue"]').text()).toContain('100.00')
    expect(card.get('[data-metric="net-profit"]').text()).toContain('40.00')
    expect(card.get('[data-metric="margin"]').text()).toContain('40.00%')
    expect(card.get('[data-metric="cost-status"]').text()).toContain('active')
    expect(card.get('[data-test="edit-procurement-21"]')).toBeTruthy()
    const zeroFlowCard = wrapper.get('[data-test="self-purchased-card-22"]')
    expect(zeroFlowCard.get('[data-metric="procurement-cost"]').text()).toContain('成本待录入')
    expect(zeroFlowCard.get('[data-metric="standard-consumed"]').text()).toContain('0.00 USD')

    getSelfPurchased.mockClear()
    await wrapper.get('[data-test="account-search"]').setValue('cost_pending')
    expect(wrapper.findAll('article[data-test^="self-purchased-card-"]')).toHaveLength(1)
    expect(wrapper.get('[data-test="self-purchased-card-22"]').text()).toContain('Pending')
    expect(getSelfPurchased).not.toHaveBeenCalled()
    await wrapper.get('[data-test="account-search"]').setValue('does-not-exist')
    expect(wrapper.get('[data-test="self-purchased-empty"]')).toBeTruthy()
  })

  it('uses a single-column contained card grid on a 390px viewport and wraps long names', async () => {
    const host = document.createElement('div')
    host.style.width = '390px'
    document.body.appendChild(host)
    const report = nativeReport()
    report.accounts[0].name = 'A very long account name that must wrap without widening the page'
    getReport.mockResolvedValueOnce(report)
    const wrapper = mountPage({ attachTo: host })
    await flushPromises()
    expect(wrapper.get('[data-test="account-card-grid"]').classes()).toContain('grid-cols-1')
    expect(wrapper.get('[data-test="account-card-7"] [data-test="account-name"]').classes()).toContain('break-words')
    expect(wrapper.get('main').classes()).toContain('overflow-x-hidden')
    wrapper.unmount()
    host.remove()
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
    const grid = wrapper.get('[data-test="account-card-grid"]')
    expect(main.classes()).toContain('overflow-x-hidden')
    expect(grid.classes()).toContain('grid-cols-1')
    expect(grid.classes()).toContain('lg:grid-cols-2')
    expect(main.element.getBoundingClientRect().width).toBeLessThanOrEqual(390)
    wrapper.unmount()
    host.remove()
  })

  it('shows seven CNY summary metrics and uses a contained card grid', async () => {
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-test="view-cny"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="self-purchased-summary-grid"]').findAll(':scope > div')).toHaveLength(7)
    expect(wrapper.get('[data-test="self-summary-account-count"]').text()).toContain('1')
    expect(wrapper.get('[data-test="self-summary-confirmed-cost"]').text()).toMatch(/60\.00/)
    expect(wrapper.get('[data-test="self-summary-pending-cost"]').text()).toMatch(/0\.00/)
    expect(wrapper.get('[data-test="self-purchased-card-grid"]').classes()).toContain('grid-cols-1')
    expect(wrapper.get('[data-test="self-purchased-card-grid"]').classes()).toContain('lg:grid-cols-2')
    expect(wrapper.find('[data-test="scope-all"]').exists()).toBe(false)
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

  it('opens the shared procurement form for every CNY OAuth row and refreshes after save', async () => {
    getSelfPurchased.mockResolvedValue({
      start_date: '2026-08-01', end_date: '2026-08-19', generated_at: '2026-08-19T00:00:00Z', currency: 'CNY',
      summary: { procurement_cost_cny: 0, standard_consumed_usd: 0, confirmed_cost_cny: 0, pending_cost_cny: 0, procurement_loss_cny: 0, revenue_cny: 0, net_profit_cny: null, margin: null, account_count: 1 },
      rows: [{ account_id: 31, name: 'OAuth Pending', platform: 'anthropic', account_type: 'oauth', status: 'active', procurement_cost_cny: null, estimated_quota_usd: null, standard_consumed_usd: 0, utilization: null, confirmed_cost_cny: 0, pending_cost_cny: 0, procurement_loss_cny: 0, revenue_cny: 0, net_profit_cny: null, margin: null, cost_status: 'cost_pending' }],
    })
    const wrapper = mountPage({ global: { stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      AccountMonitorCostDialog: { props: ['account'], emits: ['saveProcurement'], template: `<button data-test="shared-cost-save" @click="$emit('saveProcurement', 88, 60)">{{ account.estimated_usable_quota_usd ?? 60 }}</button>` },
    } } })
    await flushPromises()
    await wrapper.get('[data-test="view-cny"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('成本待录入')
    expect(wrapper.get('[data-test="edit-procurement-31"]').text()).toBe('录入成本')
    await wrapper.get('[data-test="edit-procurement-31"]').trigger('click')
    expect(wrapper.get('[data-test="shared-cost-save"]').text()).toBe('60')
    getSelfPurchased.mockClear()
    await wrapper.get('[data-test="shared-cost-save"]').trigger('click')
    await flushPromises()
    expect(updateProcurementCost).toHaveBeenCalledWith(31, 88, 60, expect.stringContaining('account-procurement-31-'))
    expect(getSelfPurchased).toHaveBeenCalledWith({ range: 'today' })
  })


  it('edits and clears an existing procurement value through the shared API and keeps the dialog on reload failure', async () => {
    const current = {
      start_date: '2026-08-01', end_date: '2026-08-19', generated_at: '2026-08-19T00:00:00Z', currency: 'CNY',
      summary: { procurement_cost_cny: 88, standard_consumed_usd: 0, confirmed_cost_cny: 0, pending_cost_cny: 88, procurement_loss_cny: 0, revenue_cny: 0, net_profit_cny: 0, margin: null, account_count: 1 },
      rows: [{ account_id: 32, name: 'OAuth Configured', platform: 'openai', account_type: 'oauth', status: 'active', procurement_cost_cny: 88, estimated_quota_usd: 60, standard_consumed_usd: 0, utilization: 0, confirmed_cost_cny: 0, pending_cost_cny: 88, procurement_loss_cny: 0, revenue_cny: 0, net_profit_cny: 0, margin: null, cost_status: 'active' }],
    }
    getSelfPurchased.mockResolvedValue(current)
    updateProcurementCost.mockResolvedValue({ procurement_cost_cny: null, estimated_usable_quota_usd: null })
    const wrapper = mountPage({ global: { stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      AccountMonitorCostDialog: { props: ['error'], emits: ['clear'], template: `<div><button data-test="shared-cost-clear" @click="$emit('clear')">clear</button><p data-test="shared-error">{{ error }}</p></div>` },
    } } })
    await flushPromises(); await wrapper.get('[data-test="view-cny"]').trigger('click'); await flushPromises()
    expect(wrapper.get('[data-test="edit-procurement-32"]').text()).toBe('编辑成本')
    await wrapper.get('[data-test="edit-procurement-32"]').trigger('click')
    getSelfPurchased.mockRejectedValueOnce(new Error('reload unavailable'))
    await wrapper.get('[data-test="shared-cost-clear"]').trigger('click'); await flushPromises()
    expect(updateProcurementCost).toHaveBeenCalledWith(32, null, null, expect.stringContaining('account-procurement-32-'))
    expect(wrapper.get('[data-test="shared-error"]').text()).toContain('已保存')
  })

  it('shows the interceptor partial-success message instead of a generic save error', async () => {
    const wrapper = mountPage({ global: { stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      AccountMonitorCostDialog: { props: ['error'], emits: ['saveProcurement'], template: `<div><button data-test="partial-save" @click="$emit('saveProcurement', 90, 60)">save</button><p data-test="partial-error">{{ error }}</p></div>` },
    } } })
    await flushPromises(); await wrapper.get('[data-test="view-cny"]').trigger('click'); await flushPromises()
    await wrapper.get('[data-test="edit-procurement-21"]').trigger('click')
    updateProcurementCost.mockResolvedValueOnce({ procurement_cost_cny: 90, estimated_usable_quota_usd: 60, procurement_readback_status: 'failed', procurement_message: '采购成本已保存，但账号刷新失败，请刷新页面确认' })
    await wrapper.get('[data-test="partial-save"]').trigger('click'); await flushPromises()
    expect(wrapper.get('[data-test="partial-error"]').text()).toContain('已保存')
    expect(wrapper.get('[data-test="partial-error"]').text()).not.toContain('internal')
  })

})
