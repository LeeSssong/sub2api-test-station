import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import AccountProfitabilityView from '../AccountProfitabilityView.vue'
import pageSource from '../AccountProfitabilityView.vue?raw'
import zhAdmin from '@/i18n/locales/zh/admin'
import enAdmin from '@/i18n/locales/en/admin'

const getReport = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({ adminAPI: { accountFinancial: { getReport } } }))

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
  summary: amounts({ requests: 6, tokens: 60, cost: 3.75, user_cost: 7.5, profit: 3.75 }),
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
  beforeEach(() => { getReport.mockReset().mockResolvedValue(nativeReport()) })

  it('shows loading before the first response and all-site summary after success', async () => {
    getReport.mockReturnValueOnce(new Promise(() => undefined))
    const loadingPage = mountPage()
    await nextTick()
    expect(loadingPage.get('[data-test="financial-loading"]')).toBeTruthy()
    expect(loadingPage.find('[data-test="summary-cost"]').exists()).toBe(false)
    loadingPage.unmount()

    const readyPage = mountPage()
    await flushPromises()
    expect(readyPage.findAll('[data-test^="summary-"]').length).toBe(8)
    expect(readyPage.get('[data-test="summary-unconsumed-balance"]').text()).toContain('$90.00')
  })

  it('renders all-site, group, and one-card-per-account hierarchy', async () => {
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('[data-test="scope-all"]')).toBeTruthy()
    expect(cardIds(wrapper)).toEqual([7, 8, 9])
    expect(wrapper.find('[data-test="account-financial-table"]').exists()).toBe(false)
    expect(wrapper.find('.card .card').exists()).toBe(false)

    await wrapper.get('[data-test="scope-group-10"]').trigger('click')
    expect(wrapper.get('[data-test="group-summary-10"]')).toBeTruthy()
    expect(cardIds(wrapper)).toEqual([7, 8])

    await wrapper.get('[data-test="scope-group-0"]').trigger('click')
    expect(cardIds(wrapper)).toEqual([9])
  })

  it('shows user-ledger account cost and independent probe cost in the same account card', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const card = wrapper.get('[data-test="account-card-7"]')
    expect(card.get('[data-metric="cost"]').text()).toContain('$1.25')
    expect(card.get('[data-metric="probe-cost"]').text()).toContain('$0.03')
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
    expect(grid.classes()).toContain('sm:grid-cols-2')
    expect(grid.classes()).not.toContain('lg:grid-cols-3')
    expect(main.element.getBoundingClientRect().width).toBeLessThanOrEqual(390)
    wrapper.unmount()
    host.remove()
  })

  it.each([
    ['requests', [7, 9, 8], [8, 9, 7]],
    ['tokens', [8, 9, 7], [7, 9, 8]],
    ['cost', [7, 9, 8], [8, 9, 7]],
    ['user_cost', [8, 9, 7], [7, 9, 8]],
    ['profit', [8, 9, 7], [7, 9, 8]],
    ['margin', [8, 7, 9], [7, 8, 9]],
  ] as const)('sorts accounts by %s in both directions with null margin last', async (key, descending, ascending) => {
    const report = nativeReport()
    report.accounts = [
      account(7, 'A', { requests: 3, tokens: 100, cost: 4, user_cost: 10, profit: 6, margin: 0.6 }),
      account(8, 'B', { requests: 1, tokens: 300, cost: 2, user_cost: 20, profit: 18, margin: 0.9 }),
      account(9, 'C', { requests: 2, tokens: 200, cost: 3, user_cost: 15, profit: 12, margin: null }),
    ]
    getReport.mockResolvedValueOnce(report)
    const wrapper = mountPage()
    await flushPromises()

    const button = wrapper.get(`[data-test="sort-${key}"]`)
    if (key !== 'requests') await button.trigger('click')
    expect(cardIds(wrapper)).toEqual([...descending])
    await button.trigger('click')
    expect(cardIds(wrapper)).toEqual([...ascending])
  })

  it('keeps sorting state across range, group, and refresh changes', async () => {
    const wrapper = mountPage()
    await flushPromises()
    await wrapper.get('[data-test="sort-tokens"]').trigger('click')
    await wrapper.get('[data-test="sort-tokens"]').trigger('click')
    expect(wrapper.get('[data-test="sort-tokens"]').attributes('aria-sort')).toBe('ascending')

    await wrapper.get('[data-test="scope-group-10"]').trigger('click')
    await wrapper.get('[data-test="range-7d"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="financial-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="sort-tokens"]').attributes('aria-sort')).toBe('ascending')
  })

  it('formats page money as ordinary two-decimal USD and margins with two decimals', async () => {
    const report = nativeReport()
    report.summary = amounts({ cost: 1.234, user_cost: 2.345, profit: 1.111, margin: 0.12345, probe_cost: 0.004 })
    report.accounts[0].amounts = amounts({ cost: 1.234, user_cost: 2.345, profit: 1.111, margin: 0.12345, probe_cost: 0.004 })
    getReport.mockResolvedValueOnce(report)
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('[data-test="summary-cost"]').text()).toContain('$1.23')
    expect(wrapper.get('[data-test="summary-user-cost"]').text()).toContain('$2.35')
    expect(wrapper.get('[data-test="summary-profit"]').text()).toContain('$1.11')
    expect(wrapper.get('[data-test="summary-margin"]').text()).toContain('12.35%')
    expect(wrapper.get('[data-test="summary-probe-cost"]').text()).toContain('$0.00')
    expect(wrapper.get('[data-test="account-card-7"]').text()).not.toContain('e-')
  })

  it('distinguishes no probe records, incomplete cost, and probe query failure', async () => {
    const noRows = nativeReport()
    noRows.summary = amounts({ probe_requests: 0, probe_tokens: 0, probe_cost: 0, probe_cost_status: 'unavailable' })
    noRows.accounts[0].amounts = amounts({ probe_requests: 0, probe_tokens: 0, probe_cost: 0, probe_cost_status: 'unavailable' })
    noRows.accounts[1].amounts = amounts({ probe_requests: 1, probe_tokens: 8, probe_cost: null, probe_cost_status: 'incomplete' })
    getReport.mockResolvedValueOnce(noRows)
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('[data-test="account-card-7"]').text()).toContain('$0.00')
    expect(wrapper.get('[data-test="account-card-7"]').text()).toContain('暂无探测记录')
    expect(wrapper.get('[data-test="account-card-8"]').text()).toContain('—')
    expect(wrapper.get('[data-test="account-card-8"]').text()).toContain('探测用量不完整')

    const failed = nativeReport()
    failed.probe_data_error = true
    failed.probe_error_code = 'probe_aggregate_unavailable'
    failed.summary = amounts({ probe_requests: null, probe_tokens: null, probe_cost: null, probe_cost_status: null })
    failed.accounts = failed.accounts.map((item) => ({
      ...item,
      amounts: amounts({ ...item.amounts, probe_requests: null, probe_tokens: null, probe_cost: null, probe_cost_status: null }),
    }))
    getReport.mockResolvedValueOnce(failed)
    await wrapper.get('[data-test="financial-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="summary-probe-cost"]').text()).toContain('探测数据暂不可用')
    expect(wrapper.get('[data-test="financial-probe-retry"]')).toBeTruthy()
    expect(wrapper.get('[data-test="summary-probe-cost"]').text()).not.toContain('$0.00')
    expect(wrapper.text()).not.toContain('暂无探测记录')
  })

  it('keeps user financial data visible while a probe query fails', async () => {
    const failed = nativeReport()
    failed.probe_data_error = true
    failed.probe_error_code = 'probe_aggregate_unavailable'
    failed.summary = amounts({ cost: 3.75, probe_requests: null, probe_tokens: null, probe_cost: null, probe_cost_status: null })
    getReport.mockResolvedValueOnce(failed)
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.get('[data-test="summary-cost"]').text()).toContain('$3.75')
    expect(wrapper.get('[data-test="account-card-7"]')).toBeTruthy()
  })

  it('keeps existing data visible while refreshing and ignores stale responses', async () => {
    let resolveRefresh!: (value: ReturnType<typeof nativeReport>) => void
    const wrapper = mountPage()
    await flushPromises()
    getReport.mockReturnValueOnce(new Promise((resolve) => { resolveRefresh = resolve }))
    await wrapper.get('[data-test="financial-refresh"]').trigger('click')
    await nextTick()
    expect(wrapper.find('[data-test="financial-refreshing"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="account-card-7"]').exists()).toBe(true)
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
    expect(wrapper.get('[data-test="account-card-17"]').text()).toContain('Newest')
    resolveOld({ ...nativeReport(), range: '24h', accounts: [account(18, 'Stale')] })
    await flushPromises()
    expect(wrapper.find('[data-test="account-card-18"]').exists()).toBe(false)
  })

  it('shows load error and retry without fabricated success cards', async () => {
    getReport.mockRejectedValueOnce(new Error('x'))
    const wrapper = mountPage()
    await flushPromises()
    expect(wrapper.get('[data-test="financial-load-error"]')).toBeTruthy()
    expect(wrapper.get('[data-test="financial-retry"]')).toBeTruthy()
    expect(wrapper.find('[data-test="summary-cost"]').exists()).toBe(false)
  })

  it('resolves production locale keys and keeps forbidden integrations absent', () => {
    expect(zhAdmin.accountProfitability.summary.requests).toBe('请求数')
    expect(enAdmin.accountProfitability.summary.requests).toBe('Requests')
    expect(zhAdmin.accountProfitability.summary.probeCost).toBe('本站探测花费')
    expect(enAdmin.accountProfitability.summary.probeCost).toBe('Site probe cost')
    expect(pageSource).not.toMatch(/\/xingqiao|control-plane|tab=cost-exceptions|setTodayOverride|setOAuthCost/)
    expect(pageSource).not.toMatch(/<main[^>]*min-w-|<main[^>]*w-screen/)
  })
})
