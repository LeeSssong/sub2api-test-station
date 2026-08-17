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
  getAccountById,
  deleteAccount,
  setPrivacy,
  updateProcurementCost,
  updateGroupScoreWeights,
  resetGroupScoreWeights,
  getGlobalScoreWeights,
  updateGlobalScoreWeights,
  resetGlobalScoreWeights,
  getModelDetectionModels,
  saveModelDetectionModels,
  enqueueModelDetection,
  operations,
  costGuard,
  refreshReconciliation,
  reconciliationHistory,
  reconciliationExceptions,
  reconciliationAdjust,
  revenue,
  accounting,
  showError,
  showSuccess,
  controlPlaneMonitor,
  controlPlaneDecision,
} = vi.hoisted(() => ({
  list: vi.fn(),
  groupsGetAllIncludingInactive: vi.fn(),
  getConcurrency: vi.fn(),
  runAll: vi.fn(),
  runOne: vi.fn(),
  updateAccount: vi.fn(),
  getAccountById: vi.fn(),
  deleteAccount: vi.fn(),
  setPrivacy: vi.fn(),
  updateProcurementCost: vi.fn(),
  updateGroupScoreWeights: vi.fn(),
  resetGroupScoreWeights: vi.fn(),
  getGlobalScoreWeights: vi.fn(),
  updateGlobalScoreWeights: vi.fn(),
  resetGlobalScoreWeights: vi.fn(),
  getModelDetectionModels: vi.fn(),
  saveModelDetectionModels: vi.fn(),
  enqueueModelDetection: vi.fn(),
  operations: vi.fn(),
  costGuard: vi.fn(),
  refreshReconciliation: vi.fn(),
  reconciliationHistory: vi.fn(),
  reconciliationExceptions: vi.fn(),
  reconciliationAdjust: vi.fn(),
  revenue: vi.fn(),
  accounting: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  controlPlaneMonitor: vi.fn(),
  controlPlaneDecision: vi.fn(),
}))

vi.mock('@/api/controlPlane', () => ({
  controlPlaneAPI: { monitor: controlPlaneMonitor, decision: controlPlaneDecision },
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accountMonitor: {
      list,
      getConcurrency,
      runAll,
      runOne,
      updateGroupScoreWeights,
      resetGroupScoreWeights,
      getGlobalScoreWeights,
      updateGlobalScoreWeights,
      resetGlobalScoreWeights,
      getModelDetectionModels,
      saveModelDetectionModels,
      enqueueModelDetection,
    },
    accounts: { update: updateAccount, updateProcurementCost, getById: getAccountById, delete: deleteAccount, setPrivacy },
    groups: { getAllIncludingInactive: groupsGetAllIncludingInactive },
    reconciliation: {
      operations,
      costGuard,
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
    'admin.accounts.privacyCfBlocked': '被 Cloudflare 拦截，训练可能仍开启',
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
  emits: ['updatePriority', 'editCost', 'refresh', 'accountInfo', 'accountEdit', 'accountDelete', 'accountMore', 'editConnectionProbeModel', 'saveModelDetectionModels', 'detectModelDetection'],
  template: `
    <article data-test="monitor-card" :data-account-id="account.account_id">
      <span data-test="card-name">{{ account.name }}</span>
      <span data-test="card-concurrency">{{ concurrency ? concurrency.current + ' / ' + concurrency.limit : '-- / --' }}</span>
      <span v-if="concurrency?.delayed" data-test="card-delayed">数据延迟</span>
      <button data-test="edit-cost" type="button" @click="$emit('editCost', account)">edit</button>
      <button data-test="account-info" type="button" @click="$emit('accountInfo', account)">info</button>
      <button data-test="account-edit" type="button" @click="$emit('accountEdit', account)">edit account</button>
      <button data-test="account-delete" type="button" @click="$emit('accountDelete', account)">delete account</button>
      <button data-test="account-more" type="button" @click="$emit('accountMore', account, $event)">more</button>
      <button data-test="model-open" type="button" @click="$emit('editConnectionProbeModel', account)">models</button>
      <button data-test="model-save" type="button" @click="$emit('saveModelDetectionModels', account.account_id, { connectionModel: 'gpt-5.6-sol', detectionModel: 'gpt-5.6-sol' })">save models</button>
      <button data-test="model-detect" type="button" @click="$emit('detectModelDetection', account.account_id)">detect</button>
    </article>
  `,
})

const AccountMonitorCostDialogStub = defineComponent({
  name: 'AccountMonitorCostDialogStub',
  props: {
    show: { type: Boolean, required: true },
    account: { type: Object, required: true },
    saving: { type: Boolean, default: false },
    error: { type: String, default: null },
  },
  emits: ['close', 'saveProcurement', 'saveMultiplier', 'restoreAuto', 'clear'],
  template: `
    <div v-if="show" data-test="cost-dialog">
      <span data-test="dialog-account">{{ account.account_id }}</span>
      <span v-if="error" data-test="dialog-error">{{ error }}</span>
      <button data-test="dialog-save-procurement" @click="$emit('saveProcurement', 4, 60)">save procurement</button>
      <button data-test="dialog-save-multiplier" @click="$emit('saveMultiplier', 0.08)">save multiplier</button>
      <button data-test="dialog-restore-auto" @click="$emit('restoreAuto')">restore</button>
      <button data-test="dialog-clear" @click="$emit('clear')">clear</button>
    </div>
  `,
})

const NativeDialogStub = defineComponent({
  name: 'NativeDialogStub',
  props: { show: { type: Boolean, required: true }, account: { type: Object, default: null } },
  emits: ['close', 'updated', 'reauthorized'],
  template: '<div v-if="show" data-test="native-dialog"><span v-if="account">{{ account.id }}</span></div>',
})

const AccountActionMenuStub = defineComponent({
  name: 'AccountActionMenuStub',
  props: { show: { type: Boolean, required: true }, account: { type: Object, default: null }, position: { type: Object, default: null } },
  emits: ['close', 'test', 'stats', 'schedule', 'duplicate', 'reauth', 'refresh-token', 'recover-state', 'reset-quota', 'set-privacy', 'create-spark-shadow'],
  template: '<div v-if="show" data-test="account-action-menu"><button data-test="menu-test" @click="$emit(\'test\', account)">test</button><button data-test="menu-set-privacy" @click="$emit(\'set-privacy\', account)">privacy</button></div>',
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
        AccountMonitorCostDialog: AccountMonitorCostDialogStub,
        AccountMonitorAccountInfoDialog: NativeDialogStub,
        EditAccountModal: NativeDialogStub,
        ConfirmDialog: defineComponent({ name: 'ConfirmDialogStub', props: { show: Boolean }, emits: ['confirm', 'cancel'], template: '<div v-if="show" data-test="confirm-dialog"><button data-test="confirm" @click="$emit(\'confirm\')">confirm</button></div>' }),
        AccountActionMenu: AccountActionMenuStub,
        AccountTestModal: NativeDialogStub,
        AccountStatsModal: NativeDialogStub,
        ScheduledTestsPanel: NativeDialogStub,
        ReAuthAccountModal: NativeDialogStub,
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
    getModelDetectionModels.mockReset().mockResolvedValue({ account_id: 10, connection_probe_model: 'gpt-5.6-sol', model_detection_model: 'gpt-5.6-sol', connection_models: [{ id: 'gpt-5.6-sol', supported: true, selected: true }], detection_models: [{ id: 'gpt-5.6-sol', supported: true, selected: true }] })
    saveModelDetectionModels.mockReset().mockResolvedValue({ account_id: 10, connection_probe_model: 'gpt-5.6-sol', model_detection_model: 'gpt-5.6-sol', connection_models: [], detection_models: [] })
    enqueueModelDetection.mockReset().mockResolvedValue({ status: 'queued', run_id: 'run-1', reused: false })
    updateAccount.mockReset().mockResolvedValue({ ...baseAccount, priority: 4 })
    getAccountById.mockReset().mockResolvedValue({
      id: 10,
      name: 'Rank one A',
      platform: 'openai',
      type: 'oauth',
      status: 'active',
      schedulable: true,
      priority: 1,
      proxy_id: null,
      concurrency: 0,
      error_message: null,
      last_used_at: null,
      expires_at: null,
      auto_pause_on_expired: false,
      created_at: '2026-08-04T04:00:00Z',
      updated_at: '2026-08-04T04:00:00Z',
      credentials_status: { has_api_key: true },
    })
    deleteAccount.mockReset().mockResolvedValue({ message: 'deleted' })
    setPrivacy.mockReset().mockResolvedValue({ ...baseAccount, id: 10, type: 'oauth', extra: { privacy_mode: 'training_off' } })
    updateProcurementCost.mockReset().mockResolvedValue({
      id: 10,
      procurement_cost_cny: 125.5,
      procurement_cost_effective_at: '2026-08-04T04:30:00Z',
    })
    updateGroupScoreWeights.mockReset().mockResolvedValue({ cost: 25, success: 35, ttft: 20, latency: 20 })
    resetGroupScoreWeights.mockReset().mockResolvedValue({ cost: 30, success: 30, ttft: 20, latency: 20 })
    getGlobalScoreWeights.mockReset().mockResolvedValue({ cost: 15, success: 45, ttft: 20, latency: 20, is_default: true })
    updateGlobalScoreWeights.mockReset().mockResolvedValue({ cost: 30, success: 30, ttft: 20, latency: 20, is_default: false })
    resetGlobalScoreWeights.mockReset().mockResolvedValue({ cost: 15, success: 45, ttft: 20, latency: 20, is_default: true })
    for (const forbidden of [operations, costGuard, refreshReconciliation, reconciliationHistory, reconciliationExceptions, reconciliationAdjust, revenue, accounting]) {
      forbidden.mockReset().mockResolvedValue({})
    }
    costGuard.mockResolvedValue({ status: 'unknown', group_multiplier: 1.2, required_sample_count: 6 })
    showError.mockReset()
    showSuccess.mockReset()
    controlPlaneMonitor.mockReset().mockResolvedValue({
      items: [],
      freshness: {
        generated_at: '2026-08-04T04:20:42Z',
        completeness: 'complete',
        calculation_version: 'accounts-v1',
      },
    })
    controlPlaneDecision.mockReset().mockResolvedValue({ page: 'monitor', requested_mode: 'legacy_only', effective_mode: 'legacy_only', use_external: false, degraded: false, reason: 'legacy_default' })
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

  it('loads native monitor snapshots without page-level control-plane reads', async () => {
    const wrapper = mountView()
    await flushPromises()

    await selectRange(wrapper, '7d')

    expect(list).toHaveBeenNthCalledWith(1, '24h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(list).toHaveBeenNthCalledWith(2, '7d', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(controlPlaneDecision).not.toHaveBeenCalled()
    expect(controlPlaneMonitor).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('Rank one A 7d')
    expect(wrapper.text()).not.toContain('控制面暂时不可用')
    expect(wrapper.text()).not.toContain('完整性：')
    expect(wrapper.text()).not.toContain('来源：现有系统')
    expect(wrapper.text()).not.toContain('来源：控制面')
    expect(wrapper.find('[data-test="read-model-status"]').exists()).toBe(false)
  })

  it('renders an explicit retryable empty state when the first native monitor load fails', async () => {
    list.mockRejectedValueOnce(new Error('initial monitor unavailable'))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="account-monitor-error-empty"]').text()).toContain('initial monitor unavailable')
    expect(wrapper.get('[data-test="account-monitor-error-empty"] button').text()).toContain('刷新')
    expect(wrapper.findAll('[data-test="monitor-card"]')).toHaveLength(0)
    expect(controlPlaneDecision).not.toHaveBeenCalled()
    expect(controlPlaneMonitor).not.toHaveBeenCalled()

    await wrapper.get('[data-test="account-monitor-error-empty"] button').trigger('click')
    await flushPromises()

    expect(list).toHaveBeenNthCalledWith(2, '24h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
  })

  it('does not load reconciled cost guards for account cards', async () => {
    mountView()
    await flushPromises()

    expect(costGuard).not.toHaveBeenCalled()
  })

  it('orders native group tabs by multiplier descending, then native order and id', async () => {
    list.mockResolvedValueOnce({
      ...projection('24h'),
      groups: [
        { ...projection('24h').groups[0], id: 30, name: 'Low multiplier', rate_multiplier: 0.8, native_order: 0 },
        { ...projection('24h').groups[0], id: 20, name: 'Equal later', rate_multiplier: 1.2, native_order: 20 },
        { ...projection('24h').groups[0], id: 10, name: 'High multiplier', rate_multiplier: 1.5, native_order: 99 },
        { ...projection('24h').groups[0], id: 15, name: 'Equal earlier', rate_multiplier: 1.2, native_order: 10 },
      ],
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.findAll('[data-test^="group-tab-"]').map((tab) => tab.text())).toEqual([
      'High multiplier 4',
      'Equal earlier 4',
      'Equal later 4',
      'Low multiplier 4',
    ])
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
    expect(controlPlaneDecision).not.toHaveBeenCalled()
    expect(controlPlaneMonitor).not.toHaveBeenCalled()
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

  it('renders the constrained V3 shell, one status selector, eight selected-group summary fields, deterministic card order, and responsive columns', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="account-monitor-page"]').classes()).toEqual(expect.arrayContaining(['mx-auto', 'w-full', 'max-w-[1240px]']))
    expect(wrapper.findAll('select')).toHaveLength(1)
    expect(wrapper.get('[data-test="all-site-tab-button"]').attributes('aria-selected')).toBe('true')
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="group-tab-3"]').attributes('aria-selected')).toBe('true')
    const summaryFields = wrapper.findAll('[data-test="group-summary-field"]')
    expect(summaryFields).toHaveLength(8)
    expect(summaryFields.every((field) => field.classes().includes('min-h-[82px]'))).toBe(true)
    expect(summaryFields.map((field) => field.attributes('data-field'))).toEqual([
      'status',
      'platform',
      'rate_multiplier',
      'rpm_limit',
      'account_count',
      'active_account_count',
      'rate_limited_account_count',
      'score_weights',
    ])
    expect(wrapper.get('[data-test="group-summary"]').text()).toContain('1.20×')
    expect(wrapper.get('[data-test="group-summary"]').text()).toContain('120')
    expect(wrapper.get('[data-test="edit-group-score-weights"]').exists()).toBe(true)

    const cards = wrapper.findAll('[data-test="monitor-card"]')
    expect(cards).toHaveLength(4)
    expect(cards.map((card) => Number(card.attributes('data-account-id')))).toEqual([10, 11, 20, 30])
    expect(wrapper.get('[data-test="account-card-grid"]').classes()).toEqual(expect.arrayContaining(['grid-cols-1', 'lg:grid-cols-2']))
  })

  it('restores selected-group score weight editing and reloads the active range after save and reset', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="group-summary"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="edit-group-score-weights"]').exists()).toBe(false)

    await selectRange(wrapper, '7d')
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await flushPromises()

    expect(wrapper.findAll('[data-test="group-summary-field"]')).toHaveLength(8)
    await wrapper.get('[data-test="edit-group-score-weights"]').trigger('click')
    const dialog = wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' })
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('groupId')).toBe(3)

    list.mockClear()
    dialog.vm.$emit('save', {
      cost: 25,
      success: 35,
      ttft: 20,
      latency: 20,
      ttft_target_ms: 900,
      ttft_limit_ms: 4500,
      latency_target_ms: 9000,
      latency_limit_ms: 55000,
    })
    await flushPromises()

    expect(updateGroupScoreWeights).toHaveBeenCalledWith(3, {
      cost: 25,
      success: 35,
      ttft: 20,
      latency: 20,
      ttft_target_ms: 900,
      ttft_limit_ms: 4500,
      latency_target_ms: 9000,
      latency_limit_ms: 55000,
    })
    expect(list).toHaveBeenCalledWith('7d', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('show')).toBe(false)

    await wrapper.get('[data-test="edit-group-score-weights"]').trigger('click')
    const reopenedDialog = wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' })
    list.mockClear()
    reopenedDialog.vm.$emit('reset')
    await flushPromises()

    expect(resetGroupScoreWeights).toHaveBeenCalledWith(3)
    expect(list).toHaveBeenCalledWith('7d', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('show')).toBe(false)
  })

  it('shows global score settings only on the all-site view and loads weights when opened', async () => {
    getGlobalScoreWeights.mockResolvedValue({ cost: 15, success: 45, ttft: 20, latency: 20, is_default: true })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="edit-global-score-weights"]').exists()).toBe(true)
    await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
    await flushPromises()

    expect(getGlobalScoreWeights).toHaveBeenCalledTimes(1)
    const dialog = wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' })
    expect(dialog.props('mode')).toBe('global')
    expect(dialog.props('weights')).toMatchObject({ cost: 15, success: 45, ttft: 20, latency: 20 })

    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="edit-global-score-weights"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="edit-group-score-weights"]').exists()).toBe(true)
  })

  it('closes the global score dialog when switching to a group after its weights load', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
    await flushPromises()
    expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('show')).toBe(true)

    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await flushPromises()

    const dialog = wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' })
    expect(dialog.props('mode')).toBe('group')
    expect(dialog.props('show')).toBe(false)
  })

  it('does not open a pending global score dialog after switching to a group', async () => {
    let resolveWeights!: (weights: { cost: number; success: number; ttft: number; latency: number; is_default: boolean }) => void
    getGlobalScoreWeights.mockImplementationOnce(() => new Promise((resolve) => { resolveWeights = resolve }))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    resolveWeights({ cost: 25, success: 35, ttft: 20, latency: 20, is_default: false })
    await flushPromises()

    const dialog = wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' })
    expect(dialog.props('mode')).toBe('group')
    expect(dialog.props('show')).toBe(false)
  })

  it('saves and resets global score weights with four-field payloads and reloads active range', async () => {
    getGlobalScoreWeights.mockResolvedValue({ cost: 15, success: 45, ttft: 20, latency: 20, is_default: true })
    updateGlobalScoreWeights.mockResolvedValue({ cost: 30, success: 30, ttft: 20, latency: 20, is_default: false })
    resetGlobalScoreWeights.mockResolvedValue({ cost: 15, success: 45, ttft: 20, latency: 20, is_default: true })
    const wrapper = mountView()
    await flushPromises()
    await selectRange(wrapper, '7d')
    await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
    await flushPromises()

    list.mockClear()
    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('save', {
      cost: 30,
      success: 30,
      ttft: 20,
      latency: 20,
      ttft_target_ms: 1,
      ttft_limit_ms: 2,
      latency_target_ms: 3,
      latency_limit_ms: 4,
    })
    await flushPromises()

    expect(updateGlobalScoreWeights).toHaveBeenCalledWith({ cost: 30, success: 30, ttft: 20, latency: 20 })
    expect(list).toHaveBeenCalledWith('7d', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('show')).toBe(false)

    await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
    await flushPromises()
    list.mockClear()
    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('reset')
    await flushPromises()

    expect(resetGlobalScoreWeights).toHaveBeenCalledTimes(1)
    expect(list).toHaveBeenCalledWith('7d', expect.objectContaining({ signal: expect.any(AbortSignal) }))
  })

  it('does not let a stale global save completion close or relabel a newly opened group dialog', async () => {
    let resolveSave!: (weights: { cost: number; success: number; ttft: number; latency: number; is_default: boolean }) => void
    updateGlobalScoreWeights.mockImplementationOnce(() => new Promise((resolve) => { resolveSave = resolve }))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
    await flushPromises()
    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('save', {
      cost: 30,
      success: 30,
      ttft: 20,
      latency: 20,
    })
    await flushPromises()
    expect(updateGlobalScoreWeights).toHaveBeenCalledTimes(1)

    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('close')
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await wrapper.get('[data-test="edit-group-score-weights"]').trigger('click')
    showSuccess.mockClear()

    resolveSave({ cost: 30, success: 30, ttft: 20, latency: 20, is_default: false })
    await flushPromises()

    const dialog = wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' })
    expect(dialog.props('mode')).toBe('group')
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('error')).toBeNull()
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('does not let a stale global reset completion close or relabel a newly opened group dialog', async () => {
    let resolveReset!: (weights: { cost: number; success: number; ttft: number; latency: number; is_default: boolean }) => void
    resetGlobalScoreWeights.mockImplementationOnce(() => new Promise((resolve) => { resolveReset = resolve }))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
    await flushPromises()
    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('reset')
    await flushPromises()
    expect(resetGlobalScoreWeights).toHaveBeenCalledTimes(1)

    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('close')
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await wrapper.get('[data-test="edit-group-score-weights"]').trigger('click')
    showSuccess.mockClear()

    resolveReset({ cost: 15, success: 45, ttft: 20, latency: 20, is_default: true })
    await flushPromises()

    const dialog = wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' })
    expect(dialog.props('mode')).toBe('group')
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('error')).toBeNull()
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('does not apply a stale global save reload after a group dialog takes ownership', async () => {
    let resolveReload!: (value: ReturnType<typeof projection>) => void
    const wrapper = mountView()
    await flushPromises()
    list.mockImplementationOnce(() => new Promise((resolve) => { resolveReload = resolve }))

    await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
    await flushPromises()
    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('save', {
      cost: 30,
      success: 30,
      ttft: 20,
      latency: 20,
    })
    await flushPromises()
    expect(list).toHaveBeenCalledTimes(2)

    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('close')
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await wrapper.get('[data-test="edit-group-score-weights"]').trigger('click')
    const groupCostInput = document.body.querySelector<HTMLInputElement>('.modal-overlay input')
    expect(groupCostInput).not.toBeNull()
    groupCostInput!.value = '26'
    groupCostInput!.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    showError.mockClear()
    showSuccess.mockClear()

    const staleProjection = projection('24h')
    staleProjection.accounts[0].name = 'STALE GLOBAL SAVE RELOAD'
    staleProjection.groups[0].accounts[0].name = 'STALE GLOBAL SAVE RELOAD'
    staleProjection.groups[0].score_weights = { cost: 5, success: 55, ttft: 20, latency: 20 }
    resolveReload(staleProjection)
    await flushPromises()

    const currentDialog = wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' })
    expect(currentDialog.props('mode')).toBe('group')
    expect(currentDialog.props('show')).toBe(true)
    expect(document.body.querySelector<HTMLInputElement>('.modal-overlay input')?.value).toBe('26')
    expect(wrapper.text()).not.toContain('STALE GLOBAL SAVE RELOAD')
    expect(wrapper.find('[data-test="range-error"]').exists()).toBe(false)
    expect(currentDialog.props('error')).toBeNull()
    expect(showError).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('does not surface a stale global reset reload failure after a group dialog takes ownership', async () => {
    let rejectReload!: (reason: Error) => void
    const wrapper = mountView()
    await flushPromises()
    list.mockImplementationOnce(() => new Promise((_resolve, reject) => { rejectReload = reject }))

    await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
    await flushPromises()
    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('reset')
    await flushPromises()
    expect(list).toHaveBeenCalledTimes(2)

    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('close')
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await wrapper.get('[data-test="edit-group-score-weights"]').trigger('click')
    const groupCostInput = document.body.querySelector<HTMLInputElement>('.modal-overlay input')
    expect(groupCostInput).not.toBeNull()
    groupCostInput!.value = '26'
    groupCostInput!.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    showError.mockClear()
    showSuccess.mockClear()

    rejectReload(new Error('stale global reset reload failed'))
    await flushPromises()

    const currentDialog = wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' })
    expect(currentDialog.props('mode')).toBe('group')
    expect(currentDialog.props('show')).toBe(true)
    expect(document.body.querySelector<HTMLInputElement>('.modal-overlay input')?.value).toBe('26')
    expect(wrapper.find('[data-test="range-error"]').exists()).toBe(false)
    expect(currentDialog.props('error')).toBeNull()
    expect(showError).not.toHaveBeenCalled()
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('keeps the global score weight dialog open and skips success when save reload fails', async () => {
    const wrapper = mountView()
    await flushPromises()
    await selectRange(wrapper, '7d')
    await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
    await flushPromises()

    list.mockRejectedValueOnce(new Error('reload failed'))
    showSuccess.mockReset()
    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('save', {
      cost: 25,
      success: 35,
      ttft: 20,
      latency: 20,
    })
    await flushPromises()

    expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('show')).toBe(true)
    expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('error')).toContain('最新监控卡片加载失败')
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('shows returned defaults after global reset when projection reload fails', async () => {
    getGlobalScoreWeights.mockResolvedValue({ cost: 60, success: 20, ttft: 10, latency: 10, is_default: false })
    resetGlobalScoreWeights.mockResolvedValue({ cost: 15, success: 45, ttft: 20, latency: 20, is_default: true })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="edit-global-score-weights"]').trigger('click')
    await flushPromises()
    expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('weights')).toMatchObject({
      cost: 60,
      success: 20,
      ttft: 10,
      latency: 10,
    })

    list.mockRejectedValueOnce(new Error('reload failed'))
    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('reset')
    await flushPromises()

    const dialog = wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' })
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('weights')).toMatchObject({ cost: 15, success: 45, ttft: 20, latency: 20 })
    expect(dialog.props('error')).toContain('最新监控卡片加载失败')
  })

  it('keeps the score weight dialog open and skips success when save reload fails', async () => {
    const wrapper = mountView()
    await flushPromises()
    await selectRange(wrapper, '7d')
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await wrapper.get('[data-test="edit-group-score-weights"]').trigger('click')

    list.mockRejectedValueOnce(new Error('reload failed'))
    showSuccess.mockReset()
    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('save', {
      cost: 25,
      success: 35,
      ttft: 20,
      latency: 20,
      ttft_target_ms: 900,
      ttft_limit_ms: 4500,
      latency_target_ms: 9000,
      latency_limit_ms: 55000,
    })
    await flushPromises()

    expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('show')).toBe(true)
    expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('error')).toContain('最新监控卡片加载失败')
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('keeps the score weight dialog open and skips success when reset reload fails', async () => {
    const wrapper = mountView()
    await flushPromises()
    await selectRange(wrapper, '7d')
    await wrapper.get('[data-test="group-tab-3"]').trigger('click')
    await wrapper.get('[data-test="edit-group-score-weights"]').trigger('click')

    list.mockRejectedValueOnce(new Error('reload failed'))
    showSuccess.mockReset()
    wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).vm.$emit('reset')
    await flushPromises()

    expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('show')).toBe(true)
    expect(wrapper.getComponent({ name: 'AccountMonitorGroupScoreDialog' }).props('error')).toContain('最新监控卡片加载失败')
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('preserves the accepted shell spacing and exact responsive page-header typography', async () => {
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

    const titleClasses = wrapper.get('h1').classes()
    expect(titleClasses).toEqual(expect.arrayContaining([
      'text-[27px]',
      'leading-[1.25]',
      'max-[430px]:text-[23px]',
    ]))
    expect(titleClasses).not.toContain('text-2xl')
    expect(titleClasses.filter((className) => className.startsWith('max-[430px]:text-'))).toEqual([
      'max-[430px]:text-[23px]',
    ])

    const descriptionClasses = wrapper.get('header p').classes()
    expect(descriptionClasses).toEqual(expect.arrayContaining([
      'mt-[7px]',
      'max-[760px]:max-w-[272px]',
    ]))
    expect(descriptionClasses).not.toContain('mt-1')
    expect(descriptionClasses).not.toContain('max-[760px]:max-w-[34ch]')
    expect(descriptionClasses.filter((className) => className.startsWith('max-[760px]:max-w-'))).toEqual([
      'max-[760px]:max-w-[272px]',
    ])
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

  it('blurs pointer-originated all-site and dynamic-group selections after selecting the requested scope', async () => {
    const wrapper = mountView()
    await flushPromises()

    const groupTab = wrapper.get('[data-test="group-tab-3"]')
    const groupBlur = vi.spyOn(groupTab.element as HTMLButtonElement, 'blur')
    groupTab.element.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }))
    await flushPromises()

    expect(groupBlur).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-test="group-tab-3"]').attributes('aria-selected')).toBe('true')

    const allSiteTab = wrapper.get('[data-test="all-site-tab-button"]')
    const allSiteBlur = vi.spyOn(allSiteTab.element as HTMLButtonElement, 'blur')
    allSiteTab.element.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 }))
    await flushPromises()

    expect(allSiteBlur).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-test="all-site-tab-button"]').attributes('aria-selected')).toBe('true')
  })

  it('keeps keyboard-originated all-site and dynamic-group activation focused with the focus-visible contract', async () => {
    const wrapper = mountView()
    await flushPromises()

    const groupTab = wrapper.get('[data-test="group-tab-3"]')
    const groupBlur = vi.spyOn(groupTab.element as HTMLButtonElement, 'blur')
    groupTab.element.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 0 }))
    await flushPromises()

    expect(groupBlur).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="group-tab-3"]').attributes('aria-selected')).toBe('true')

    const allSiteTab = wrapper.get('[data-test="all-site-tab-button"]')
    const allSiteBlur = vi.spyOn(allSiteTab.element as HTMLButtonElement, 'blur')
    allSiteTab.element.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 0 }))
    await flushPromises()

    expect(allSiteBlur).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="all-site-tab-button"]').attributes('aria-selected')).toBe('true')

    for (const tab of [wrapper.get('[data-test="all-site-tab-button"]'), wrapper.get('[data-test="group-tab-3"]')]) {
      expect(tab.classes()).toEqual(expect.arrayContaining([
        'focus:outline-none',
        'focus-visible:ring-2',
        'focus-visible:ring-primary-500/30',
      ]))
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

  it('keeps the old card snapshot and reports failure when a completed single probe cannot reload the current range', async () => {
    list
      .mockResolvedValueOnce(projection('24h'))
      .mockRejectedValueOnce(new Error('monitor reload unavailable'))
    const wrapper = mountView()
    await flushPromises()

    wrapper.findAllComponents(AccountMonitorCardStub)[0].vm.$emit('refresh', 10)
    await flushPromises()

    expect(runOne).toHaveBeenCalledWith(10)
    expect(list).toHaveBeenLastCalledWith('24h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(wrapper.text()).toContain('Rank one A 24h')
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('探测已完成，但最新卡片加载失败，请重试')
    expect(controlPlaneDecision).not.toHaveBeenCalled()
    expect(controlPlaneMonitor).not.toHaveBeenCalled()
  })

  it('reports explicit success only after a single probe and current-range reload both complete', async () => {
    const refreshed = projection('24h')
    refreshed.accounts = refreshed.accounts.map((item) => item.account_id === 10
      ? { ...item, name: 'Rank one refreshed', checked_at: '2026-08-04T04:30:00Z' }
      : item)
    list
      .mockResolvedValueOnce(projection('24h'))
      .mockResolvedValueOnce(refreshed)
    const wrapper = mountView()
    await flushPromises()

    wrapper.findAllComponents(AccountMonitorCardStub)[0].vm.$emit('refresh', 10)
    await flushPromises()

    expect(wrapper.text()).toContain('Rank one refreshed')
    expect(showError).not.toHaveBeenCalled()
    expect(showSuccess).toHaveBeenCalledWith('账号探测与监控卡片已刷新')
    expect(list).toHaveBeenNthCalledWith(1, '24h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(list).toHaveBeenNthCalledWith(2, '24h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(controlPlaneDecision).not.toHaveBeenCalled()
    expect(controlPlaneMonitor).not.toHaveBeenCalled()
  })

  it('opens the page-level cost dialog and persists procurement, multiplier, restore, and clear actions', async () => {
    const wrapper = mountView()
    await flushPromises()
    await selectRange(wrapper, '7d')
    const card = wrapper.findAllComponents(AccountMonitorCardStub)[0]
    await card.get('[data-test="edit-cost"]').trigger('click')
    expect(wrapper.get('[data-test="cost-dialog"]').get('[data-test="dialog-account"]').text()).toBe('10')

    list.mockClear()
    await wrapper.get('[data-test="dialog-save-procurement"]').trigger('click')
    await flushPromises()
    expect(updateProcurementCost).toHaveBeenCalledWith(10, 4, 60)
    expect(list).toHaveBeenCalledWith('7d', expect.objectContaining({ signal: expect.any(AbortSignal) }))

    await card.get('[data-test="edit-cost"]').trigger('click')
    list.mockClear()
    await wrapper.get('[data-test="dialog-save-multiplier"]').trigger('click')
    await flushPromises()
    expect(updateAccount).toHaveBeenCalledWith(10, {
      rate_multiplier: 0.08,
      upstream_billing_rate_sync_enabled: false,
    })

    await card.get('[data-test="edit-cost"]').trigger('click')
    list.mockClear()
    await wrapper.get('[data-test="dialog-restore-auto"]').trigger('click')
    await flushPromises()
    expect(updateAccount).toHaveBeenCalledWith(10, {
      upstream_billing_probe_enabled: true,
      upstream_billing_rate_sync_enabled: true,
    })
    expect(runOne).toHaveBeenCalledWith(10)

    await card.get('[data-test="edit-cost"]').trigger('click')
    list.mockClear()
    await wrapper.get('[data-test="dialog-clear"]').trigger('click')
    await flushPromises()
    expect(updateProcurementCost).toHaveBeenCalledWith(10, null, null)
  })

  it('keeps the cost dialog open and exposes the API error when a save fails', async () => {
    updateProcurementCost.mockRejectedValueOnce(new Error('采购成本保存失败'))
    const wrapper = mountView()
    await flushPromises()

    await wrapper.findAllComponents(AccountMonitorCardStub)[0].get('[data-test="edit-cost"]').trigger('click')
    await wrapper.get('[data-test="dialog-save-procurement"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="cost-dialog"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="dialog-error"]').text()).toContain('采购成本保存失败')
  })

  it.each([
    ['dialog-save-procurement', '保存采购成本'],
    ['dialog-save-multiplier', '保存账号倍率'],
    ['dialog-restore-auto', '恢复自动倍率'],
    ['dialog-clear', '清空采购成本'],
  ])('surfaces a current-range reload failure inside the dialog for %s', async (button, operation) => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAllComponents(AccountMonitorCardStub)[0].get('[data-test="edit-cost"]').trigger('click')

    list.mockRejectedValueOnce(new Error('监控数据刷新失败'))
    await wrapper.get(`[data-test="${button}"]`).trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="cost-dialog"]').exists()).toBe(true)
    const expectedError = `${operation}成功，但最新监控卡片加载失败，请重试`
    expect(wrapper.get('[data-test="dialog-error"]').text()).toBe(expectedError)
    expect(wrapper.get('[data-test="range-error"]').text()).toContain('监控数据刷新失败')
    expect(showError).toHaveBeenCalledWith(expectedError)
    expect(showSuccess).not.toHaveBeenCalled()
  })

  it('fetches the compact card account by id and opens the native account information entry', async () => {
    const wrapper = mountView()
    await flushPromises()
    const card = wrapper.findAllComponents(AccountMonitorCardStub)[0]

    await card.get('[data-test="account-info"]').trigger('click')
    await flushPromises()

    expect(getAccountById).toHaveBeenCalledTimes(1)
    expect(getAccountById).toHaveBeenCalledWith(10)
    expect(wrapper.get('[data-test="native-dialog"]').text()).toContain('10')
  })

  it('deduplicates concurrent same-id entry requests and keeps only the latest requested native shell', async () => {
    let resolveAccount!: (account: typeof baseAccount) => void
    getAccountById.mockImplementationOnce(() => new Promise((resolve) => { resolveAccount = resolve }))
    const wrapper = mountView()
    await flushPromises()
    const card = wrapper.findAllComponents(AccountMonitorCardStub)[0]

    const infoClick = card.get('[data-test="account-info"]').trigger('click')
    const editClick = card.get('[data-test="account-edit"]').trigger('click')
    const moreClick = card.get('[data-test="account-more"]').trigger('click')
    expect(getAccountById).toHaveBeenCalledTimes(1)
    resolveAccount({ ...baseAccount, id: 10, type: 'oauth' } as never)
    await Promise.all([infoClick, editClick, moreClick])
    await flushPromises()

    expect(wrapper.find('[data-test="native-dialog"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="account-action-menu"]').exists()).toBe(true)
  })

  it('ignores a stale different-account response instead of replacing the latest native action target', async () => {
    let resolveFirst!: (account: unknown) => void
    let resolveSecond!: (account: unknown) => void
    getAccountById.mockImplementation((id: number) => new Promise((resolve) => {
      if (id === 10) resolveFirst = resolve
      else resolveSecond = resolve
    }))
    const wrapper = mountView()
    await flushPromises()
    const cards = wrapper.findAllComponents(AccountMonitorCardStub)

    const staleDelete = cards[0].get('[data-test="account-delete"]').trigger('click')
    const latestEdit = cards[1].get('[data-test="account-edit"]').trigger('click')
    resolveSecond({ ...baseAccount, id: 11, name: 'Rank two', type: 'oauth' })
    await latestEdit
    await flushPromises()
    resolveFirst({ ...baseAccount, id: 10, name: 'Rank one A', type: 'oauth' })
    await staleDelete
    await flushPromises()

    expect(wrapper.get('[data-test="native-dialog"]').text()).toContain('11')
    expect(wrapper.find('[data-test="confirm-dialog"]').exists()).toBe(false)
  })

  it('does not surface a stale request error or loading state after a newer account entry succeeds', async () => {
    let rejectFirst!: (reason?: unknown) => void
    let resolveSecond!: (account: unknown) => void
    getAccountById.mockImplementation((id: number) => new Promise((resolve, reject) => {
      if (id === 10) rejectFirst = reject
      else resolveSecond = resolve
    }))
    const wrapper = mountView()
    await flushPromises()
    const cards = wrapper.findAllComponents(AccountMonitorCardStub)

    const staleInfo = cards[0].get('[data-test="account-info"]').trigger('click')
    const latestEdit = cards[1].get('[data-test="account-edit"]').trigger('click')
    resolveSecond({ ...baseAccount, id: 11, name: 'Rank two', type: 'oauth' })
    await latestEdit
    await flushPromises()
    rejectFirst(new Error('stale account 10 failed'))
    await staleInfo
    await flushPromises()

    expect(wrapper.get('[data-test="native-dialog"]').text()).toContain('11')
    expect(wrapper.find('[data-test="account-native-loading"]').exists()).toBe(false)
    expect(showError).not.toHaveBeenCalledWith('stale account 10 failed')
  })

  it('anchors the more menu to the clicked card trigger and preserves native privacy result semantics', async () => {
    const wrapper = mountView()
    await flushPromises()
    const trigger = wrapper.findAllComponents(AccountMonitorCardStub)[0].get<HTMLElement>('[data-test="account-more"]')
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue({ top: 100, right: 700, bottom: 130 } as DOMRect)

    await trigger.trigger('click')
    await flushPromises()
    const menu = wrapper.getComponent(AccountActionMenuStub)
    expect(menu.props('position')).toEqual(expect.objectContaining({ left: expect.any(Number), top: expect.any(Number) }))
    expect((menu.props('position') as { left: number }).left).toBeLessThan(window.innerWidth - 224)

    setPrivacy.mockResolvedValueOnce({ ...baseAccount, id: 10, type: 'oauth', extra: { privacy_mode: 'training_set_cf_blocked' } })
    await wrapper.get('[data-test="menu-set-privacy"]').trigger('click')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('被 Cloudflare 拦截，训练可能仍开启')
    expect(showSuccess).not.toHaveBeenCalledWith('被 Cloudflare 拦截，训练可能仍开启')
  })

  it('uses native delete confirmation and reloads the compact monitor projection after confirmation', async () => {
    const wrapper = mountView()
    await flushPromises()
    const card = wrapper.findAllComponents(AccountMonitorCardStub)[0]
    await card.get('[data-test="account-delete"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="confirm-dialog"]').exists()).toBe(true)
    list.mockClear()
    await wrapper.get('[data-test="confirm"]').trigger('click')
    await flushPromises()
    expect(deleteAccount).toHaveBeenCalledWith(10)
    expect(list).toHaveBeenCalledWith('24h', expect.objectContaining({ signal: expect.any(AbortSignal) }))
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

  it('loads, saves, and asynchronously enqueues per-account detector models', async () => {
    const wrapper = mountView()
    await flushPromises()
    const card = wrapper.findAllComponents(AccountMonitorCardStub)[0]

    await card.get('[data-test="model-open"]').trigger('click')
    await flushPromises()
    expect(getModelDetectionModels).toHaveBeenCalledWith(10)

    await card.get('[data-test="model-save"]').trigger('click')
    await flushPromises()
    expect(saveModelDetectionModels).toHaveBeenCalledWith(10, { connection_probe_model: 'gpt-5.6-sol', model_detection_model: 'gpt-5.6-sol' })

    await card.get('[data-test="model-detect"]').trigger('click')
    await flushPromises()
    expect(enqueueModelDetection).toHaveBeenCalledWith(10)
  })
})
