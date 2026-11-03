import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'

import type { ApiKey, ApiKeyConcurrencySnapshot } from '@/types'
import { keysAPI } from '@/api'
import KeysView from '../KeysView.vue'
import LineSelect from '@/components/keys/LineSelect.vue'
import CreateLineKeyDialog from '@/features/ai-tools/CreateLineKeyDialog.vue'

enableAutoUnmount(afterEach)

const {
  listKeys,
  createKey,
  updateKey,
  getConcurrency,
  getPublicSettings,
  getDashboardApiKeysUsage,
  getAvailableGroups,
  getUserGroupRates,
  getLineSelectionMetrics,
  getHybridPerformanceSnapshot,
  showError,
  showSuccess,
  copyToClipboard,
  isCurrentStep,
  nextStep,
} = vi.hoisted(() => ({
  listKeys: vi.fn(),
  createKey: vi.fn(),
  updateKey: vi.fn(),
  getConcurrency: vi.fn(),
  getPublicSettings: vi.fn(),
  getDashboardApiKeysUsage: vi.fn(),
  getAvailableGroups: vi.fn(),
  getUserGroupRates: vi.fn(),
  getLineSelectionMetrics: vi.fn(),
  getHybridPerformanceSnapshot: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  copyToClipboard: vi.fn(),
  isCurrentStep: vi.fn(),
  nextStep: vi.fn(),
}))

const messages: Record<string, string> = {
  'common.actions': 'Actions',
  'common.name': 'Name',
  'common.refresh': 'Refresh',
  'common.status': 'Status',
  'keys.apiKey': 'API Key',
  'keys.allGroups': 'All Groups',
  'keys.allStatus': 'All Status',
  'keys.columnSettings': 'Column Settings',
  'keys.createKey': 'Create API Key',
  'keys.created': 'Created',
  'keys.expiresAt': 'Expires',
  'keys.group': 'Group',
  'keys.id': 'ID',
  'keys.currentConcurrency': 'Current Concurrency',
  'keys.concurrencyAndWaiting': 'Concurrency / Waiting',
  'keys.concurrencyCount': 'Concurrency',
  'keys.waitingCount': 'Waiting',
  'keys.queueFull': 'Full',
  'keys.queueOff': 'Queuing globally disabled',
  'keys.queuePolicy': 'This key allows {max} waiting requests for up to {seconds} seconds.',
  'keys.queuePolicyOff': 'Queuing globally disabled; new requests are rejected at the key limit.',
  'keys.queueNotApplicable': 'No key-level queue.',
  'keys.queuePolicyLoading': 'Loading shared queue settings…',
  'keys.queuePolicyUnavailable': 'Shared queue settings unavailable.',
  'keys.concurrencyLoading': 'Loading',
  'keys.concurrencyUnavailable': 'Statistics unavailable',
  'keys.concurrencyStale': 'Not updated',
  'keys.noAdditionalConcurrencyLimit': 'No additional limit',
  'keys.concurrencyLimitInvalid': 'Enter a nonnegative whole number for the concurrency limit.',
  'keys.lastUsedAt': 'Last Used',
  'keys.lastUsedIP': 'Last Used IP',
  'keys.rateLimitColumn': 'Rate Limit',
  'keys.searchPlaceholder': 'Search name or key...',
  'keys.status.active': 'Active',
  'keys.status.expired': 'Expired',
  'keys.status.inactive': 'Inactive',
  'keys.status.quota_exhausted': 'Quota exhausted',
  'keys.usage': 'Usage',
  'keys.usageUnavailable': 'Usage unavailable',
  'keys.failedToLoad': 'Failed to load API keys',
  'keys.endpoints.title': 'API endpoint',
  'keys.endpoints.default': 'Default',
  'keys.endpoints.copyAddress': 'Copy address',
  'keys.endpoints.instructions': 'Connection instructions',
  'keys.endpoints.unavailable': 'Endpoint unavailable',
}

vi.mock('@/api', () => ({
  keysAPI: {
    list: listKeys,
    create: createKey,
    update: updateKey,
    getConcurrency,
    delete: vi.fn(),
    toggleStatus: vi.fn(),
  },
  authAPI: {
    getPublicSettings,
  },
  usageAPI: {
    getDashboardApiKeysUsage,
  },
  userGroupsAPI: {
    getAvailable: getAvailableGroups,
    getUserGroupRates,
  },
}))
vi.mock('@/api/keys', () => ({ keysAPI: { create: createKey } }))
vi.mock('@/features/monitor-v4/lineSelection', () => ({ getLineSelectionMetrics }))
vi.mock('@/features/monitor-v4/api', () => ({ getHybridPerformanceSnapshot }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
  }),
}))

vi.mock('@/stores/onboarding', () => ({
  useOnboardingStore: () => ({
    isCurrentStep,
    nextStep,
  }),
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params: Record<string, unknown> = {}) =>
        (messages[key] ?? key).replace(/\{(\w+)\}/g, (_, name) => String(params[name] ?? name)),
    }),
  }
})

const createApiKey = (): ApiKey => ({
  id: 1,
  user_id: 1,
  key: 'sk-test-key',
  name: 'test-key',
  group_id: null,
  status: 'active',
  ip_whitelist: [],
  ip_blacklist: [],
  last_used_at: null,
  last_used_ip: null,
  quota: 0,
  quota_used: 0,
  expires_at: null,
  created_at: '2026-06-27T00:00:00Z',
  updated_at: '2026-06-27T00:00:00Z',
  current_concurrency: 3,
  concurrency_limit: 0,
  rate_limit_5h: 0,
  rate_limit_1d: 0,
  rate_limit_7d: 0,
  usage_5h: 0,
  usage_1d: 0,
  usage_7d: 0,
  window_5h_start: null,
  window_1d_start: null,
  window_7d_start: null,
  reset_5h_at: null,
  reset_1d_at: null,
  reset_7d_at: null,
})

const AppLayoutStub = {
  template: '<div><slot /></div>',
}

const TablePageLayoutStub = {
  props: ['continuous'],
  template: `
    <div :data-continuous="continuous">
      <slot name="heading" /><slot name="actions" />
      <slot name="filters" />
      <slot name="endpoint" />
      <slot name="table" />
      <slot name="pagination" />
    </div>
  `,
}

const DataTableStub = {
  name: 'DataTable',
  props: { columns: Array, data: Array, selectedKeys: Array, selectable: Boolean, stickyFirstColumn: Boolean },
  emits: ['sort', 'update:selectedKeys'],
  template: `
    <div>
      <div data-test="columns">{{ columns.map((col) => col.key).join(',') }}</div>
      <div data-test="columns-meta">{{ JSON.stringify(columns.map((col) => ({ key: col.key, sortable: !!col.sortable }))) }}</div>
      <button data-test="sort-current-concurrency" @click="$emit('sort', 'current_concurrency', 'asc')">
        Sort Current Concurrency
      </button>
      <div v-for="row in data" :key="row.id">
        <div
          v-if="columns.some((col) => col.key === 'id')"
          data-test="key-id"
        >
          <slot name="cell-id" :value="row.id" :row="row" />
        </div>
        <slot name="cell-name" :value="row.name" :row="row" />
        <slot name="cell-actions" :row="row" />
        <slot name="cell-group" :row="row" />
        <div data-test="current-concurrency">
          <slot name="cell-current_concurrency" :value="row.current_concurrency" :row="row" />
        </div>
        <div data-test="key-usage"><slot name="cell-usage" :row="row" /></div>
        <div
          v-if="columns.some((col) => col.key === 'last_used_ip')"
          data-test="last-used-ip"
        >
          <slot name="cell-last_used_ip" :value="row.last_used_ip" :row="row" />
        </div>
      </div>
      <slot name="empty" />
    </div>
  `,
}

const SelectStub = {
  name: 'Select',
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"></select>',
}

const SearchInputStub = {
  name: 'SearchInput',
  props: ['modelValue'],
  emits: ['update:modelValue', 'search'],
  template: '<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
}

const PaginationStub = {
  name: 'Pagination',
  props: ['page', 'total', 'pageSize'],
  emits: ['update:page', 'update:pageSize'],
  template: `
    <div>
      <button data-test="page-size-50" @click="$emit('update:pageSize', 50)">50</button>
      <button data-test="page-2" @click="$emit('update:page', 2)">2</button>
    </div>
  `,
}

const IconStub = {
  props: ['name'],
  template: '<span data-test="icon">{{ name }}</span>',
}

const mountView = async (recommendedGroupIds?: ReadonlyMap<string, number>) => {
  const wrapper = mount(KeysView, {
    props: { recommendedGroupIds },
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        TablePageLayout: TablePageLayoutStub,
        DataTable: DataTableStub,
        Pagination: PaginationStub,
        BaseDialog: {
          props: ['show', 'title'],
          emits: ['close'],
          template: '<div v-if="show" role="dialog"><button data-test="close-dialog" @click="$emit(\'close\')">Close</button><slot /><slot name="footer" /></div>',
        },
        ConfirmDialog: true,
        EmptyState: true,
        Select: SelectStub,
        SearchInput: SearchInputStub,
        Icon: IconStub,
        UseKeyModal: true,
        BulkEditKeysModal: true,
        EndpointPopover: false,
        GroupBadge: true,
        GroupOptionItem: true,
        Teleport: { template: '<div><slot /></div>' },
      },
    },
  })
  await flushPromises()
  await nextTick()
  return wrapper
}

const visibleColumnKeys = (wrapper: VueWrapper) =>
  wrapper.get('[data-test="columns"]').text().split(',').filter(Boolean)

const visibleColumnMeta = (wrapper: VueWrapper): Array<{ key: string; sortable: boolean }> =>
  JSON.parse(wrapper.get('[data-test="columns-meta"]').text())

const getButtonByText = (wrapper: VueWrapper, text: string) => {
  const button = wrapper.findAll('button').find((item) => item.text().includes(text))
  if (!button) {
    throw new Error(`Button not found: ${text}`)
  }
  return button
}

const snapshot = (max = 7, waiting = 2): ApiKeyConcurrencySnapshot => ({
  queue_policy: { max_waiting: max, timeout_seconds: 12 },
  items: [{ id: 1, current_concurrency: 3, current_waiting: waiting }],
})

const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

const setKeys = (...keys: ApiKey[]) => listKeys.mockResolvedValue({
  items: keys, total: keys.length, page: 1, page_size: 20, pages: 2,
})

describe('user KeysView column settings', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  afterEach(() => { vi.useRealTimers() })
  it('expires inline selector health as time passes', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-05T00:00:00Z'))
    getAvailableGroups.mockResolvedValue([{ id: 12, name: 'GPT', status: 'active', platform: 'openai', rate_multiplier: 1 }])
    getLineSelectionMetrics.mockResolvedValue({ generated_at: new Date().toISOString(), groups: [{ id: 12, sla_request_count: 100, sla_success_count: 95 }] })
    const wrapper = await mountView()
    await wrapper.get('.keys-line-trigger').trigger('click');await flushPromises()
    expect(wrapper.get('.keys-line-popup').text()).toContain('正常运行')
    vi.advanceTimersByTime(8 * 60 * 1000);await nextTick()
    expect(wrapper.get('.keys-line-popup').text()).not.toContain('正常运行')
    expect(wrapper.get('.keys-line-popup').text()).toContain('统计已过期')
    wrapper.unmount()
  })
  it('shows inline statistics failure only in route status and reloads on re-entry', async () => {
    getAvailableGroups.mockResolvedValue([{ id: 12, name: 'GPT', status: 'active', platform: 'openai', rate_multiplier: 1 }])
    getLineSelectionMetrics.mockRejectedValueOnce(new Error('offline'))
    const wrapper = await mountView()
    await wrapper.get('.keys-line-trigger').trigger('click');await flushPromises()
    const popup = wrapper.get('.keys-line-popup')
    expect(popup.find('[role="alert"]').exists()).toBe(false)
    expect(popup.find('.keys-line-metrics-error').exists()).toBe(false)
    expect(popup.text()).toContain('统计读取失败')
    expect(popup.text()).toContain('可用性 —')
    expect(popup.text()).toContain('首字 —')
    expect(popup.text()).not.toContain('请重试')
    await wrapper.get('.keys-line-trigger').trigger('click')
    await wrapper.get('.keys-line-trigger').trigger('click');await flushPromises()
    expect(wrapper.get('.keys-line-popup').text()).not.toContain('统计读取失败')
    expect(getLineSelectionMetrics).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
  it('keeps the newest line statistics when an earlier request fails later', async () => {
    const older = deferred<unknown>()
    getLineSelectionMetrics.mockImplementationOnce(() => older.promise)
      .mockResolvedValueOnce({ generated_at: new Date().toISOString(), groups: [] })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'Create API Key').trigger('click');await flushPromises()
    const selector = wrapper.getComponent(LineSelect)
    await wrapper.get('.keys-line-trigger').trigger('click');await flushPromises()
    expect(selector.props('metricsError')).toBe(false)
    older.reject(new Error('late old failure'));await flushPromises()
    expect(selector.props('metricsError')).toBe(false)
    wrapper.unmount()
  })
  it('ignores an earlier successful response and aborts statistics on unmount', async () => {
    const older = deferred<unknown>()
    const latest = { id: 12, sla_request_count: 100, sla_success_count: 95 }
    getLineSelectionMetrics.mockImplementationOnce(() => older.promise)
      .mockResolvedValueOnce({ generated_at: new Date().toISOString(), groups: [latest] })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'Create API Key').trigger('click');await flushPromises()
    const selector = wrapper.getComponent(LineSelect)
    const previousSignal = getLineSelectionMetrics.mock.calls[0]![0] as AbortSignal
    await wrapper.get('.keys-line-trigger').trigger('click');await flushPromises()
    expect(previousSignal.aborted).toBe(true)
    older.resolve({ generated_at: new Date().toISOString(), groups: [] });await flushPromises()
    expect(selector.props('metrics').get(12)).toEqual(latest)
    const currentSignal = getLineSelectionMetrics.mock.calls[1]![0] as AbortSignal
    wrapper.unmount()
    expect(currentSignal.aborted).toBe(true)
  })
  it('shows create statistics failure only inside the dropdown and reloads when reopened', async () => {
    getAvailableGroups.mockResolvedValue([{ id: 12, name: 'GPT', status: 'active', platform: 'openai', rate_multiplier: 1 }])
    getLineSelectionMetrics.mockRejectedValueOnce(new Error('offline'))
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'Create API Key').trigger('click');await flushPromises()
    const selector = wrapper.getComponent(LineSelect)
    expect(selector.props('metricsError')).toBe(true)
    expect(selector.find('[role="alert"]').exists()).toBe(false)
    expect(selector.props('metrics').size).toBe(0)
    expect(wrapper.text()).not.toContain('请重试')
    wrapper.getComponent(CreateLineKeyDialog).vm.$emit('close');await flushPromises()
    await getButtonByText(wrapper, 'Create API Key').trigger('click');await flushPromises()
    expect(wrapper.getComponent(LineSelect).props('metricsError')).toBe(false)
    expect(getLineSelectionMetrics).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
  it('shows each tool best route in ordinary creation without choosing a cross-tool winner', async () => {
    getAvailableGroups.mockResolvedValue([
      { id: 1, name: 'Codex 推荐', status: 'active', platform: 'openai', rate_multiplier: 2 },
      { id: 2, name: 'Gemini 推荐', status: 'active', platform: 'gemini', rate_multiplier: 1 },
    ])
    const recommended = new Map([['codex', 1], ['gemini', 2]])
    const wrapper = await mountView(recommended)
    await getButtonByText(wrapper, 'Create API Key').trigger('click');await flushPromises()
    const selector = wrapper.getComponent(LineSelect)
    expect(selector.props('recommendedGroupIds')).toEqual(recommended)
    expect(selector.props('recommendedGroupIds').size).toBe(2)
    wrapper.unmount()
  })
  it('loads the same 24-hour ranking for independent ordinary creation and expires its badges', async () => {
    getAvailableGroups.mockResolvedValue([
      { id: 1, name: 'Codex 推荐', status: 'active', platform: 'openai', rate_multiplier: 2 },
      { id: 2, name: 'Gemini 推荐', status: 'active', platform: 'gemini', rate_multiplier: 1 },
    ])
    getHybridPerformanceSnapshot.mockResolvedValue({ generated_at: new Date().toISOString(), groups: [
      { id: 1, sla_request_count: 100, sla_success_count: 99, cache_hit_rate: 0.5, ttft_p50_ms: 2000 },
      { id: 2, sla_request_count: 100, sla_success_count: 95, cache_hit_rate: 0.6, ttft_p50_ms: 1000 },
    ] })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'Create API Key').trigger('click');await flushPromises()
    expect(getHybridPerformanceSnapshot).toHaveBeenCalledWith('24h', expect.any(AbortSignal))
    const selector = wrapper.getComponent(LineSelect)
    expect(selector.props('recommendedGroupIds').size).toBe(2)
    vi.advanceTimersByTime(8 * 60 * 1000);await nextTick()
    expect(selector.props('recommendedGroupIds').size).toBe(0)
    wrapper.unmount()
  })
  it('does not fabricate a recommendation when the independent ranking request fails', async () => {
    getAvailableGroups.mockResolvedValue([{ id: 1, name: 'Codex', status: 'active', platform: 'openai', rate_multiplier: 1 }])
    getHybridPerformanceSnapshot.mockRejectedValue(new Error('ranking offline'))
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'Create API Key').trigger('click');await flushPromises()
    const selector = wrapper.getComponent(LineSelect)
    expect(selector.props('recommendedGroupIds').size).toBe(0)
    wrapper.unmount()
  })

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date', 'setTimeout', 'clearTimeout', 'setInterval', 'clearInterval'] })
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.clear()

    listKeys.mockReset()
    createKey.mockReset().mockResolvedValue(createApiKey())
    updateKey.mockReset().mockResolvedValue(createApiKey())
    getConcurrency.mockReset().mockResolvedValue({
      queue_policy: { max_waiting: 7, timeout_seconds: 12 },
      items: [{ id: 1, current_concurrency: 3, current_waiting: 2 }],
    })
    getPublicSettings.mockReset()
    getDashboardApiKeysUsage.mockReset()
    getAvailableGroups.mockReset()
    getUserGroupRates.mockReset()
    getLineSelectionMetrics.mockReset()
    getHybridPerformanceSnapshot.mockReset().mockResolvedValue({ generated_at: new Date().toISOString(), groups: [] })
    showError.mockReset()
    showSuccess.mockReset()
    copyToClipboard.mockReset()
    isCurrentStep.mockReset()
    nextStep.mockReset()

    listKeys.mockResolvedValue({
      items: [createApiKey()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    getPublicSettings.mockResolvedValue({})
    getDashboardApiKeysUsage.mockResolvedValue({ stats: {} })
    getAvailableGroups.mockResolvedValue([])
    getUserGroupRates.mockResolvedValue({})
    getLineSelectionMetrics.mockResolvedValue({ groups: [] })
    isCurrentStep.mockReturnValue(false)
  })

  it.each([
    { initialStatus: 'quota_exhausted', status: 'active', formStatus: 'active' },
    { initialStatus: 'inactive', status: 'inactive', formStatus: 'inactive' },
    { initialStatus: 'active', status: 'active', formStatus: 'inactive' },
  ] as const)('syncs quota reset from $initialStatus to $status with form status $formStatus', async ({ initialStatus, status, formStatus }) => {
    const key: ApiKey = {
      ...createApiKey(), group_id: 1, quota: 10, quota_used: 10,
      status: initialStatus,
    }
    listKeys.mockResolvedValueOnce({ items: [key], total: 1, page: 1, page_size: 20, pages: 1 })
    updateKey.mockResolvedValue({ ...key, status, quota_used: 0 })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    await wrapper.get('[data-tour="key-form-name"]').setValue('Unsaved name')
    const statusSelect = wrapper.findAllComponents({ name: 'Select' })
      .find((select) => select.props('options').length === 2 &&
        select.props('options')[0].value === 'active')!
    statusSelect.vm.$emit('update:modelValue', 'inactive')
    await wrapper.get('button[title="keys.resetQuotaUsed"]').trigger('click')
    const confirmation = wrapper.findAllComponents({ name: 'ConfirmDialog' })
      .find((dialog) => dialog.props('title') === 'keys.resetQuotaTitle')!
    confirmation.vm.$emit('confirm')
    await flushPromises()

    expect(updateKey).toHaveBeenNthCalledWith(1, key.id, { reset_quota: true })
    expect(wrapper.findComponent({ name: 'DataTable' }).props('data')[0])
      .toMatchObject({ status, quota_used: 0 })
    expect(statusSelect.props('modelValue')).toBe(formStatus)
    expect((wrapper.get('[data-tour="key-form-name"]').element as HTMLInputElement).value)
      .toBe('Unsaved name')

    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(updateKey).toHaveBeenNthCalledWith(2, key.id, expect.objectContaining({ name: 'Unsaved name', status: formStatus }))
    wrapper.unmount()
  })

  it('uses a bordered line trigger with an expanded state and no redundant selection text', async () => {
    const key = createApiKey()
    key.group_id = 12
    key.group = { id: 12, name: 'GPT-Pro', platform: 'openai', status: 'active', rate_multiplier: 0.3 } as ApiKey['group']
    listKeys.mockResolvedValue({ items: [key], total: 1, page: 1, page_size: 20, pages: 1 })
    const wrapper = await mountView()
    const trigger = wrapper.get('.keys-line-trigger')
    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(trigger.text()).not.toContain('keys.selectGroup')
    expect(wrapper.getComponent({ name: 'GroupBadge' }).props('rateMultiplier')).toBe(0.3)
    await trigger.trigger('click')
    expect(wrapper.get('.keys-line-trigger').attributes('aria-expanded')).toBe('true')
    await wrapper.get('.keys-line-popup input').trigger('keydown.esc')
    expect(wrapper.get('.keys-line-trigger').attributes('aria-expanded')).toBe('false')
  })

  it('keeps the line menu below a low trigger and limits it to available space', async () => {
    const wrapper = await mountView()
    const trigger = wrapper.get('.keys-line-trigger')
    const bottom = window.innerHeight - 130
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue({
      x: 200, y: bottom - 36, left: 200, top: bottom - 36,
      right: 400, bottom, width: 200, height: 36, toJSON: () => ({}),
    })
    await trigger.trigger('click')
    const popup = wrapper.get('.keys-line-popup').element as HTMLElement
    expect(popup.style.top).toBe(String(bottom + 4) + 'px')
    expect(popup.style.bottom).toBe('')
    expect(popup.style.maxHeight).toBe('118px')
  })

  it('opens the inline line popup with real metrics and unknown availability when monitoring is missing', async () => {
    getAvailableGroups.mockResolvedValue([
      { id: 12, name: 'GPT Plus', status: 'active', platform: 'openai', rate_multiplier: 1.2 },
      { id: 13, name: 'Other line', status: 'active', platform: 'openai', rate_multiplier: 1 }
    ])
    getUserGroupRates.mockResolvedValue({ 12: 0.8 })
    getLineSelectionMetrics.mockResolvedValue({ generated_at: new Date().toISOString(), groups: [{ id: 12, sla_request_count: 100, sla_success_count: 75, success_rate: 75, ttft_p50_ms: 2160, request_count: 10 }] })
    const wrapper = await mountView()
    await wrapper.get('button[title$=" · keys.clickToChangeGroup"]').trigger('click')
    await flushPromises()
    const popup = wrapper.get('.keys-line-popup')
    expect(popup.text()).toContain('0.8x')
    expect(popup.text()).toContain('75%')
    expect(popup.text()).toContain('波动')
    expect(popup.text()).toContain('暂无数据')
    expect(popup.findComponent({ name: 'GroupOptionItem' }).exists()).toBe(false)
    await popup.get('input').setValue('Other')
    await flushPromises()
    await nextTick()
    expect(wrapper.findAll('.keys-line-popup-option')).toHaveLength(1)
    await wrapper.get('.keys-line-popup input').trigger('keydown.esc')
    expect(wrapper.find('.keys-line-popup').exists()).toBe(false)
  })

  it('uses the shared line selector in the full key management form', async () => {
    getAvailableGroups.mockResolvedValue([{ id: 12, name: 'GPT Plus', status: 'active', platform: 'openai', rate_multiplier: 1.2 }])
    getUserGroupRates.mockResolvedValue({ 12: 0.8 })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'Create API Key').trigger('click')
    const selector = wrapper.getComponent(LineSelect)
    expect(selector.props('groups')).toHaveLength(1)
    expect(selector.props('rates')).toEqual({ 12: 0.8 })
  })

  it('keeps actions, filters, endpoints, list, and pagination in one work surface', async () => {
    getPublicSettings.mockResolvedValue({ api_base_url: 'https://api.example.test/v1' })
    const wrapper = await mountView()
    expect(wrapper.get('[data-continuous]').exists()).toBe(true)
    const content = wrapper.get('[data-continuous]').html()
    expect(wrapper.get('[data-test="keys-actions"]').classes()).toContain('justify-between')
    expect(content.indexOf('Search name or key...')).toBeLessThan(content.indexOf('data-tour="keys-create-btn"'))
    expect(content.indexOf('Search name or key...')).toBeLessThan(content.indexOf('data-test="keys-list-heading"'))
    expect(wrapper.find('button').exists()).toBe(true)
    expect(content.indexOf('data-test="keys-list-heading"')).toBeLessThan(content.indexOf('data-test="columns"'))
    expect(content.indexOf('data-test="columns"')).toBeLessThan(content.indexOf('data-test="keys-pagination"'))
    expect(wrapper.get('.merged-endpoints').text()).toContain('https://api.example.test/v1')
    expect(wrapper.getComponent({ name: 'DataTable' }).props('stickyFirstColumn')).toBe(false)
  })

  it('uses the current origin when no public endpoint is configured and copies only that address', async () => {
    const wrapper = await mountView()
    expect(wrapper.get('.merged-endpoints').text()).toContain(`${window.location.origin}/v1`)
    await wrapper.get('.merged-endpoints .group\\/endpoint button').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith(`${window.location.origin}/v1`, 'keys.endpoints.copied')
    expect(copyToClipboard).not.toHaveBeenCalledWith('sk-test-key', expect.anything())
  })

  it('uses the same complete key form as the AI tool entry when creating a key', async () => {
    getAvailableGroups.mockResolvedValue([{ id: 12, name: 'GPT Plus', status: 'active', platform: 'openai', rate_multiplier: 1.2 }])
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'Create API Key').trigger('click')
    expect(wrapper.getComponent(CreateLineKeyDialog).props('show')).toBe(true)
    await wrapper.get('[name="name"]').setValue('new-key')
    wrapper.getComponent(LineSelect).vm.$emit('update:modelValue', 12)
    await nextTick()
    createKey.mockResolvedValue({ id: 7, group_id: 12 })
    await wrapper.get('#xq-create-line-key').trigger('submit')
    await flushPromises()
    expect(createKey).toHaveBeenCalledWith('new-key', 12, undefined, [], [], 0, undefined, {
      rate_limit_5h: 0, rate_limit_1d: 0, rate_limit_7d: 0,
    })
  })

  it('exposes result count, page size, and navigation in the narrow key footer', async () => {
    const wrapper = await mountView()
    const footer = wrapper.get('[data-test="keys-pagination"]')
    expect(footer.text()).toContain('1')
    expect(footer.findAll('button')).toHaveLength(2)
    expect(footer.text()).toContain('5')
    expect(footer.findComponent({ name: 'Select' }).exists()).toBe(false)
  })

  it('uses the default API key columns with low-frequency columns hidden', async () => {
    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'key',
      'group',
      'current_concurrency',
      'usage',
      'expires_at',
      'actions',
    ])
    expect(visibleColumnKeys(wrapper)).not.toContain('rate_limit')
    expect(visibleColumnKeys(wrapper)).not.toContain('last_used_at')
    expect(visibleColumnKeys(wrapper)).not.toContain('last_used_ip')
    expect(visibleColumnKeys(wrapper)).not.toContain('id')
    expect(visibleColumnKeys(wrapper)).not.toContain('status')
    expect(visibleColumnKeys(wrapper)).not.toContain('created_at')
  })

  it('shows a retryable error instead of the empty state after an initial list failure', async () => {
    listKeys.mockRejectedValueOnce(new Error('offline'))
    const wrapper = await mountView()

    expect(wrapper.get('[role="alert"]').text()).toContain('Failed to load API keys')
    expect(wrapper.text()).not.toContain('keys.noKeysYet')

    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(listKeys).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
  })

  it('keeps existing keys but marks usage unavailable after a usage refresh failure', async () => {
    getDashboardApiKeysUsage.mockResolvedValueOnce({ stats: { 1: { today_actual_cost: 1.25, total_actual_cost: 2.5 } } })
    const wrapper = await mountView()
    getDashboardApiKeysUsage.mockRejectedValueOnce(new Error('offline'))

    await wrapper.get('button[title="Refresh"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('test-key')
    expect(wrapper.text()).toContain('$1.25')
    expect(wrapper.find('[data-test="key-usage-error"]').exists()).toBe(true)
  })

  it('shows an unavailable quota amount rather than a fabricated zero when used quota is missing', async () => {
    listKeys.mockResolvedValue({ items: [{ ...createApiKey(), quota: 20.1234, quota_used: null }], total: 1, page: 1, page_size: 20, pages: 1 })
    const wrapper = await mountView()
    expect(wrapper.get('.keys-usage-summary').text()).not.toContain('详情')
    expect(wrapper.get('.keys-usage-summary').element.children).toHaveLength(2)
    await wrapper.get('.keys-usage-details[aria-haspopup="dialog"]').trigger('click')
    expect(wrapper.get('[data-test="key-usage-details"]').text()).toContain('— / $20.12')
    expect(wrapper.get('[data-test="key-usage-details"]').text()).not.toContain('$0.00 / $20.12')
  })

  it('opens bulk editing with only selected visible keys', async () => {
    const wrapper = await mountView()
    const table = wrapper.findComponent({ name: 'DataTable' })
    expect(table.props('selectable')).toBe(true)
    table.vm.$emit('update:selectedKeys', [1])
    await nextTick()
    await wrapper.get('.keys-bulk-toolbar .btn-primary').trigger('click')
    const modal = wrapper.findComponent({ name: 'BulkKeyEditDialog' })
    expect(modal.props('show')).toBe(true)
    expect(modal.props('ids')).toEqual([1])
    wrapper.unmount()
  })

  it.each(['filter', 'page size', 'sort'])('clears selection on %s changes', async (change) => {
    const wrapper = await mountView()
    const table = wrapper.findComponent({ name: 'DataTable' })
    table.vm.$emit('update:selectedKeys', [1])
    await nextTick()
    if (change === 'filter') {
      wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('search')
    } else if (change === 'page size') {
      await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('search')
    } else {
      table.vm.$emit('sort', 'created_at', 'asc')
    }
    await flushPromises()
    expect(table.props('selectedKeys')).toEqual([])
    expect(wrapper.find('.keys-bulk-toolbar .btn-primary').exists()).toBe(false)
    wrapper.unmount()
  })

  it('removes successful keys from the selection and refreshes the table', async () => {
    listKeys.mockResolvedValue({
      items: [createApiKey(), { ...createApiKey(), id: 2, name: 'Second' }],
      total: 2, pages: 1
    })
    const wrapper = await mountView()
    const table = wrapper.findComponent({ name: 'DataTable' })
    table.vm.$emit('update:selectedKeys', [1, 2])
    await nextTick()
    await wrapper.get('.keys-bulk-toolbar .btn-primary').trigger('click')
    wrapper.findComponent({ name: 'BulkKeyEditDialog' }).vm.$emit('finished', [2])
    await flushPromises()
    expect(listKeys).toHaveBeenCalledTimes(2)
    expect(table.props('selectedKeys')).toEqual([2])
    wrapper.unmount()
  })

  it('drops keys that are no longer visible after a refresh', async () => {
    const wrapper = await mountView()
    const table = wrapper.findComponent({ name: 'DataTable' })
    table.vm.$emit('update:selectedKeys', [1])
    await nextTick()
    listKeys.mockResolvedValue({ items: [], total: 0, pages: 0 })
    await wrapper.get('button[title="Refresh"]').trigger('click')
    await flushPromises()
    expect(table.props('selectedKeys')).toEqual([])
    wrapper.unmount()
  })

  it('shows a hidden column when toggled and persists the preference', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Rate Limit').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('rate_limit')
    expect(localStorage.getItem('api-key-hidden-columns')).toBe(
      JSON.stringify(['id', 'last_used_at', 'last_used_ip', 'status', 'created_at'])
    )
    expect(localStorage.getItem('api-key-column-settings-version')).toBe('4')
  })

  it('shows the API key ID column when toggled', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'ID').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('id')
    expect(wrapper.get('[data-test="key-id"]').text()).toBe('#1')
    expect(visibleColumnMeta(wrapper).find((column) => column.key === 'id')?.sortable).toBe(true)
  })

  it('shows the last used IP column when toggled', async () => {
    listKeys.mockResolvedValueOnce({
      items: [{ ...createApiKey(), last_used_ip: '203.0.113.10' }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await getButtonByText(wrapper, 'Last Used IP').trigger('click')
    await nextTick()

    expect(visibleColumnKeys(wrapper)).toContain('last_used_ip')
    expect(wrapper.get('[data-test="last-used-ip"]').text()).toBe('203.0.113.10')
  })

  it('restores column preferences from localStorage on mount', async () => {
    localStorage.setItem('api-key-hidden-columns', JSON.stringify(['group', 'created_at']))
    localStorage.setItem('api-key-column-settings-version', '1')

    const wrapper = await mountView()

    expect(visibleColumnKeys(wrapper)).toEqual([
      'name',
      'key',
      'current_concurrency',
      'usage',
      'rate_limit',
      'expires_at',
      'status',
      'last_used_at',
      'actions',
    ])
    expect(localStorage.getItem('api-key-hidden-columns')).toBe(
      JSON.stringify(['group', 'created_at', 'last_used_ip', 'id'])
    )
    expect(localStorage.getItem('api-key-column-settings-version')).toBe('4')
  })

  it('does not include always-visible columns in the toggleable menu', async () => {
    const wrapper = await mountView()

    await wrapper.get('button[title="Column Settings"]').trigger('click')
    await nextTick()

    const columnMenuText = wrapper.text()
    expect(columnMenuText).toContain('API Key')
    expect(columnMenuText).toContain('ID')
    expect(columnMenuText).toContain('Concurrency / Waiting')
    expect(columnMenuText).toContain('Rate Limit')
    expect(columnMenuText).toContain('Last Used IP')
    expect(columnMenuText).not.toContain('Name')
    expect(columnMenuText).not.toContain('Actions')
  })

  it.each([0, 3])('keeps the concurrency badge at %s for an unlimited key', async (current) => {
    getConcurrency.mockResolvedValue({ ...snapshot(), items: [
      { id: 1, current_concurrency: current, current_waiting: 0 },
    ] })
    const wrapper = await mountView()
    const cell = wrapper.get('[data-test="current-concurrency"]')
    const badge = cell.get('[title="Concurrency"]')
    expect(badge.find('svg').exists()).toBe(true)
    expect(badge.classes()).toContain(current === 0 ? 'bg-gray-100' : 'bg-emerald-50')
    expect(cell.text()).toBe(`Concurrency ${current}`)
    expect(cell.text()).not.toMatch(/\/|No additional limit|Waiting/)
  })

  it('shows current / max for a key with an additional concurrency limit', async () => {
    listKeys.mockResolvedValueOnce({ items: [{ ...createApiKey(), concurrency_limit: 8 }], total: 1 })
    const wrapper = await mountView()
    const cell = wrapper.get('[data-test="current-concurrency"]')
    expect(cell.text()).toContain('Concurrency 3 / 8')
    expect(cell.text()).toContain('Waiting 2 / 7')
  })

  it.each([[0, 'gray'], [2, 'emerald'], [4, 'amber'], [5, 'amber']])(
    'distinguishes concurrency %s against limit 4', async (current, color) => {
      setKeys({ ...createApiKey(), concurrency_limit: 4 })
      getConcurrency.mockResolvedValue({ ...snapshot(), items: [
        { id: 1, current_concurrency: current as number, current_waiting: 0 },
      ] })
      const wrapper = await mountView()
      const badge = wrapper.get('[data-test="current-concurrency"] [title="Concurrency"]')
      expect(badge.text()).toContain(`Concurrency ${current} / 4`)
      expect(badge.classes().join(' ')).toContain(`text-${color}-`)
    }
  )

  it('keeps the badge and switches its denominator after saving a changed limit', async () => {
    setKeys({ ...createApiKey(), group_id: 42, concurrency_limit: 0 })
    const wrapper = await mountView()
    for (const limit of [4, 0]) {
      await getButtonByText(wrapper, 'common.edit').trigger('click')
      if (limit === 4) expect(wrapper.get('#key-queue-policy').text()).toBe('No key-level queue.')
      await wrapper.get('#key-concurrency-limit').setValue(limit)
      setKeys({ ...createApiKey(), group_id: 42, concurrency_limit: limit })
      await wrapper.get('#key-form').trigger('submit')
      await flushPromises()
      const cell = wrapper.get('[data-test="current-concurrency"]')
      expect(cell.get('[title="Concurrency"]').find('svg').exists()).toBe(true)
      const text = cell.text()
      if (limit === 0) expect(text).toBe('Concurrency 3')
      else expect(text).toContain('Concurrency 3 / 4')
    }
  })

  it.each([12, 0, ''])('loads the saved limit and updates concurrency input %s', async (input) => {
    listKeys.mockResolvedValueOnce({ items: [{ ...createApiKey(), group_id: 42, concurrency_limit: 8 }], total: 1 })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    expect((wrapper.get('#key-concurrency-limit').element as HTMLInputElement).value).toBe('8')
    await wrapper.get('#key-concurrency-limit').setValue(input)
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(updateKey).toHaveBeenCalledWith(1, expect.objectContaining({ concurrency_limit: Number(input) }))
  })

  it.each([
    ['edit', -1], ['edit', 1.5],
  ])('rejects invalid concurrency in %s mode: %s', async (mode, input) => {
    listKeys.mockResolvedValueOnce({ items: [{ ...createApiKey(), group_id: 42 }], total: 1 })
    const wrapper = await mountView()
    await getButtonByText(wrapper, mode === 'edit' ? 'common.edit' : 'Create API Key').trigger('click')
    await wrapper.get('#key-concurrency-limit').setValue(input)
    expect(wrapper.get('#key-concurrency-limit').attributes('aria-invalid')).toBe('true')
    expect(wrapper.get('#key-concurrency-error').text()).toBe(messages['keys.concurrencyLimitInvalid'])
    await wrapper.get('#key-form').trigger('submit')
    expect(showError).toHaveBeenCalledWith(messages['keys.concurrencyLimitInvalid'])
    expect(createKey).not.toHaveBeenCalled()
    expect(updateKey).not.toHaveBeenCalled()
  })

  it('keeps the edit form and entered limit when the server rejects an update', async () => {
    listKeys.mockResolvedValueOnce({ items: [{ ...createApiKey(), group_id: 42, concurrency_limit: 8 }], total: 1 })
    updateKey.mockRejectedValueOnce({ response: { data: { detail: 'Concurrency limit rejected' } } })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    await wrapper.get('#key-concurrency-limit').setValue(12)
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('Concurrency limit rejected')
    expect(showSuccess).not.toHaveBeenCalled()
    expect((wrapper.get('#key-concurrency-limit').element as HTMLInputElement).value).toBe('12')
  })

  it('marks current concurrency as sortable', async () => {
    const wrapper = await mountView()

    const currentConcurrencyColumn = visibleColumnMeta(wrapper).find(
      (column) => column.key === 'current_concurrency'
    )
    expect(currentConcurrencyColumn?.sortable).toBe(true)
  })

  it('shows actual shared policy as read-only metadata and never submits it', async () => {
    setKeys({ ...createApiKey(), group_id: 42, concurrency_limit: 1 })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    expect(wrapper.get('#key-queue-policy').text()).toBe('This key allows 7 waiting requests for up to 12 seconds.')
    expect(wrapper.get('#key-concurrency-limit').attributes('aria-describedby')).toContain('key-queue-policy')
    expect(wrapper.find('#key-queue-policy input').exists()).toBe(false)
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    const payload = updateKey.mock.calls[0][1]
    expect(payload).toHaveProperty('concurrency_limit', 1)
    expect(Object.keys(payload).some(key => /queue|waiting|timeout/.test(key))).toBe(false)
  })

  it('distinguishes unknown statistics and policy from zero and still permits saving', async () => {
    setKeys({ ...createApiKey(), group_id: 42, concurrency_limit: 1 })
    const request = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(request.promise)
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    expect(wrapper.get('#key-queue-policy').text()).toBe('Loading shared queue settings…')
    expect(wrapper.get('[data-test="current-concurrency"]').text()).toContain('Concurrency — / 1')
    request.reject({ response: { status: 503 } })
    await flushPromises()
    expect(wrapper.get('#key-queue-policy').text()).toBe('Shared queue settings unavailable.')
    expect(wrapper.get('[data-test="current-concurrency"]').text()).not.toContain('/ 20')
    expect(showError).not.toHaveBeenCalled()
    await wrapper.get('#key-form').trigger('submit')
    await flushPromises()
    expect(updateKey).toHaveBeenCalled()
  })

  it('uses one actual policy for independent counts, including over-capacity and unlimited keys', async () => {
    setKeys(
      { ...createApiKey(), concurrency_limit: 1 },
      { ...createApiKey(), id: 2, concurrency_limit: 4 },
      { ...createApiKey(), id: 3, concurrency_limit: 0 },
    )
    getConcurrency.mockResolvedValue({ ...snapshot(), items: [
      { id: 1, current_concurrency: 1, current_waiting: 2 },
      { id: 2, current_concurrency: 4, current_waiting: 9 },
      { id: 3, current_concurrency: 2, current_waiting: 0 },
    ] })
    const wrapper = await mountView()
    const rows = wrapper.findAll('[data-test="current-concurrency"]').map(row => row.text())
    expect(rows[0]).toContain('Waiting 2 / 7')
    expect(rows[0]).not.toContain('Full')
    expect(rows[1]).toContain('Waiting 9 / 7 · Full')
    expect(rows[2]).toBe('Concurrency 2')
  })

  it('shows global-off policy and actual residual waiting without a zero denominator', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    getConcurrency.mockResolvedValue(snapshot(0, 2))
    const wrapper = await mountView()
    const text = wrapper.get('[data-test="current-concurrency"]').text()
    expect(text).toContain('Queuing globally disabled')
    expect(text).toContain('Waiting 2')
    expect(text).not.toContain('/ 0')
    expect(text).not.toContain('Full')
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    expect(wrapper.get('#key-queue-policy').text()).toBe(messages['keys.queuePolicyOff'])
  })

  it('does not treat a missing key limit or missing counts as an unlimited idle key', async () => {
    const key = { ...createApiKey(), concurrency_limit: undefined } as unknown as ApiKey
    setKeys(key)
    getConcurrency.mockResolvedValue({ ...snapshot(), items: [] })
    const wrapper = await mountView()
    const text = wrapper.get('[data-test="current-concurrency"]').text()
    expect(text).toContain('Concurrency —')
    expect(text).toContain('Statistics unavailable')
    expect(text).not.toContain('No additional limit')
  })

  it('shows unknown waiting against the actual policy when a requested count is missing', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    getConcurrency.mockResolvedValue({ ...snapshot(), items: [] })
    const wrapper = await mountView()
    const text = wrapper.get('[data-test="current-concurrency"]').text()
    expect(text).toContain('Concurrency — / 1')
    expect(text).toContain('Waiting — / 7')
    expect(text).toContain('Statistics unavailable')
    expect(text).not.toContain('Full')
  })

  it('polls after completion without reloading the list, usage, row order or modal input', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    const wrapper = await mountView()
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    await wrapper.get('#key-concurrency-limit').setValue(6)
    const listCalls = listKeys.mock.calls.length
    const usageCalls = getDashboardApiKeysUsage.mock.calls.length
    const request = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(request.promise)
    await vi.advanceTimersByTimeAsync(5000)
    expect(getConcurrency).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(15000)
    expect(getConcurrency).toHaveBeenCalledTimes(2)
    request.resolve(snapshot(4, 4))
    await flushPromises()
    expect(wrapper.get('[data-test="current-concurrency"]').text()).toContain('Waiting 4 / 4 · Full')
    expect(wrapper.get('#key-queue-policy').text()).toContain('4 waiting requests')
    expect((wrapper.get('#key-concurrency-limit').element as HTMLInputElement).value).toBe('6')
    expect(listKeys).toHaveBeenCalledTimes(listCalls)
    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(usageCalls)
    await vi.advanceTimersByTimeAsync(4999)
    expect(getConcurrency).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(getConcurrency).toHaveBeenCalledTimes(3)
  })

  it('marks old counts and policy stale on failure and removes the full marker until recovery', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    getConcurrency.mockResolvedValue(snapshot(7, 7))
    const wrapper = await mountView()
    expect(wrapper.get('[data-test="current-concurrency"]').text()).toContain('Full')
    getConcurrency.mockRejectedValueOnce({ response: { status: 503 } })
    await vi.advanceTimersByTimeAsync(5000)
    const text = wrapper.get('[data-test="current-concurrency"]').text()
    expect(text).toContain('Waiting 7 / 7')
    expect(text).toContain('Not updated')
    expect(text).not.toContain('Full')
    await getButtonByText(wrapper, 'common.edit').trigger('click')
    expect(wrapper.get('#key-queue-policy').text()).toContain('Not updated')
    expect(showError).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(5000)
    expect(wrapper.get('[data-test="current-concurrency"]').text()).toContain('Full')
    expect(wrapper.get('[data-test="current-concurrency"]').text()).not.toContain('Not updated')
  })

  it('aborts on hide, ignores a late reply, and resumes only after the old request settles', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    const request = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(request.promise)
    const wrapper = await mountView()
    const signal = getConcurrency.mock.calls[0][1].signal as AbortSignal
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(signal.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(15000)
    expect(getConcurrency).toHaveBeenCalledTimes(1)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(getConcurrency).toHaveBeenCalledTimes(1)
    request.resolve(snapshot(99, 99))
    await flushPromises()
    expect(getConcurrency).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-test="current-concurrency"]').text()).toContain('Waiting 2 / 7')
    expect(wrapper.text()).not.toContain('99')
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(10000)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(getConcurrency).toHaveBeenCalledTimes(2)
  })

  it('cancels pagination statistics and discards late old-page policy and counts', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    const request = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(request.promise)
    const wrapper = await mountView()
    const signal = getConcurrency.mock.calls[0][1].signal as AbortSignal
    setKeys({ ...createApiKey(), id: 2, concurrency_limit: 2 })
    getConcurrency.mockResolvedValue({ ...snapshot(4), items: [{ id: 2, current_concurrency: 1, current_waiting: 0 }] })
    await wrapper.get('[data-test="keys-pagination"] button[aria-label="pagination.next"]').trigger('click')
    await flushPromises()
    expect(signal.aborted).toBe(true)
    expect(getConcurrency).toHaveBeenCalledTimes(1)
    request.resolve(snapshot(99, 99))
    await flushPromises()
    expect(getConcurrency).toHaveBeenLastCalledWith([2], expect.any(Object))
    const cell = wrapper.get('[data-test="current-concurrency"]')
    expect(cell.text()).toContain('Concurrency 1 / 2')
    expect(cell.text()).toContain('Waiting 0 / 4')
  })

  it('aborts an in-flight statistics request on unmount without scheduling another', async () => {
    const request = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(request.promise)
    const wrapper = await mountView()
    const signal = getConcurrency.mock.calls[0][1].signal as AbortSignal
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    request.resolve(snapshot())
    await flushPromises()
    await vi.advanceTimersByTimeAsync(10000)
    expect(getConcurrency).toHaveBeenCalledTimes(1)
  })

  it('reads pages above 100 IDs serially and publishes policy and counts only when complete', async () => {
    setKeys(...Array.from({ length: 101 }, (_, i) => ({ ...createApiKey(), id: i + 1, concurrency_limit: 1 })))
    const first = deferred<ApiKeyConcurrencySnapshot>()
    const second = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const wrapper = await mountView()
    expect(getConcurrency.mock.calls[0][0]).toHaveLength(100)
    expect(getConcurrency).toHaveBeenCalledTimes(1)
    first.resolve(snapshot(4, 2))
    await flushPromises()
    expect(getConcurrency.mock.calls[1][0]).toEqual([101])
    expect(wrapper.findAll('[data-test="current-concurrency"]')[0].text()).toContain('Concurrency — / 1')
    second.resolve({ ...snapshot(4), items: [{ id: 101, current_concurrency: 1, current_waiting: 3 }] })
    await flushPromises()
    const rows = wrapper.findAll('[data-test="current-concurrency"]')
    expect(rows[0].text()).toContain('Waiting 2 / 4')
    expect(rows[100].text()).toContain('Waiting 3 / 4')
  })

  it('does not combine counts from batches reporting different policies', async () => {
    setKeys(...Array.from({ length: 101 }, (_, i) => ({ ...createApiKey(), id: i + 1, concurrency_limit: 1 })))
    getConcurrency.mockResolvedValueOnce(snapshot(4)).mockResolvedValueOnce(snapshot(8))
    const wrapper = await mountView()
    const text = wrapper.findAll('[data-test="current-concurrency"]')[0].text()
    expect(text).toContain('Concurrency — / 1')
    expect(text).toContain('Shared queue settings unavailable.')
    expect(text).not.toContain('Waiting 2 / 4')
    expect(showError).not.toHaveBeenCalled()
  })

  it('cancels old statistics during a filter change and keeps filters and row order on polling', async () => {
    setKeys({ ...createApiKey(), concurrency_limit: 1 })
    const request = deferred<ApiKeyConcurrencySnapshot>()
    getConcurrency.mockReturnValueOnce(request.promise)
    const wrapper = await mountView()
    const signal = getConcurrency.mock.calls[0][1].signal as AbortSignal
    setKeys(
      { ...createApiKey(), id: 2, concurrency_limit: 1 },
      { ...createApiKey(), id: 3, concurrency_limit: 1 },
    )
    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('update:modelValue', 'target')
    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('search')
    await flushPromises()
    expect(signal.aborted).toBe(true)
    getConcurrency.mockResolvedValue({ ...snapshot(), items: [
      { id: 3, current_concurrency: 8, current_waiting: 6 },
      { id: 2, current_concurrency: 1, current_waiting: 2 },
    ] })
    request.resolve(snapshot(99, 99))
    await flushPromises()
    expect(getConcurrency).toHaveBeenLastCalledWith([2, 3], expect.any(Object))
    await vi.advanceTimersByTimeAsync(5000)
    expect(wrapper.findComponent({ name: 'SearchInput' }).props('modelValue')).toBe('target')
    expect(wrapper.findComponent({ name: 'DataTable' }).props('data').map((key: ApiKey) => key.id)).toEqual([2, 3])
    expect(listKeys).toHaveBeenCalledTimes(2)
    expect(getDashboardApiKeysUsage).toHaveBeenCalledTimes(2)
    const rows = wrapper.findAll('[data-test="current-concurrency"]')
    expect(rows[0].text()).toContain('Concurrency 1 / 1')
    expect(rows[1].text()).toContain('Concurrency 8 / 1')
  })

  it('does not poll a page mounted in the background until it becomes visible', async () => {
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    await mountView()
    await vi.advanceTimersByTimeAsync(10000)
    expect(getConcurrency).not.toHaveBeenCalled()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    expect(getConcurrency).toHaveBeenCalledTimes(1)
  })

  it.each([
    { key: 'current_concurrency', order: 'asc' },
    { key: 'group', order: 'asc' },
    { key: 'group', order: 'desc' },
  ] as const)('keeps filters and resets pagination and selection when sorting $key $order', async ({ key, order }) => {
    getAvailableGroups.mockResolvedValue([{ id: 42, name: 'OpenAI' }])
    const wrapper = await mountView()


    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('update:modelValue', 'target')
    await wrapper.findComponent({ name: 'SearchInput' }).vm.$emit('search')
    await flushPromises()

    const selects = wrapper.findAllComponents({ name: 'Select' })
    await selects[0].vm.$emit('update:modelValue', 42)
    await flushPromises()
    await selects[1].vm.$emit('update:modelValue', 'active')
    await flushPromises()

    await wrapper.get('[data-test="keys-pagination"] button[aria-label="pagination.next"]').trigger('click')
    await flushPromises()
    const table = wrapper.findComponent({ name: 'DataTable' })
    table.vm.$emit('update:selectedKeys', [1])
    await nextTick()
    expect(table.props('selectedKeys')).toEqual([1])
    expect(visibleColumnMeta(wrapper).find((column) => column.key === key)?.sortable).toBe(true)
    listKeys.mockClear()

    table.vm.$emit('sort', key, order)
    await flushPromises()

    expect(table.props('selectedKeys')).toEqual([])
    expect(listKeys).toHaveBeenLastCalledWith(
      1,
      5,
      {
        search: 'target',
        status: 'active',
        group_id: 42,
        sort_by: key,
        sort_order: order,
      },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })

})
