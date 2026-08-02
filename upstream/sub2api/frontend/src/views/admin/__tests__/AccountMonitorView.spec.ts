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
  accounts: [account],
  groups: [{
    id: 3,
    name: 'Production',
    rate_multiplier: 1,
    customer_visible: true,
    native_order: 0,
    score_weights: { cost: 30, success: 30, ttft: 20, latency: 20 },
    operational_state: 'operational',
  }],
})

function mountView() {
  return mount(AccountMonitorView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
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
          props: ['search', 'platform', 'status', 'groupId'],
          emits: ['update:search', 'update:platform', 'update:status', 'update:groupId'],
          template: `
            <div data-test="filters">
              <button data-test="select-group-3" @click="$emit('update:groupId', '3')">group 3</button>
              <button data-test="select-group-5" @click="$emit('update:groupId', '5')">group 5</button>
              <button data-test="search-backup" @click="$emit('update:search', 'backup')">search backup</button>
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

  it('orders group tabs by rate and reloads scoped group operations on selection', async () => {
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

    expect(wrapper.findAll('[data-test="group-tab"] .font-medium').map((tab) => tab.text())).toEqual(['Production', 'Overflow', 'Archive'])
    expect(wrapper.text()).toContain('Production Claude')
    expect(reconciliationOperations).toHaveBeenCalledWith({ group_id: 3 })

    await wrapper.get('[data-test="group-tab-1"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Archive Claude')
    expect(reconciliationOperations).toHaveBeenCalledWith({ group_id: 1 })
    expect(reconciliationOperations).toHaveBeenCalledWith({ group_id: 1, account_id: 8 })
  })

  it('opens the daily ledger history from the all-site overview', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="open-ledger-history"]').trigger('click')

    expect(wrapper.get('[data-test="ledger-history-drawer"]').text()).toBe('global')
  })

  it('keeps the newest group operations response when tabs are switched quickly', async () => {
    let resolveProduction: ((value: Record<string, unknown>) => void) | undefined
    reconciliationOperations.mockImplementation((params: { group_id?: number }) => {
      if (params.group_id === 3) return new Promise((resolve) => { resolveProduction = resolve })
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
    await wrapper.get('[data-test="group-tab-1"]').trigger('click')
    await flushPromises()

    resolveProduction?.({ total_attempts: 9, matched_attempts: 9, pending_attempts: 0, conflict_attempts: 0, coverage_known: true, coverage_ratio: 1, upstream_cost: '9', user_charge: '18', paper_profit: '9', currency: 'USD', observed_at: '2026-07-25T08:01:00Z' })
    await flushPromises()

    expect(wrapper.get('[data-test="group-operations-overview"]').text()).toContain('€2.00')
    expect(wrapper.get('[data-test="group-operations-overview"]').text()).not.toContain('$9.00')
  })

  it('saves and resets only the selected group score weights', async () => {
    const wrapper = mountView()
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

  it('describes a closed group as intentionally not user-facing', async () => {
    list.mockResolvedValueOnce({
      ...projection(),
      groups: [{ ...projection().groups[0], customer_visible: false, operational_state: 'closed' }],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="group-operations-overview"]').text()).toContain('当前分组未向用户开放')
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

  it('filters multi-group accounts and composes with search', async () => {
    list.mockResolvedValueOnce({
      ...projection(),
      accounts: [
        account,
        {
          ...account,
          account_id: 8,
          name: 'Backup Claude',
          group_ids: [3, 5],
          group_names: ['Production', 'Overflow'],
        },
        {
          ...account,
          account_id: 9,
          name: 'Other Claude',
          group_ids: [5],
          group_names: ['Overflow'],
        },
      ],
    })
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="select-group-5"]').trigger('click')
    expect(wrapper.findAll('[data-test="monitor-card"]')).toHaveLength(2)
    expect(wrapper.text()).toContain('Backup Claude')
    expect(wrapper.text()).toContain('Other Claude')
    expect(wrapper.text()).not.toContain('Primary Claude')

    await wrapper.get('[data-test="search-backup"]').trigger('click')
    expect(wrapper.findAll('[data-test="monitor-card"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('Backup Claude')
  })
})
