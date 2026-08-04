import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import AccountMonitorView from '../AccountMonitorView.vue'

const {
  list,
  groupsGetAllIncludingInactive,
  getConcurrency,
  runAll,
  runOne,
  updateAccount,
  updateProcurementCost,
  operations,
  refreshReconciliation,
  reconciliationHistory,
  reconciliationExceptions,
  reconciliationAdjust,
  revenue,
  accounting,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  list: vi.fn(),
  groupsGetAllIncludingInactive: vi.fn(),
  getConcurrency: vi.fn(),
  runAll: vi.fn(),
  runOne: vi.fn(),
  updateAccount: vi.fn(),
  updateProcurementCost: vi.fn(),
  operations: vi.fn(),
  refreshReconciliation: vi.fn(),
  reconciliationHistory: vi.fn(),
  reconciliationExceptions: vi.fn(),
  reconciliationAdjust: vi.fn(),
  revenue: vi.fn(),
  accounting: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accountMonitor: { list, getConcurrency, runAll, runOne },
    accounts: { update: updateAccount, updateProcurementCost },
    groups: { getAllIncludingInactive: groupsGetAllIncludingInactive },
    reconciliation: {
      operations,
      refresh: refreshReconciliation,
      history: reconciliationHistory,
      exceptions: reconciliationExceptions,
      adjust: reconciliationAdjust,
    },
    revenue: { list: revenue },
    accounting: { list: accounting },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  const labels: Record<string, string> = {
    'admin.accountMonitor.title': '账号监控',
    'admin.accountMonitor.description': '按分组比较账号服务质量、评分与调度优先级。',
    'admin.accountMonitor.loadError': '账号监控数据加载失败',
    'admin.accountMonitor.actions.refreshAll': '立即刷新全部',
    'admin.accountMonitor.actions.running': '运行中...',
    'admin.accountMonitor.empty.filtered': '没有符合当前筛选条件的账号。',
    'admin.accountMonitor.empty.pool': '当前没有启用且可调度的账号。',
    'admin.accountMonitor.messages.refreshAllSuccess': '全部账号刷新完成',
    'admin.accountMonitor.messages.refreshFailed': '账号刷新失败',
    'common.all': '全部',
    'common.refresh': '刷新',
  }
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => labels[key] ?? key }),
  }
})

const AccountMonitorCardStub = defineComponent({
  name: 'AccountMonitorCardStub',
  props: {
    account: { type: Object, required: true },
    concurrency: { type: Object, default: null },
  },
  emits: ['updatePriority', 'updateProcurementCost', 'refresh'],
  template: `
    <article data-test="monitor-card" :data-account-id="account.account_id">
      <span data-test="card-name">{{ account.name }}</span>
      <span data-test="card-concurrency">{{ concurrency ? concurrency.current + ' / ' + concurrency.limit : '-- / --' }}</span>
      <span v-if="concurrency?.delayed" data-test="card-delayed">数据延迟</span>
    </article>
  `,
})

const baseAccount = {
  account_id: 10,
  name: 'Rank one A',
  platform: 'openai',
  account_type: 'oauth',
  status: 'active',
  schedulable: true,
  management_state: 'enabled',
  service_state: 'available',
  group_eligibility: 'eligible',
  monitor_bucket: 'available',
  priority: 1,
  group_ids: [3],
  group_names: ['GPT-PLUS-内测'],
  model_id: 'gpt-4o-mini',
  latest_status: 'success',
  sample_count: 72,
  success_sample_count: 71,
  ttft_sample_count: 71,
  latency_sample_count: 71,
  success_rate: 0.986,
  ttft_p50_ms: 1018,
  ttft_p95_ms: 1400,
  latency_p95_ms: 1962,
  multiplier: { value: 0.58, source: 'declared', status: 'ok', sample_count: 72 },
  request_count: 72,
  error_count: 1,
  range: '24h',
  base_cost: 18,
  effective_multiplier: 0.48,
  cost_mode: 'multiplier',
  cost_score: 15,
  timeline: Array.from({ length: 24 }, (_, index) => ({
    status: index === 7 ? 'failed' : 'success',
    checked_at: `2026-08-04T04:${String(index).padStart(2, '0')}:00Z`,
    latency_ms: 900 + index * 10,
  })),
  checked_at: '2026-08-04T04:20:42Z',
  stale: false,
  quality_score: 91,
  group_rank: 1,
  eligible: true,
}

function account(accountID: number, name: string, rank: number | null) {
  return { ...baseAccount, account_id: accountID, name, group_rank: rank }
}

function unrankedAccount(range: '24h' | '7d' | '30d') {
  return {
    ...account(30, `Unranked ${range}`, null),
    status: 'disabled',
    schedulable: false,
    management_state: 'disabled',
    service_state: 'not_monitored',
    monitor_bucket: 'paused',
    quality_score: null,
    eligible: false,
  }
}

function projection(range: '24h' | '7d' | '30d' = '24h') {
  const rankedAccounts = [
    account(10, `Rank one A ${range}`, 1),
    account(11, `Rank two ${range}`, 2),
    account(20, `Rank three ${range}`, 3),
    unrankedAccount(range),
  ]
  return {
    schema_version: 3,
    range,
    observed_at: '2026-08-04T04:20:42Z',
    stale: false,
    settings: { interval_seconds: 300, updated_by: 1, updated_at: '2026-08-04T04:15:00Z' },
    health: {
      total_accounts: 4,
      monitoring_accounts: 4,
      available_accounts: 4,
      unavailable_accounts: 0,
      pending_accounts: 0,
      paused_accounts: 0,
      success_rate: 0.98,
      success_sample_count: 280,
      ttft_sample_count: 275,
      latency_sample_count: 275,
    },
    accounts: rankedAccounts,
    groups: [{
      id: 3,
      name: 'GPT-PLUS-内测',
      status: 'active',
      platform: 'openai',
      rate_multiplier: 1.2,
      rpm_limit: 120,
      account_count: 4,
      active_account_count: 3,
      rate_limited_account_count: 1,
      customer_visible: true,
      native_order: 0,
      score_weights: { cost: 30, success: 30, ttft: 20, latency: 20 },
      operational_state: 'operational',
      health: {
        total_accounts: 4,
        monitoring_accounts: 4,
        available_accounts: 4,
        unavailable_accounts: 0,
        pending_accounts: 0,
        paused_accounts: 0,
        success_rate: 0.98,
        success_sample_count: 280,
        ttft_sample_count: 275,
        latency_sample_count: 275,
      },
      accounts: rankedAccounts.map((item) => ({ ...item, timeline: [...item.timeline] })),
    }],
  }
}

const mountedWrappers: ReturnType<typeof mount>[] = []
let documentHidden = false

function mountView(options: { useRealCard?: boolean } = {}) {
  const wrapper = mount(AccountMonitorView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        ...(options.useRealCard ? {} : { AccountMonitorCard: AccountMonitorCardStub }),
        Icon: true,
      },
    },
  })
  mountedWrappers.push(wrapper)
  return wrapper
}

async function selectRange(wrapper: ReturnType<typeof mount>, range: '24h' | '7d' | '30d') {
  await wrapper.get(`[data-test="range-${range}"]`).trigger('click')
  await flushPromises()
}

describe('admin account monitor view V3', () => {
  beforeEach(() => {
    vi.useRealTimers()
    documentHidden = false
    Object.defineProperty(document, 'hidden', { configurable: true, get: () => documentHidden })
    list.mockReset().mockImplementation((range: '24h' | '7d' | '30d') => Promise.resolve(projection(range)))
    groupsGetAllIncludingInactive.mockReset().mockResolvedValue([{
      id: 3,
      name: 'GPT-PLUS-内测',
      status: 'active',
      platform: 'openai',
      rate_multiplier: 1.2,
      rpm_limit: 120,
      account_count: 4,
      active_account_count: 3,
      rate_limited_account_count: 1,
      sort_order: 0,
    }])
    getConcurrency.mockReset().mockResolvedValue({
      items: [
        { account_id: 10, current: 3, limit: 8 },
        { account_id: 11, current: 2, limit: 8 },
        { account_id: 20, current: 1, limit: 8 },
        { account_id: 30, current: 0, limit: 8 },
      ],
    })
    runAll.mockReset().mockResolvedValue({ completed: 4 })
    runOne.mockReset().mockResolvedValue({ account_id: 10, status: 'success' })
    updateAccount.mockReset().mockResolvedValue({ ...baseAccount, priority: 4 })
    updateProcurementCost.mockReset().mockResolvedValue({
      id: 10,
      procurement_cost_cny: 125.5,
      procurement_cost_effective_at: '2026-08-04T04:30:00Z',
    })
    for (const forbidden of [operations, refreshReconciliation, reconciliationHistory, reconciliationExceptions, reconciliationAdjust, revenue, accounting]) {
      forbidden.mockReset().mockResolvedValue({})
    }
    showError.mockReset()
    showSuccess.mockReset()
  })

  afterEach(() => {
    for (const wrapper of mountedWrappers.splice(0)) wrapper.unmount()
    vi.useRealTimers()
    delete (document as Document & { hidden?: boolean }).hidden
  })

  it('loads 24h by default and commits only successful 24h, 7d, and 30d range snapshots', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(list).toHaveBeenNthCalledWith(1, '24h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.get('[data-test="range-24h"]').attributes('aria-pressed')).toBe('true')

    await selectRange(wrapper, '7d')
    expect(list).toHaveBeenNthCalledWith(2, '7d', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.get('[data-test="range-7d"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.text()).toContain('Rank one A 7d')

    await selectRange(wrapper, '30d')
    expect(list).toHaveBeenNthCalledWith(3, '30d', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.get('[data-test="range-30d"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.text()).toContain('Rank one A 30d')
  })

  it('passes the committed selected range to real cards for their call disclosure', async () => {
    const wrapper = mountView({ useRealCard: true })
    await flushPromises()

    await selectRange(wrapper, '7d')

    expect(wrapper.get('[data-test="calls-disclosure"]').text()).toContain('7 天调用')
  })

  it('uses API-provided global service scores and stable global rankings on the all-site tab', async () => {
    const wrapper = mountView({ useRealCard: true })
    await flushPromises()

    const cards = wrapper.findAll('[data-test="monitor-card"]')
    expect(cards.map((card) => card.get('[data-test="account-identity"]').text())).toEqual([
      'Rank one A 24h #10',
      'Rank two 24h #11',
      'Rank three 24h #20',
      'Unranked 24h #30',
    ])
    expect(cards.map((card) => card.get('[data-test="score-metric"]').text())).toEqual([
      expect.stringContaining('账号服务评分'),
      expect.stringContaining('账号服务评分'),
      expect.stringContaining('账号服务评分'),
      expect.stringContaining('账号服务评分'),
    ])
    expect(cards.map((card) => card.get('[data-test="rank-metric"]').text())).toEqual([
      expect.stringContaining('全站排名第 1/ 3'),
      expect.stringContaining('全站排名第 2/ 3'),
      expect.stringContaining('全站排名第 3/ 3'),
      expect.stringContaining('全站排名未排名'),
    ])
  })

  it('retains the last complete snapshot and selected range when a range request fails', async () => {
    list.mockResolvedValueOnce(projection('24h')).mockRejectedValueOnce(new Error('range unavailable'))
    const wrapper = mountView()
    await flushPromises()

    await selectRange(wrapper, '7d')

    expect(wrapper.text()).toContain('Rank one A 24h')
    expect(wrapper.text()).not.toContain('Rank one A 7d')
    expect(wrapper.get('[data-test="range-24h"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-test="range-error"]').text()).toContain('range unavailable')
  })

  it('retains the last complete snapshot when a response range does not match the request', async () => {
    list
      .mockResolvedValueOnce(projection('24h'))
      .mockResolvedValueOnce({
        ...projection('24h'),
        accounts: [account(99, 'Mismatched response', 1)],
      })
    const wrapper = mountView()
    await flushPromises()

    await selectRange(wrapper, '7d')

    expect(wrapper.text()).toContain('Rank one A 24h')
    expect(wrapper.text()).not.toContain('Mismatched response')
    expect(wrapper.get('[data-test="range-24h"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-test="range-error"]').text()).toContain('7d')
    expect(wrapper.get('[data-test="range-error"]').text()).toContain('24h')
  })

  it('renders the constrained V3 shell, one status selector, exactly seven native summary fields, deterministic card order, and responsive columns', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="account-monitor-page"]').classes()).toEqual(expect.arrayContaining(['mx-auto', 'w-full', 'max-w-[1240px]']))
    expect(wrapper.findAll('select')).toHaveLength(1)
    expect(wrapper.get('[data-test="all-site-tab-button"]').attributes('aria-selected')).toBe('true')
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="group-tab-3"]').attributes('aria-selected')).toBe('true')
    const summaryFields = wrapper.findAll('[data-test="group-summary-field"]')
    expect(summaryFields).toHaveLength(7)
    expect(summaryFields.every((field) => field.classes().includes('min-h-[82px]'))).toBe(true)
    expect(summaryFields.map((field) => field.attributes('data-field'))).toEqual([
      'status',
      'platform',
      'rate_multiplier',
      'rpm_limit',
      'account_count',
      'active_account_count',
      'rate_limited_account_count',
    ])
    expect(wrapper.get('[data-test="group-summary"]').text()).toContain('1.20×')
    expect(wrapper.get('[data-test="group-summary"]').text()).toContain('120')

    const cards = wrapper.findAll('[data-test="monitor-card"]')
    expect(cards).toHaveLength(4)
    expect(cards.map((card) => Number(card.attributes('data-account-id')))).toEqual([10, 11, 20, 30])
    expect(wrapper.get('[data-test="account-card-grid"]').classes()).toEqual(expect.arrayContaining(['grid-cols-1', 'lg:grid-cols-2']))
  })

  it('preserves accepted desktop shell spacing while applying only the confirmed mobile overrides', async () => {
    const wrapper = mountView()
    await flushPromises()

    const shell = wrapper.find('.min-h-full')
    const shellClasses = shell.classes()
    expect(shellClasses).toEqual(expect.arrayContaining([
      'px-5',
      'py-8',
      'sm:py-9',
      'max-sm:px-3',
      'max-sm:pt-[22px]',
    ]))
    expect(shellClasses.filter((className) => className.startsWith('max-sm:px-'))).toEqual(['max-sm:px-3'])
    expect(shellClasses.filter((className) => className.startsWith('max-sm:pt-'))).toEqual(['max-sm:pt-[22px]'])
  })

  it('keeps both tab variants pointer-clean with keyboard-only focus feedback and the selected teal underline', async () => {
    const wrapper = mountView()
    await flushPromises()

    const allSiteTab = wrapper.get('[data-test="all-site-tab-button"]')
    const groupTab = wrapper.get('[data-test="group-tab-3"]')

    for (const tab of [allSiteTab, groupTab]) {
      expect(tab.classes()).toEqual(expect.arrayContaining([
        'focus:outline-none',
        'focus-visible:ring-2',
        'focus-visible:ring-primary-500/30',
      ]))
    }

    await groupTab.trigger('click')

    const selectedGroupTab = wrapper.get('[data-test="group-tab-3"]')
    expect(selectedGroupTab.attributes('aria-selected')).toBe('true')
    expect(selectedGroupTab.classes()).toEqual(expect.arrayContaining([
      'text-primary-700',
      'after:absolute',
      'after:inset-x-0',
      'after:bottom-0',
      'after:h-0.5',
      'after:bg-primary-600',
    ]))

    for (const tab of [wrapper.get('[data-test="all-site-tab-button"]'), selectedGroupTab]) {
      const nonKeyboardFocusRectangleClasses = tab.classes().filter((className) => {
        const variantsAndUtility = className.split(':')
        const utility = variantsAndUtility.at(-1) ?? ''
        if (variantsAndUtility.includes('focus-visible')) return false
        return utility.startsWith('ring-')
          || (utility.startsWith('outline-') && utility !== 'outline-none')
      })
      expect(nonKeyboardFocusRectangleClasses).toEqual([])
    }
  })

  it('polls one deduplicated batch for currently visible cards every five seconds, pauses hidden, and refreshes on return', async () => {
    vi.useFakeTimers()
    const wrapper = mountView()
    await flushPromises()

    expect(getConcurrency).toHaveBeenCalledTimes(1)
    expect(getConcurrency).toHaveBeenLastCalledWith([10, 11, 20, 30])

    getConcurrency.mockClear()
    await vi.advanceTimersByTimeAsync(5000)
    expect(getConcurrency).toHaveBeenCalledTimes(1)

    documentHidden = true
    document.dispatchEvent(new Event('visibilitychange'))
    getConcurrency.mockClear()
    await vi.advanceTimersByTimeAsync(10000)
    expect(getConcurrency).not.toHaveBeenCalled()

    documentHidden = false
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(getConcurrency).toHaveBeenCalledTimes(1)
    expect(getConcurrency).toHaveBeenLastCalledWith([10, 11, 20, 30])

    getConcurrency.mockClear()
    await wrapper.get('input[type="search"]').setValue('Rank three')
    await flushPromises()
    expect(getConcurrency).toHaveBeenCalledTimes(1)
    expect(getConcurrency).toHaveBeenLastCalledWith([20])
  })

  it('retains the last successful concurrency values and marks them delayed after a poll failure', async () => {
    vi.useFakeTimers()
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.find('[data-account-id="10"] [data-test="card-concurrency"]').text()).toBe('3 / 8')

    getConcurrency.mockRejectedValueOnce(new Error('concurrency unavailable'))
    await vi.advanceTimersByTimeAsync(5000)
    await flushPromises()

    expect(wrapper.find('[data-account-id="10"] [data-test="card-concurrency"]').text()).toBe('3 / 8')
    expect(wrapper.find('[data-account-id="10"] [data-test="card-delayed"]').exists()).toBe(true)
  })

  it('settles card save completions and reloads the current successful range after each save', async () => {
    const wrapper = mountView()
    await flushPromises()
    await selectRange(wrapper, '7d')
    list.mockClear()

    const priorityCompletion = { resolve: vi.fn(), reject: vi.fn() }
    wrapper.findAllComponents(AccountMonitorCardStub)[0].vm.$emit('updatePriority', 10, 4, priorityCompletion)
    await flushPromises()

    expect(updateAccount).toHaveBeenCalledWith(10, { priority: 4 })
    expect(list).toHaveBeenCalledWith('7d', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(priorityCompletion.resolve).toHaveBeenCalledOnce()
    expect(priorityCompletion.reject).not.toHaveBeenCalled()

    list.mockClear()
    const costCompletion = { resolve: vi.fn(), reject: vi.fn() }
    wrapper.findAllComponents(AccountMonitorCardStub)[0].vm.$emit('updateProcurementCost', 10, 125.5, costCompletion)
    await flushPromises()

    expect(updateProcurementCost).toHaveBeenCalledWith(10, 125.5)
    expect(list).toHaveBeenCalledWith('7d', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(costCompletion.resolve).toHaveBeenCalledOnce()
    expect(costCompletion.reject).not.toHaveBeenCalled()

    updateAccount.mockRejectedValueOnce(new Error('priority rejected'))
    const rejectedCompletion = { resolve: vi.fn(), reject: vi.fn() }
    wrapper.findAllComponents(AccountMonitorCardStub)[0].vm.$emit('updatePriority', 10, 6, rejectedCompletion)
    await flushPromises()

    expect(rejectedCompletion.resolve).not.toHaveBeenCalled()
    expect(rejectedCompletion.reject).toHaveBeenCalledWith(expect.objectContaining({ message: 'priority rejected' }))
  })

  it('invokes and renders no revenue, operations, profit, accounting, ledger, history, reconciliation, adjustment, or exception surface', async () => {
    const wrapper = mountView()
    await flushPromises()

    for (const forbidden of [operations, refreshReconciliation, reconciliationHistory, reconciliationExceptions, reconciliationAdjust, revenue, accounting]) {
      expect(forbidden).not.toHaveBeenCalled()
    }
    const forbiddenText = [
      '营收', '经营', '利润', '账务', '台账', '历史', '对账', '补登记', '调整', '异常',
      'revenue', 'operations', 'profit', 'accounting', 'ledger', 'history', 'reconciliation', 'adjustment', 'exception',
    ]
    const text = wrapper.text().toLowerCase()
    for (const label of forbiddenText) expect(text).not.toContain(label)
  })
})
