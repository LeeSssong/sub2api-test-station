import { describe, expect, it, vi, beforeEach, afterEach, beforeAll, afterAll } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'

import UsageView from '../UsageView.vue'

const { list, exportList, listCostExceptions, getStats, getSnapshotV2, getById, getModelStats, listErrorLogs, routeQuery, observedXingqiaoRequests, aoaToSheet, sheetAddAoa, saveAs, xlsxWrite } = vi.hoisted(() => {
  vi.stubGlobal('localStorage', {
    getItem: vi.fn(() => null),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  })

  return {
    list: vi.fn(),
	 exportList: vi.fn(),
    listCostExceptions: vi.fn(),
    getStats: vi.fn(),
    getSnapshotV2: vi.fn(),
    getById: vi.fn(),
    getModelStats: vi.fn(),
    listErrorLogs: vi.fn(),
    routeQuery: {} as Record<string, string>,
    observedXingqiaoRequests: [] as string[],
		aoaToSheet: vi.fn(() => ({})),
		sheetAddAoa: vi.fn(),
		saveAs: vi.fn(),
		xlsxWrite: vi.fn(() => new Uint8Array([1, 2, 3])),
  }
})

const messages: Record<string, string> = {
  'admin.dashboard.timeRange': 'Time Range',
  'admin.dashboard.day': 'Day',
  'admin.dashboard.hour': 'Hour',
  'admin.usage.failedToLoadUser': 'Failed to load user',
  'admin.users.columnSettings': 'Columns',
  'usage.detail.action': 'Details',
  'admin.usage.requestId': 'Request ID',
	'usage.requestedModel': 'Requested model',
	'usage.sentUpstreamModel': 'Sent upstream model',
	'usage.upstreamResponseModel': 'Upstream response model',
	'usage.upstreamModelMismatch': 'Upstream model mismatch',
	'common.yes': 'Yes',
	'common.no': 'No',
}

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

vi.mock('@/api/admin', () => ({
  adminAPI: {
    usage: {
      list,
      getStats,
    },
    dashboard: {
      getSnapshotV2,
      getModelStats,
    },
    users: {
      getById,
    },
  },
}))

vi.mock('@/api/admin/usage', () => ({
  adminUsageAPI: {
	list: exportList,
    listCostExceptions,
  },
}))

vi.mock('file-saver', () => ({ saveAs }))

vi.mock('xlsx', () => ({
	utils: {
		aoa_to_sheet: aoaToSheet,
		sheet_add_aoa: sheetAddAoa,
		book_new: vi.fn(() => ({})),
		book_append_sheet: vi.fn(),
	},
	write: xlsxWrite,
}))

vi.mock('@/api/admin/ops', () => ({
  listErrorLogs,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showWarning: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/utils/format', () => ({
  formatReasoningEffort: (value: string | null | undefined) => value ?? '-',
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: routeQuery
  })
}))

const AppLayoutStub = { template: '<div><slot /></div>' }

let originalXhrOpen: typeof XMLHttpRequest.prototype.open

beforeAll(() => {
  originalXhrOpen = XMLHttpRequest.prototype.open
  XMLHttpRequest.prototype.open = function (...args: Parameters<typeof XMLHttpRequest.prototype.open>) {
    const url = String(args[1] ?? '')
    const externalPathSegment = ['xing', 'qiao'].join('')
    if (url.includes(`/${externalPathSegment}/`)) {
      observedXingqiaoRequests.push(url)
    }
    return originalXhrOpen.apply(this, args)
  }
})

afterAll(() => {
  XMLHttpRequest.prototype.open = originalXhrOpen
})

const UsageFiltersStub = defineComponent({
  emits: ['refresh'],
  setup(_, { expose }) {
    const userKeyword = ref('')
    let userSearchRevision = 0
    const setUserKeyword = (email: string) => {
      userSearchRevision += 1
      userKeyword.value = email
    }
    expose({
      getUserSearchRevision: () => userSearchRevision,
      setUserKeyword,
      simulateUserInput: setUserKeyword,
    })
    return { userKeyword }
  },
  template: '<div><span data-test="user-filter-label">{{ userKeyword }}</span><button data-test="refresh-filter" @click="$emit(\'refresh\')">refresh</button><slot name="after-reset" /></div>',
})
const UsageTableStub = {
  name: 'UsageTable',
  props: ['columns'],
  emits: ['userClick', 'detailClick'],
  template: `
    <div data-test="usage-table">
      <button class="user-click" @click="$emit('userClick', 2)">user</button>
      <button class="detail-click" @click="$emit('detailClick', 42)">details</button>
    </div>
  `,
}
const UsageDetailDialogStub = {
  name: 'UsageDetailDialog',
  props: ['show', 'usageId', 'scope'],
  emits: ['update:show'],
  template: '<div data-test="usage-detail-dialog" />',
}
const CostExceptionTableStub = {
  name: 'CostExceptionTable',
  props: ['filters'],
  emits: ['detail', 'reviewed'],
  template: '<div data-test="cost-exception-table"><button data-test="exception-detail" @click="$emit(\'detail\', 11)">exception details</button></div>',
}
const OpsErrorLogTableStub = {
  name: 'OpsErrorLogTable',
  emits: ['openErrorDetail'],
  template: '<button data-test="error-detail-action" @click="$emit(\'openErrorDetail\', 7)">error details</button>',
}
const OpsErrorDetailModalStub = {
  name: 'OpsErrorDetailModal',
  props: ['show', 'errorId', 'errorType'],
  emits: ['update:show'],
  template: '<div data-test="error-detail-dialog" />',
}
const UserTokenRankingStub = {
  emits: ['select-user'],
  template: '<div data-test="ranking"><button class="pick-user" @click="$emit(\'select-user\', 5, \'rank@test.com\')">pick</button></div>',
}
const ModelDistributionChartStub = {
  props: ['metric'],
  emits: ['update:metric'],
  template: `
    <div data-test="model-chart">
      <span class="metric">{{ metric }}</span>
      <button class="switch-metric" @click="$emit('update:metric', 'actual_cost')">switch</button>
    </div>
  `,
}
const GroupDistributionChartStub = {
  props: ['metric'],
  emits: ['update:metric'],
  template: `
    <div data-test="group-chart">
      <span class="metric">{{ metric }}</span>
      <button class="switch-metric" @click="$emit('update:metric', 'actual_cost')">switch</button>
    </div>
  `,
}

const mountRouteFilteredUsageView = () => mount(UsageView, {
  global: { stubs: {
    AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
    UsageTable: true, CostExceptionTable: CostExceptionTableStub, UsageExportProgress: true, UsageCleanupDialog: true,
    UserBalanceHistoryModal: true, Pagination: true, Select: true,
    DateRangePicker: true, Icon: true, TokenUsageTrend: true,
    ModelDistributionChart: true, GroupDistributionChart: true,
    EndpointDistributionChart: true, UserTokenRanking: true,
  } },
})

describe('admin UsageView route filters', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    Object.keys(routeQuery).forEach((key) => delete routeQuery[key])
    observedXingqiaoRequests.length = 0
    list.mockReset().mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStats.mockReset().mockResolvedValue({
      total_requests: 0,
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_cache_tokens: 0,
      total_tokens: 0,
      total_cost: 0,
      total_actual_cost: 0,
      average_duration_ms: 0,
    })
    getSnapshotV2.mockReset().mockResolvedValue({ trend: [], models: [], groups: [] })
    getModelStats.mockReset().mockResolvedValue({ models: [] })
    getById.mockReset()
    listCostExceptions.mockReset().mockResolvedValue({ generated_at: '2026-08-15T10:00:00Z', items: [], total: 0, page: 1, page_size: 20 })
  })

  afterEach(() => {
    Object.keys(routeQuery).forEach((key) => delete routeQuery[key])
    vi.useRealTimers()
  })

  it('shows the routed user while applying user_id to usage requests', async () => {
    routeQuery.user_id = '42'
    getById.mockResolvedValue({ id: 42, email: 'route-user@test.com' })

    const wrapper = mountRouteFilteredUsageView()
    await flushPromises()

    expect(getById).toHaveBeenCalledWith(42, true)
    expect(list).toHaveBeenCalledWith(expect.objectContaining({ user_id: 42 }), expect.anything())
    expect(wrapper.find('[data-test="user-filter-label"]').text()).toBe('route-user@test.com')
  })

  it('restores the cost exception tab and financial route filters', async () => {
    routeQuery.tab = 'cost-exceptions'
    routeQuery.range = 'today'
    routeQuery.account_id = '42'
    routeQuery.evidence = 'unavailable'
    routeQuery.review = 'reviewed'

    const wrapper = mountRouteFilteredUsageView()
    await flushPromises()

    const tabs = wrapper.findAll('[data-testid="usage-detail-tab"]')
    expect(tabs).toHaveLength(4)
    expect(tabs[3].classes()).toContain('border-primary-500')
    const exceptionTable = wrapper.getComponent(CostExceptionTableStub)
    expect(exceptionTable.props('filters')).toEqual(expect.objectContaining({
      account_id: 42,
      evidence_status: 'unavailable',
      review_status: 'reviewed',
      start_time: expect.any(String),
      end_time: expect.any(String),
    }))
  })

  it('mounts the routed exception table once with pending account and RFC3339 range filters', async () => {
    routeQuery.tab = 'cost-exceptions'
    routeQuery.range = 'today'
    routeQuery.account_id = '42'
    routeQuery.review = 'pending'

    mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true,
        EndpointDistributionChart: true, UserTokenRanking: true,
      } },
    })
    await flushPromises()

    expect(listCostExceptions).toHaveBeenCalledTimes(1)
    expect(listCostExceptions).toHaveBeenCalledWith(expect.objectContaining({
      account_id: 42,
      review_status: 'pending',
      start_time: expect.stringMatching(/^\d{4}-\d{2}-\d{2}T/),
      end_time: expect.stringMatching(/^\d{4}-\d{2}-\d{2}T/),
    }))
  })

  it('opens the existing administrator detail from an exception row', async () => {
    routeQuery.tab = 'cost-exceptions'
    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, CostExceptionTable: CostExceptionTableStub,
        UsageDetailDialog: UsageDetailDialogStub, UsageExportProgress: true,
        UsageCleanupDialog: true, UserBalanceHistoryModal: true, Pagination: true,
        Select: true, DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true,
        EndpointDistributionChart: true, UserTokenRanking: true,
      } },
    })
    await flushPromises()

    await wrapper.get('[data-test="exception-detail"]').trigger('click')
    const dialog = wrapper.getComponent(UsageDetailDialogStub)
    expect(dialog.props()).toMatchObject({ usageId: 11, scope: 'admin', show: true })
  })

  it('uses native usage stats and rows without rendering control-plane status', async () => {
    getStats.mockResolvedValueOnce({
      total_requests: 7, total_input_tokens: 10, total_output_tokens: 20, total_cache_tokens: 0,
      total_cache_creation_tokens: 0, total_cache_read_tokens: 0, total_tokens: 30,
      total_cost: 2, total_actual_cost: 1, total_account_cost: 1, average_duration_ms: 100,
    })

    const wrapper = mountRouteFilteredUsageView()
    await flushPromises()

    expect(list).toHaveBeenCalledWith(expect.objectContaining({ page: 1 }), expect.anything())
    expect(getStats).toHaveBeenCalledWith(expect.objectContaining({ start_date: expect.any(String), end_date: expect.any(String) }))
    expect((wrapper.vm as any).usageStats).toMatchObject({
      total_requests: 7,
      total_cost: 2,
      total_actual_cost: 1,
      total_tokens: 30,
    })
    expect(wrapper.text()).not.toContain('控制面暂时不可用')
    expect(wrapper.text()).not.toContain('完整性')
    expect(wrapper.text()).not.toContain('来源：现有系统')
    expect(wrapper.text()).not.toContain('来源：控制面')
    expect(observedXingqiaoRequests).toEqual([])
  })

  it('refreshes usage data through native endpoints without control-plane status', async () => {
    const wrapper = mountRouteFilteredUsageView()
    await flushPromises()
    getStats.mockClear()
    list.mockClear()

    await wrapper.getComponent(UsageFiltersStub).vm.$emit('refresh')
    await flushPromises()

    expect(list).toHaveBeenCalledWith(expect.objectContaining({ page: 1 }), expect.anything())
    expect(getStats).toHaveBeenCalledWith(expect.objectContaining({ start_date: expect.any(String), end_date: expect.any(String) }))
    expect(wrapper.text()).not.toContain('控制面暂时不可用')
    expect(wrapper.text()).not.toContain('完整性')
    expect(observedXingqiaoRequests).toEqual([])
  })

  it('does not apply a stale routed user label after user_id changes', async () => {
    routeQuery.user_id = '42'
    let resolveLookup!: (user: { id: number; email: string }) => void
    getById.mockReturnValue(new Promise((resolve) => { resolveLookup = resolve }))

    const wrapper = mountRouteFilteredUsageView()
    await wrapper.vm.$nextTick()
    ;(wrapper.vm as any).filters.user_id = 84
    ;(wrapper.findComponent(UsageFiltersStub).vm as any).setUserKeyword('current-user@test.com')

    resolveLookup({ id: 42, email: 'stale-user@test.com' })
    await flushPromises()

    expect(wrapper.find('[data-test="user-filter-label"]').text()).toBe('current-user@test.com')
  })

  it('does not overwrite newer user input when the routed user lookup succeeds', async () => {
    routeQuery.user_id = '42'
    let resolveLookup!: (user: { id: number; email: string }) => void
    getById.mockReturnValue(new Promise((resolve) => { resolveLookup = resolve }))

    const wrapper = mountRouteFilteredUsageView()
    await wrapper.vm.$nextTick()
    ;(wrapper.findComponent(UsageFiltersStub).vm as any).simulateUserInput('new-search@test.com')

    resolveLookup({ id: 42, email: 'route-user@test.com' })
    await flushPromises()

    expect((wrapper.vm as any).filters.user_id).toBe(42)
    expect(wrapper.find('[data-test="user-filter-label"]').text()).toBe('new-search@test.com')
  })

  it('does not overwrite newer user input when the routed user lookup fails', async () => {
    routeQuery.user_id = '42'
    let rejectLookup!: (error: Error) => void
    getById.mockReturnValue(new Promise((_, reject) => { rejectLookup = reject }))

    const wrapper = mountRouteFilteredUsageView()
    await wrapper.vm.$nextTick()
    ;(wrapper.findComponent(UsageFiltersStub).vm as any).simulateUserInput('new-search@test.com')

    rejectLookup(new Error('lookup failed'))
    await flushPromises()

    expect((wrapper.vm as any).filters.user_id).toBe(42)
    expect(wrapper.find('[data-test="user-filter-label"]').text()).toBe('new-search@test.com')
  })

  it('shows the routed user ID when its label lookup fails', async () => {
    routeQuery.user_id = '42'
    getById.mockRejectedValue(new Error('lookup failed'))

    const wrapper = mountRouteFilteredUsageView()
    await flushPromises()

    expect(list).toHaveBeenCalledWith(expect.objectContaining({ user_id: 42 }), expect.anything())
    expect(wrapper.find('[data-test="user-filter-label"]').text()).toBe('42')
  })
})

describe('admin UsageView distribution metric toggles', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    list.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getById.mockReset()
    getModelStats.mockReset()

    list.mockResolvedValue({
      items: [],
      total: 0,
      pages: 0,
    })
    getStats.mockResolvedValue({
      total_requests: 0,
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_cache_tokens: 0,
      total_tokens: 0,
      total_cost: 0,
      total_actual_cost: 0,
      average_duration_ms: 0,
    })
    getSnapshotV2.mockResolvedValue({
      trend: [],
      models: [],
      groups: [],
    })
    getModelStats.mockResolvedValue({ models: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('keeps previous model stats visible during refresh until new data arrives', async () => {
    // 首次加载返回 A
    getModelStats.mockResolvedValueOnce({ models: [{ model: 'A', total_tokens: 10 }] })

    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, AuditLogModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: ModelDistributionChartStub, GroupDistributionChart: GroupDistributionChartStub,
        EndpointDistributionChart: true, UserTokenRanking: true,
      } },
    })
    vi.advanceTimersByTime(120)
    await flushPromises()
    expect((wrapper.vm as any).requestedModelStats).toEqual([{ model: 'A', total_tokens: 10 }])

    // 刷新:让第二次 getModelStats 处于 pending,断言旧数据 A 仍在(不被清空成 [])
    let resolveSecond: (v: any) => void = () => {}
    getModelStats.mockReturnValueOnce(new Promise((res) => { resolveSecond = res }))
    ;(wrapper.vm as any).refreshData()
    await flushPromises()
    expect((wrapper.vm as any).requestedModelStats).toEqual([{ model: 'A', total_tokens: 10 }])

    // 新数据到达后替换为 B
    resolveSecond({ models: [{ model: 'B', total_tokens: 20 }] })
    await flushPromises()
    expect((wrapper.vm as any).requestedModelStats).toEqual([{ model: 'B', total_tokens: 20 }])
  })

  it('keeps model and group metric toggles independent without refetching chart data', async () => {
    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: true,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: ModelDistributionChartStub,
          GroupDistributionChart: GroupDistributionChartStub,
          UserTokenRanking: true,
        },
      },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: formatLocalDate(yesterday),
      end_date: formatLocalDate(now),
      granularity: 'hour'
    }))

    const modelChart = wrapper.find('[data-test="model-chart"]')
    const groupChart = wrapper.find('[data-test="group-chart"]')

    expect(modelChart.find('.metric').text()).toBe('tokens')
    expect(groupChart.find('.metric').text()).toBe('tokens')

    await modelChart.find('.switch-metric').trigger('click')
    await flushPromises()

    expect(modelChart.find('.metric').text()).toBe('actual_cost')
    expect(groupChart.find('.metric').text()).toBe('tokens')
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)

    await groupChart.find('.switch-metric').trigger('click')
    await flushPromises()

    expect(modelChart.find('.metric').text()).toBe('actual_cost')
    expect(groupChart.find('.metric').text()).toBe('actual_cost')
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
  })
})

describe('admin UsageView request ID column visibility', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.mocked(localStorage.getItem).mockReset().mockReturnValue(null)
    vi.mocked(localStorage.setItem).mockReset()
    list.mockReset().mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStats.mockReset().mockResolvedValue({
      total_requests: 0, total_input_tokens: 0, total_output_tokens: 0,
      total_cache_tokens: 0, total_tokens: 0, total_cost: 0, total_actual_cost: 0, average_duration_ms: 0,
    })
    getSnapshotV2.mockReset().mockResolvedValue({ trend: [], models: [], groups: [] })
    getModelStats.mockReset().mockResolvedValue({ models: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('keeps request ID hidden by default and allows enabling it from column settings', async () => {
    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: UsageTableStub,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          AuditLogModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: true,
          GroupDistributionChart: true,
          EndpointDistributionChart: true,
          UserTokenRanking: true,
        },
      },
    })
    await wrapper.vm.$nextTick()

    const usageTable = wrapper.findComponent(UsageTableStub)
    expect(usageTable.props('columns')).not.toEqual(
      expect.arrayContaining([expect.objectContaining({ key: 'request_id' })]),
    )

    await wrapper.get('button[title="Columns"]').trigger('click')
    const requestIdToggle = wrapper.findAll('button').find((button) => button.text() === 'Request ID')
    expect(requestIdToggle).toBeDefined()
    await requestIdToggle!.trigger('click')

    expect(usageTable.props('columns')).toEqual(
      expect.arrayContaining([expect.objectContaining({ key: 'request_id', label: 'Request ID' })]),
    )
    expect(localStorage.setItem).toHaveBeenCalledWith(
      'usage-hidden-columns-version',
      'request-id-hidden-by-default',
    )
  })
})

describe('admin UsageView handleUserClick', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    list.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getById.mockReset()

    list.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStats.mockResolvedValue({
      total_requests: 0, total_input_tokens: 0, total_output_tokens: 0,
      total_cache_tokens: 0, total_tokens: 0, total_cost: 0, total_actual_cost: 0, average_duration_ms: 0,
    })
    getSnapshotV2.mockResolvedValue({ trend: [], models: [], groups: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('opens user via include_deleted when clicking a usage row user', async () => {
    getById.mockResolvedValue({ id: 2, email: 'd@test.com', deleted_at: '2026-05-28T00:00:00Z' })

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: UsageTableStub,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          AuditLogModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: true,
          GroupDistributionChart: true,
          EndpointDistributionChart: true,
          UserTokenRanking: true,
        },
      },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    await wrapper.find('[data-test="usage-table"] .user-click').trigger('click')
    await flushPromises()

    expect(getById).toHaveBeenCalledWith(2, true)
  })
})

describe('admin UsageView detail wiring', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    list.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getModelStats.mockReset()
    listErrorLogs.mockReset()

    list.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStats.mockResolvedValue({
      total_requests: 0, total_input_tokens: 0, total_output_tokens: 0,
      total_cache_tokens: 0, total_tokens: 0, total_cost: 0, total_actual_cost: 0, average_duration_ms: 0,
    })
    getSnapshotV2.mockResolvedValue({ trend: [], models: [], groups: [] })
    getModelStats.mockResolvedValue({ models: [] })
    listErrorLogs.mockResolvedValue({ items: [], total: 0, pages: 0 })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('keeps the successful detail column fixed at the right edge and opens the admin dialog', async () => {
    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: UsageTableStub, UsageDetailDialog: UsageDetailDialogStub,
        UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        UserTokenRanking: true, OpsErrorLogTable: true, OpsErrorDetailModal: true,
      } },
    })
    vi.advanceTimersByTime(120)
    await flushPromises()

    const usageTable = wrapper.getComponent({ name: 'UsageTable' })
    const columns = usageTable.props('columns') as Array<{ key: string; sortable?: boolean; class?: string }>
    expect(columns.at(-1)).toEqual(expect.objectContaining({
      key: 'detail',
      sortable: false,
      class: 'w-24 min-w-24',
    }))
    expect((wrapper.vm as any).toggleableColumns.map((column: { key: string }) => column.key))
      .not.toContain('detail')

    await wrapper.get('.detail-click').trigger('click')

    const dialog = wrapper.getComponent({ name: 'UsageDetailDialog' })
    expect(dialog.props('show')).toBe(true)
    expect(dialog.props('usageId')).toBe(42)
    expect(dialog.props('scope')).toBe('admin')
  })

  it('preserves the explicit administrator error-row detail action and dialog', async () => {
    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: UsageTableStub, UsageDetailDialog: UsageDetailDialogStub,
        UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        UserTokenRanking: true, OpsErrorLogTable: OpsErrorLogTableStub,
        OpsErrorDetailModal: OpsErrorDetailModalStub,
      } },
    })
    vi.advanceTimersByTime(120)
    await flushPromises()

    const tabs = wrapper.findAll('[data-testid="usage-detail-tab"]')
    await tabs[1].trigger('click')
    await wrapper.get('[data-test="error-detail-action"]').trigger('click')

    const errorDialog = wrapper.getComponent({ name: 'OpsErrorDetailModal' })
    expect(errorDialog.props('show')).toBe(true)
    expect(errorDialog.props('errorId')).toBe(7)
    expect(errorDialog.props('errorType')).toBe('request')
  })
})

describe('admin UsageView errors tab filter forwarding', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    list.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getModelStats.mockReset()
    listErrorLogs.mockReset()

    list.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStats.mockResolvedValue({
      total_requests: 0, total_input_tokens: 0, total_output_tokens: 0,
      total_cache_tokens: 0, total_tokens: 0, total_cost: 0, total_actual_cost: 0, average_duration_ms: 0,
    })
    getSnapshotV2.mockResolvedValue({ trend: [], models: [], groups: [] })
    getModelStats.mockResolvedValue({ models: [] })
    listErrorLogs.mockResolvedValue({ items: [], total: 0, pages: 0 })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('forwards model/account_id/group_id to listErrorLogs on the errors tab', async () => {
    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, AuditLogModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        UserTokenRanking: true, OpsErrorLogTable: true, OpsErrorDetailModal: true,
      } },
    })
    vi.advanceTimersByTime(120)
    await flushPromises()

    // 模拟用户在过滤器里选择了模型/账户/分组
    const vm = wrapper.vm as any
    vm.filters.model = 'gpt-5.3-codex'
    vm.filters.account_id = 7
    vm.filters.group_id = 3
    await flushPromises()

    // 切换到「错误请求」标签（第二个 tab 按钮）触发 loadAdminErrors
    const tabs = wrapper.findAll('[data-testid="usage-detail-tab"]')
    await tabs[1].trigger('click')
    await flushPromises()

    expect(listErrorLogs).toHaveBeenCalledWith(expect.objectContaining({
      view: 'all',
      model: 'gpt-5.3-codex',
      account_id: 7,
      group_id: 3,
    }))
  })
})

describe('admin UsageView ranking tab', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    list.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getModelStats.mockReset()

    list.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStats.mockResolvedValue({
      total_requests: 0, total_input_tokens: 0, total_output_tokens: 0,
      total_cache_tokens: 0, total_tokens: 0, total_cost: 0, total_actual_cost: 0, average_duration_ms: 0,
    })
    getSnapshotV2.mockResolvedValue({ trend: [], models: [], groups: [] })
    getModelStats.mockResolvedValue({ models: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('mounts ranking lazily and drill-down sets user filter then jumps back to usage tab', async () => {
    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        UserTokenRanking: UserTokenRankingStub, OpsErrorLogTable: true, OpsErrorDetailModal: true,
      } },
    })
    vi.advanceTimersByTime(120)
    await flushPromises()

    // 懒挂载:切到排行 tab 前不渲染
    expect(wrapper.find('[data-test="ranking"]').exists()).toBe(false)

    const tabs = wrapper.findAll('[data-testid="usage-detail-tab"]')
    expect(tabs).toHaveLength(4)
    await tabs[2].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="ranking"]').exists()).toBe(true)

    // 下钻:设置 user_id、切回用量明细 tab 并按新筛选重新拉取列表
    list.mockClear()
    await wrapper.find('[data-test="ranking"] .pick-user').trigger('click')
    await flushPromises()

    expect((wrapper.vm as any).activeTab).toBe('usage')
    expect((wrapper.vm as any).filters.user_id).toBe(5)
    expect(list).toHaveBeenCalledWith(expect.objectContaining({ user_id: 5 }), expect.anything())
  })
})

describe('admin UsageView model audit export', () => {
	beforeEach(() => {
		vi.useFakeTimers()
		list.mockReset().mockResolvedValue({ items: [], total: 0, pages: 0 })
		exportList.mockReset().mockResolvedValue({
			items: [{
				id: 1,
				created_at: '2026-08-04T00:00:00Z',
				model: 'gpt-5.6-sol',
				upstream_model: 'gpt-5.5',
				upstream_response_model: 'gpt-5.4',
				upstream_model_mismatch: true,
				request_type: 'sync',
				input_tokens: 1,
				output_tokens: 1,
				cache_read_tokens: 0,
				cache_creation_tokens: 0,
				duration_ms: 10,
			}],
			total: 1,
			pages: 1,
		})
		getStats.mockReset().mockResolvedValue({
			total_requests: 0, total_input_tokens: 0, total_output_tokens: 0,
			total_cache_tokens: 0, total_tokens: 0, total_cost: 0, total_actual_cost: 0, average_duration_ms: 0,
		})
		getSnapshotV2.mockReset().mockResolvedValue({ trend: [], models: [], groups: [] })
		getModelStats.mockReset().mockResolvedValue({ models: [] })
		aoaToSheet.mockClear()
		sheetAddAoa.mockClear()
		saveAs.mockClear()
		xlsxWrite.mockClear()
	})

	afterEach(() => {
		vi.useRealTimers()
	})

	it('exports requested, sent, response, and mismatch as separate admin columns', async () => {
		const wrapper = mountRouteFilteredUsageView()
		vi.advanceTimersByTime(120)
		await flushPromises()

		await (wrapper.vm as any).exportToExcel()
		await flushPromises()

		const headers = aoaToSheet.mock.calls[0][0][0]
		expect(headers.slice(4, 8)).toEqual([
			'Requested model',
			'Sent upstream model',
			'Upstream response model',
			'Upstream model mismatch',
		])
		const row = sheetAddAoa.mock.calls[0][1][0]
		expect(row.slice(4, 8)).toEqual(['gpt-5.6-sol', 'gpt-5.5', 'gpt-5.4', 'Yes'])
		expect(saveAs).toHaveBeenCalledTimes(1)
	})
})
