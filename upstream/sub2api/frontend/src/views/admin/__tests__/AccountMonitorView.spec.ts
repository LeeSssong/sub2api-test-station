import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountMonitorView from '../AccountMonitorView.vue'

const {
  list,
  updateSettings,
  runAll,
  runOne,
  history,
  reconciliationOperations,
  reconciliationRefresh,
  reconciliationExceptions,
  reconciliationAdjust,
  updateGroupScoreWeights,
  resetGroupScoreWeights,
} = vi.hoisted(() => ({
  list: vi.fn(),
  updateSettings: vi.fn(),
  runAll: vi.fn(),
  runOne: vi.fn(),
  history: vi.fn(),
  reconciliationOperations: vi.fn(),
  reconciliationRefresh: vi.fn(),
  reconciliationExceptions: vi.fn(),
  reconciliationAdjust: vi.fn(),
  updateGroupScoreWeights: vi.fn(),
  resetGroupScoreWeights: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accountMonitor: {
      list,
      updateSettings,
      runAll,
      runOne,
      history,
      updateGroupScoreWeights,
      resetGroupScoreWeights,
    },
    reconciliation: {
      operations: reconciliationOperations,
      refresh: reconciliationRefresh,
      exceptions: reconciliationExceptions,
      adjust: reconciliationAdjust,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

const account = {
  account_id: 7,
  name: 'Primary Claude',
  platform: 'anthropic',
  account_type: 'oauth',
  status: 'active',
  schedulable: true,
  management_state: 'enabled',
  service_state: 'available',
  group_eligibility: 'eligible',
  monitor_bucket: 'available',
  group_ids: [3],
  group_names: ['Production'],
  model_id: 'claude-sonnet-4-5',
  latest_status: 'success',
  sample_count: 10,
  success_rate: 0.9,
  ttft_p50_ms: 120,
  ttft_p95_ms: 190,
  latency_p95_ms: 840,
  multiplier: { value: 0.1, source: 'declared', status: 'ok' },
  request_count: 12,
  error_count: 1,
  today_stats: { requests: 12, tokens: 100, cost: 0.1, standard_cost: 0.2, user_cost: 0.15 },
  usage_windows: [{ name: '5h', utilization: 0.2, requests: 2, tokens: 100 }],
  checked_at: '2026-07-25T08:00:00Z',
  stale: false,
}

const projection = () => ({
  schema_version: 2,
  observed_at: '2026-07-25T08:01:00Z',
  stale: false,
  settings: {
    interval_seconds: 300,
    updated_by: 1,
    updated_at: '2026-07-25T07:55:00Z',
  },
  health: {
    total_accounts: 3,
    monitoring_accounts: 3,
    available_accounts: 1,
    unavailable_accounts: 1,
    pending_accounts: 1,
    paused_accounts: 0,
    success_rate: 0.9,
    ttft_p50_ms: 120,
    latency_p95_ms: 840,
  },
  accounts: [account],
  groups: [{
    id: 3,
    name: 'Production',
    rate_multiplier: 1,
    customer_visible: true,
    native_order: 0,
    score_weights: { cost: 30, success: 30, ttft: 20, latency: 20 },
    operational_state: 'operational',
    health: {
      total_accounts: 2,
      monitoring_accounts: 1,
      available_accounts: 1,
      unavailable_accounts: 0,
      pending_accounts: 0,
      paused_accounts: 1,
      success_rate: 0.95,
      ttft_p50_ms: 100,
      latency_p95_ms: 700,
    },
  }],
})

function mountView() {
  return mount(AccountMonitorView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: { props: ['show'], template: '<div v-if="show" data-test="base-dialog"><slot /></div>' },
        AccountMonitorCard: {
          props: ['account', 'operations', 'groupOperationalState'],
          emits: ['refresh', 'settings', 'history'],
          template: `
            <article data-test="monitor-card">
              <span>{{ account.name }}</span>
              <span>{{ account.multiplier.value?.toFixed(2) }}x</span>
              <span>{{ account.latest_status }}</span>
              <span>{{ account.error_code }}</span>
              <span>{{ operations?.currency }}</span>
              <button data-test="card-refresh" @click="$emit('refresh', account.account_id)">refresh</button>
              <button data-test="card-settings" @click="$emit('settings')">settings</button>
              <button data-test="card-history" @click="$emit('history', account.account_id)">history</button>
            </article>
          `,
        },
        AccountMonitorFilters: {
          props: ['search', 'platform', 'status'],
          emits: ['update:search', 'update:platform', 'update:status'],
          template: `
            <div data-test="filters">
              <button data-test="search-backup" @click="$emit('update:search', '备份')">搜索备份</button>
              <button data-test="filter-available" @click="$emit('update:status', 'available')">筛选可用</button>
            </div>
          `,
        },
        AccountMonitorSettingsDialog: {
          props: ['show', 'intervalSeconds'],
          emits: ['close', 'save'],
          template: `
            <div v-if="show" data-test="settings-dialog">
              <button data-test="save-settings" @click="$emit('save', 60)">save</button>
            </div>
          `,
        },
        AccountMonitorLedgerHistoryDrawer: {
          props: ['show', 'scope'],
          template: '<div v-if="show" data-test="ledger-history-drawer">{{ scope?.group_id ?? \'global\' }}</div>',
        },
        AccountMonitorGroupScoreDialog: {
          props: ['show', 'groupId', 'weights'],
          emits: ['close', 'save', 'reset'],
          template: '<div v-if="show" data-test="score-dialog"><span>{{ groupId }}</span><button data-test="save-score" @click="$emit(\'save\', { cost: 20, success: 40, ttft: 20, latency: 20 })">save</button><button data-test="reset-score" @click="$emit(\'reset\')">reset</button></div>',
        },
        AccountTodayStatsCell: true,
        AccountUsageCell: true,
        HelpTooltip: true,
        Icon: true,
      },
    },
  })
}

describe('admin account monitor view', () => {
  beforeEach(() => {
    list.mockReset().mockResolvedValue(projection())
    updateSettings.mockReset().mockResolvedValue({
      interval_seconds: 60,
      updated_by: 1,
      updated_at: '2026-07-25T08:02:00Z',
    })
    runAll.mockReset().mockResolvedValue({ completed: 1 })
    runOne.mockReset().mockResolvedValue({ account_id: 7, status: 'success' })
    history.mockReset().mockResolvedValue({ items: [] })
    reconciliationOperations.mockReset().mockResolvedValue({ total_attempts: 10, matched_attempts: 10, pending_attempts: 0, conflict_attempts: 0, coverage_known: true, coverage_ratio: 1, upstream_cost: '1', user_charge: '2', paper_profit: '1', currency: 'EUR', observed_at: '2026-07-25T08:01:00Z' })
    reconciliationRefresh.mockReset().mockResolvedValue({ coverage_known: true, coverage_ratio: 1, pending_attempts: 0 })
    reconciliationExceptions.mockReset().mockResolvedValue({ items: [] })
    reconciliationAdjust.mockReset().mockResolvedValue({})
    updateGroupScoreWeights.mockReset().mockResolvedValue({ cost: 20, success: 40, ttft: 20, latency: 20 })
    resetGroupScoreWeights.mockReset().mockResolvedValue({ cost: 15, success: 45, ttft: 20, latency: 20 })
  })

  it('renders monitored account quality and today-stat projection', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('Primary Claude')
    expect(wrapper.text()).toContain('0.10x')
    expect(wrapper.text()).toContain('success')
    expect(wrapper.get('[data-test="monitor-card"]').exists()).toBe(true)
  })

  it('uses the today global operations ledger and formats its returned currency', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(reconciliationOperations).toHaveBeenCalledWith({})
    const summary = wrapper.get('[data-test="operations-overview"]')
    expect(summary.text()).toContain('€1.00')
    expect(summary.text()).not.toContain('$1.00')
  })

  it('keeps all-site operating and account-service summaries independent', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="global-operating-summary"]').text()).toContain('全站经营数据')
    expect(wrapper.get('[data-test="global-service-summary"]').text()).toContain('全站账号数据')
    expect(wrapper.get('[data-test="global-service-summary"]').text()).toContain('监控中')
    expect(wrapper.get('[data-test="global-service-summary"]').text()).toContain('成本不合格0')
  })

  it('partitions accounts into exactly five monitor buckets and only ranks group available accounts', async () => {
    list.mockResolvedValueOnce({
      ...projection(),
      accounts: [
        { ...account, account_id: 9, name: '全站可用后', monitor_bucket: 'available', quality_score: 99, group_rank: 1 },
        { ...account, account_id: 7, name: '全站可用前', monitor_bucket: 'available', quality_score: 1, group_rank: 9 },
        { ...account, account_id: 8, name: '不可用', monitor_bucket: 'unavailable', service_state: 'unavailable' },
        { ...account, account_id: 10, name: '成本不合格', monitor_bucket: 'cost_ineligible', group_eligibility: 'cost_ineligible' },
        { ...account, account_id: 11, name: '待确认', monitor_bucket: 'pending', service_state: 'pending' },
        { ...account, account_id: 12, name: '暂停', monitor_bucket: 'paused', management_state: 'paused', service_state: 'not_monitored' },
      ],
      groups: [{
        ...projection().groups[0],
        accounts: [
          { ...account, account_id: 7, name: '组内低分', monitor_bucket: 'available', quality_score: 70, group_rank: 2 },
          { ...account, account_id: 9, name: '组内高分', monitor_bucket: 'available', quality_score: 90, group_rank: 1 },
        ],
      }],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="account-section-available"]').findAll('[data-test="monitor-card"]')).toHaveLength(2)
    expect(wrapper.get('[data-test="account-section-unavailable"]').text()).toContain('不可用')
    expect(wrapper.get('[data-test="account-section-cost_ineligible"]').text()).toContain('成本不合格')
    expect(wrapper.get('[data-test="account-section-pending"]').text()).toContain('待确认')
    expect(wrapper.get('[data-test="account-section-paused"]').text()).toContain('暂停')
    expect(wrapper.get('[data-test="account-section-available"]').text().indexOf('全站可用前')).toBeLessThan(wrapper.get('[data-test="account-section-available"]').text().indexOf('全站可用后'))

    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="account-section-available"]').text().indexOf('组内高分')).toBeLessThan(wrapper.get('[data-test="account-section-available"]').text().indexOf('组内低分'))
    expect(wrapper.get('[data-test="group-service-summary"]').text()).toContain('成本不合格0')
  })

  it('deduplicates account projections by account ID in all-site and group scopes', async () => {
    list.mockResolvedValueOnce({
      ...projection(),
      accounts: [
        { ...account, account_id: 7, name: '全站首个投影' },
        { ...account, account_id: 7, name: '全站重复投影' },
        { ...account, account_id: 8, name: '全站另一个账号' },
      ],
      groups: [{
        ...projection().groups[0],
        accounts: [
          { ...account, account_id: 7, name: '分组首个投影', quality_score: 80, group_rank: 1 },
          { ...account, account_id: 7, name: '分组重复投影', quality_score: 60, group_rank: 2 },
          { ...account, account_id: 9, name: '分组另一个账号', quality_score: 70, group_rank: 3 },
        ],
      }],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('当前展示 2 个账号')
    expect(wrapper.get('[data-test="account-section-available"]').findAll('[data-test="monitor-card"]')).toHaveLength(2)
    expect(wrapper.get('[data-test="account-section-available"]').text()).toContain('全站首个投影')
    expect(wrapper.get('[data-test="account-section-available"]').text()).not.toContain('全站重复投影')

    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="group-scope-action-row"]').text()).toContain('可用2')
    expect(wrapper.get('[data-test="account-section-available"]').findAll('[data-test="monitor-card"]')).toHaveLength(2)
    expect(wrapper.get('[data-test="account-section-available"]').text()).toContain('分组首个投影')
    expect(wrapper.get('[data-test="account-section-available"]').text()).not.toContain('分组重复投影')
  })

  it('keeps group summaries and the weight action row visible for empty and search-zero results', async () => {
    list.mockResolvedValueOnce({
      ...projection(),
      groups: [{ ...projection().groups[0], accounts: [] }],
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="group-operating-summary"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="group-service-summary"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="group-scope-action-row"]').text()).toContain('评分权重')
    expect(wrapper.get('[data-test="account-empty"]').exists()).toBe(true)
  })

  it('keeps group summaries and scope counts visible when search returns no accounts', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await wrapper.get('[data-test="search-backup"]').trigger('click')

    expect(wrapper.get('[data-test="group-operating-summary"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="group-service-summary"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="group-scope-action-row"]').text()).toContain('可用1')
    expect(wrapper.get('[data-test="account-empty"]').exists()).toBe(true)
  })

  it('renders Chinese protocol history statuses', async () => {
    history.mockResolvedValueOnce({ items: [{ account_id: 7, model_id: 'claude-sonnet-4-5', status: 'success', checked_at: '2026-07-25T08:00:00Z' }] })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="card-history"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="base-dialog"]').text()).toContain('成功')
    expect(wrapper.get('[data-test="base-dialog"]').text()).not.toContain('success')
  })

  it('discloses global ledger amounts that are not attributed to any group', async () => {
    reconciliationOperations.mockResolvedValue({
      total_attempts: 10,
      matched_attempts: 10,
      pending_attempts: 0,
      conflict_attempts: 0,
      coverage_known: true,
      coverage_ratio: 1,
      upstream_cost: '1',
      user_charge: '2',
      paper_profit: '1',
      currency: 'EUR',
      observed_at: '2026-07-25T08:01:00Z',
      unattributed_attempts: 2,
      unattributed_user_charge: '0.50',
      unattributed_upstream_cost: '0.10',
    })
    const wrapper = mountView()
    await flushPromises()

    const note = wrapper.get('[data-test="unattributed-group-ledger"]')
    expect(note.text()).toContain('未归属分组')
    expect(note.text()).toContain('2 笔请求')
    expect(note.text()).toContain('营收 €0.50')
    expect(note.text()).toContain('成本 €0.10')
  })

  it('hides the unattributed-group note when the ledger has no unattributed amounts', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="unattributed-group-ledger"]').exists()).toBe(false)
  })

  it('切换分组后显示分组历史经营与服务健康', async () => {
    reconciliationOperations.mockImplementation((params: { group_id?: number; start?: string }) => {
      if (params.start && params.group_id === 3) return Promise.resolve({ total_attempts: 50, matched_attempts: 50, pending_attempts: 0, conflict_attempts: 0, coverage_known: true, coverage_ratio: 1, upstream_cost: '10', user_charge: '20', paper_profit: '10', profit_margin: 0.5, currency: 'EUR', observed_at: '2026-07-25T08:01:00Z' })
      if (params.start) return Promise.resolve({ total_attempts: 100, matched_attempts: 100, pending_attempts: 0, conflict_attempts: 0, coverage_known: true, coverage_ratio: 1, upstream_cost: '30', user_charge: '60', paper_profit: '30', profit_margin: 0.5, currency: 'EUR', observed_at: '2026-07-25T08:01:00Z', unattributed_attempts: 4, unattributed_user_charge: '8', unattributed_upstream_cost: '3' })
      return Promise.resolve({ total_attempts: 10, matched_attempts: 10, pending_attempts: 0, conflict_attempts: 0, coverage_known: true, coverage_ratio: 1, upstream_cost: '1', user_charge: '2', paper_profit: '1', profit_margin: 0.5, currency: 'EUR', observed_at: '2026-07-25T08:01:00Z' })
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await flushPromises()

    expect(reconciliationOperations).toHaveBeenCalledWith(expect.objectContaining({ start: '1970-01-01T00:00:00.000Z' }))
    expect(wrapper.get('[data-test="group-lifetime-ledger"]').text()).toContain('€20.00')
    expect(wrapper.find('[data-test="global-lifetime-unattributed-group-ledger"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="group-service-summary"]').text()).toContain('成功率95.0%')
    expect(wrapper.get('[data-test="group-service-summary"]').text()).toContain('TTFT P50100 ms')
  })

  it('全站 Tab 固定在首位，切换分组后加载分组账务', async () => {
    list.mockResolvedValueOnce({
      ...projection(),
      groups: [
        { id: 1, name: 'Archive', rate_multiplier: 0.5, customer_visible: true, native_order: 0, score_weights: { cost: 30, success: 30, ttft: 20, latency: 20 }, operational_state: 'operational' },
        { id: 3, name: 'Production', rate_multiplier: 2, customer_visible: true, native_order: 1, score_weights: { cost: 30, success: 30, ttft: 20, latency: 20 }, operational_state: 'operational' },
        { id: 5, name: 'Overflow', rate_multiplier: 2, customer_visible: true, native_order: 2, score_weights: { cost: 30, success: 30, ttft: 20, latency: 20 }, operational_state: 'operational' },
      ],
      accounts: [
        { ...account, account_id: 8, name: 'Archive Claude', group_ids: [1], group_names: ['Archive'], quality_score: 50 },
        { ...account, account_id: 7, name: 'Production Claude', group_ids: [3, 5], group_names: ['Production', 'Overflow'], quality_score: 90 },
      ],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="all-site-tab"]').text()).toContain('全站')
    expect(wrapper.findAll('[data-test="group-tab"] .font-medium').map((tab) => tab.text())).toEqual(['Production', 'Overflow', 'Archive'])
    expect(wrapper.text()).toContain('Archive Claude')
    expect(wrapper.text()).toContain('Production Claude')
    expect(reconciliationOperations).not.toHaveBeenCalledWith({ group_id: 3 })

    await wrapper.get('[data-test="group-tab-1"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Archive Claude')
    expect(reconciliationOperations).toHaveBeenCalledWith({ group_id: 1 })
    expect(reconciliationOperations).toHaveBeenCalledWith({ group_id: 1, account_id: 8 })
  })

  it('首次加载默认全站，展示全部唯一账号且账务请求不带分组范围', async () => {
    list.mockResolvedValueOnce({
      ...projection(),
      accounts: [
        { ...account, account_id: 7, name: '生产账号', group_ids: [3], group_names: ['生产组'] },
        { ...account, account_id: 8, name: '未分组账号', group_ids: [], group_names: [] },
        {
          ...account,
          account_id: 9,
          name: '暂停账号',
          status: 'paused',
          schedulable: false,
          management_state: 'paused',
          service_state: 'not_monitored',
          group_eligibility: 'not_applicable',
          monitor_bucket: 'paused',
          group_ids: [3],
          group_names: ['生产组'],
        },
      ],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="all-site-tab"]').text()).toContain('全站')
    expect(wrapper.get('[data-test="all-site-tab-button"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.findAll('[data-test="monitor-card"]')).toHaveLength(3)
    expect(wrapper.text()).toContain('生产账号')
    expect(wrapper.text()).toContain('未分组账号')
    expect(wrapper.text()).toContain('暂停账号')
    expect(wrapper.text()).toContain('当前展示 3 个账号')
    expect(wrapper.text()).not.toContain('admin.accountMonitor.monitoredCount')
    expect(reconciliationOperations.mock.calls.map(([params]) => params)).not.toContainEqual(expect.objectContaining({ group_id: expect.any(Number) }))
    expect(reconciliationOperations.mock.calls.map(([params]) => params).every((params) => !Object.hasOwn(params, 'group_id'))).toBe(true)
  })

  it('没有分组时仍保留全站 Tab', async () => {
    list.mockResolvedValueOnce({
      ...projection(),
      groups: [],
      accounts: [{ ...account, name: '未分组账号', group_ids: [], group_names: [] }],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="all-site-tab-button"]').text()).toContain('全站')
    expect(wrapper.get('[data-test="all-site-tab-button"]').attributes('aria-selected')).toBe('true')
  })

  it('opens the daily ledger history from the all-site overview', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="open-ledger-history"]').trigger('click')

    expect(wrapper.get('[data-test="ledger-history-drawer"]').text()).toBe('global')
  })

  it('快速切换分组时保留最新分组账务响应', async () => {
    let resolveProduction: ((value: Record<string, unknown>) => void) | undefined
    reconciliationOperations.mockImplementation((params: { group_id?: number; account_id?: number }) => {
      if (params.group_id === 3 && params.account_id === undefined) {
        return new Promise((resolve) => { resolveProduction = resolve })
      }
      if (params.group_id === 1) return Promise.resolve({ total_attempts: 2, matched_attempts: 2, pending_attempts: 0, conflict_attempts: 0, coverage_known: true, coverage_ratio: 1, upstream_cost: '2', user_charge: '4', paper_profit: '2', currency: 'EUR', observed_at: '2026-07-25T08:01:00Z' })
      return Promise.resolve({ total_attempts: 1, matched_attempts: 1, pending_attempts: 0, conflict_attempts: 0, coverage_known: true, coverage_ratio: 1, upstream_cost: '1', user_charge: '2', paper_profit: '1', currency: 'EUR', observed_at: '2026-07-25T08:01:00Z' })
    })
    list.mockResolvedValueOnce({
      ...projection(),
      groups: [
        { id: 3, name: 'Production', rate_multiplier: 2, customer_visible: true, native_order: 0, score_weights: { cost: 30, success: 30, ttft: 20, latency: 20 }, operational_state: 'operational' },
        { id: 1, name: 'Archive', rate_multiplier: 1, customer_visible: true, native_order: 1, score_weights: { cost: 30, success: 30, ttft: 20, latency: 20 }, operational_state: 'operational' },
      ],
      accounts: [
        { ...account, account_id: 7, group_ids: [3], group_names: ['Production'] },
        { ...account, account_id: 8, name: 'Archive Claude', group_ids: [1], group_names: ['Archive'] },
      ],
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await wrapper.get('[data-test="group-tab-1"]').trigger('click')
    await flushPromises()

    resolveProduction?.({ total_attempts: 9, matched_attempts: 9, pending_attempts: 0, conflict_attempts: 0, coverage_known: true, coverage_ratio: 1, upstream_cost: '9', user_charge: '18', paper_profit: '9', currency: 'USD', observed_at: '2026-07-25T08:01:00Z' })
    await flushPromises()

    expect(wrapper.get('[data-test="group-operations-overview"]').text()).toContain('€2.00')
    expect(wrapper.get('[data-test="group-operations-overview"]').text()).not.toContain('$9.00')
  })

  it('只保存和重置当前选中分组的评分权重', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="edit-group-score-weights"]').trigger('click')
    expect(wrapper.get('[data-test="score-dialog"]').text()).toContain('3')
    await wrapper.get('[data-test="save-score"]').trigger('click')
    await flushPromises()
    expect(updateGroupScoreWeights).toHaveBeenCalledWith(3, { cost: 20, success: 40, ttft: 20, latency: 20 })

    await wrapper.get('[data-test="edit-group-score-weights"]').trigger('click')
    await wrapper.get('[data-test="reset-score"]').trigger('click')
    await flushPromises()
    expect(resetGroupScoreWeights).toHaveBeenCalledWith(3)
  })

  it('说明关闭分组并非面向用户的服务异常', async () => {
    list.mockResolvedValueOnce({
      ...projection(),
      groups: [{ ...projection().groups[0], customer_visible: false, operational_state: 'closed' }],
    })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="group-operating-summary"]').text()).toContain('当前分组未向用户开放')
  })

  it('updates the global interval from the header and card settings action', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="open-settings"]').trigger('click')
    await wrapper.get('[data-test="save-settings"]').trigger('click')
    await flushPromises()

    expect(updateSettings).toHaveBeenCalledWith(60)

    await wrapper.get('[data-test="card-settings"]').trigger('click')
    expect(wrapper.get('[data-test="settings-dialog"]').exists()).toBe(true)
  })

  it('runs the whole pool and one account, including history loading', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="run-all"]').trigger('click')
    await wrapper.get('[data-test="card-refresh"]').trigger('click')
    await wrapper.get('[data-test="card-history"]').trigger('click')
    await flushPromises()

    expect(runAll).toHaveBeenCalledOnce()
    expect(runOne).toHaveBeenCalledWith(7)
    expect(history).toHaveBeenCalledWith(7, 25)
  })

  it('shows stale, failed, and no-history states without removing the card', async () => {
    list.mockResolvedValueOnce({
      ...projection(),
      stale: true,
      accounts: [
        { ...account, latest_status: 'failed', error_code: 'timeout', sample_count: 0, stale: true },
      ],
    })

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="monitor-card"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('failed')
    expect(wrapper.text()).toContain('timeout')
  })

  it('分组 Tab 与搜索、服务状态筛选可组合使用', async () => {
    list.mockResolvedValueOnce({
      ...projection(),
      groups: [
        ...projection().groups,
        {
          ...projection().groups[0],
          id: 5,
          name: 'Overflow',
          native_order: 1,
        },
      ],
      accounts: [
        account,
        {
          ...account,
          account_id: 8,
          name: '备份账号',
          group_ids: [3, 5],
          group_names: ['Production', 'Overflow'],
          latest_status: 'failed',
        },
        {
          ...account,
          account_id: 9,
          name: '其他账号',
          group_ids: [5],
          group_names: ['Overflow'],
          latest_status: 'failed',
        },
      ],
    })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="group-tab-5"]').trigger('click')
    expect(wrapper.findAll('[data-test="monitor-card"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('备份账号')
    expect(wrapper.text()).toContain('其他账号')
    expect(wrapper.text()).not.toContain('Primary Claude')

    await wrapper.get('[data-test="search-backup"]').trigger('click')
    expect(wrapper.findAll('[data-test="monitor-card"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('备份账号')

    await wrapper.get('[data-test="filter-available"]').trigger('click')
    expect(wrapper.findAll('[data-test="monitor-card"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('备份账号')
  })
})
