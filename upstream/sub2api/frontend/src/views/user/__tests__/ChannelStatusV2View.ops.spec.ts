import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const routeQuery = vi.hoisted(() => ({ query: {} as Record<string, unknown> }))
const apiMocks = vi.hoisted(() => ({
  getDimensions: vi.fn(),
  getSnapshot: vi.fn(),
  getMatrix: vi.fn(),
  getModels: vi.fn(),
  getErrors: vi.fn(),
  getUsers: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeQuery,
  useRouter: () => ({ replace: vi.fn() }),
}))

vi.mock('@/api/channelMonitorV2', async () => {
  const actual = await vi.importActual<typeof import('@/api/channelMonitorV2')>('@/api/channelMonitorV2')
  return { ...actual, ...apiMocks }
})

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ isAdmin: false }),
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn() }),
}))
vi.mock('@/utils/featureFlags', () => ({
  isChannelMonitorThroughputHidden: () => false,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
    locale: { value: 'zh-CN' },
    te: (key: string) => key.startsWith('channelMonitorV2.'),
    t: (key: string, params?: Record<string, unknown>) => {
      const map: Record<string, string> = {
        'channelMonitorV2.title': '渠道监控',
        'channelMonitorV2.updatedTo': '更新至 {time}',
        'channelMonitorV2.timeRange': '时间范围',
        'channelMonitorV2.ranges.1h': '1小时',
        'channelMonitorV2.ranges.24h': '24h',
        'channelMonitorV2.ranges.7d': '7d',
        'channelMonitorV2.summaryAria': '整体汇总',
        'channelMonitorV2.metrics.successRate': '成功率',
        'channelMonitorV2.metrics.errorRateValue': '错误率 {value}',
        'channelMonitorV2.metrics.ttftP50': '首 Token P50',
        'channelMonitorV2.metrics.cacheRate': '缓存率',
        'channelMonitorV2.metrics.cacheDetail': '读缓存占比',
        'channelMonitorV2.matrix.dimension': '渠道维度',
        'channelMonitorV2.matrix.title': '可用性趋势',
        'channelMonitorV2.matrix.description': '趋势色块视图',
        'channelMonitorV2.trendView.label': '趋势视图',
        'channelMonitorV2.trendView.pulse': '色块矩阵',
        'channelMonitorV2.trendView.line': '折线图',
        'channelMonitorV2.healthMode.label': '健康显示',
        'channelMonitorV2.healthMode.overall': '综合',
        'channelMonitorV2.filters.platform': '平台',
        'channelMonitorV2.filters.allPlatforms': '全部',
        'channelMonitorV2.filters.group': '分组',
        'channelMonitorV2.filters.allGroups': '全部',
        'channelMonitorV2.filters.model': '模型',
        'channelMonitorV2.filters.allModels': '全部',
        'channelMonitorV2.groupBy.label': '展示维度',
        'channelMonitorV2.groupBy.platform': '平台',
        'channelMonitorV2.groupBy.platformGroup': '平台 / 分组',
        'channelMonitorV2.groupBy.platformModel': '平台 / 模型',
        'channelMonitorV2.groupBy.platformGroupModel': '平台 / 分组 / 模型',
        'channelMonitorV2.clearFilters': '重置',
        'channelMonitorV2.tabs.aria': '明细维度',
        'channelMonitorV2.tabs.models': '模型',
        'channelMonitorV2.tabs.errors': '错误原因',
        'channelMonitorV2.tabs.users': '用户排行',
        'channelMonitorV2.details.title': '详细分析',
        'channelMonitorV2.details.description': '模型、错误分类与用户排行',
        'channelMonitorV2.details.expand': '展开详细分析',
        'channelMonitorV2.details.collapse': '收起详细分析',
        'channelMonitorV2.readiness.noTraffic': '已就绪·暂无流量',
        'channelMonitorV2.readiness.observing': '待观察',
        'channelMonitorV2.empty.title': '没有可展示的数据',
        'channelMonitorV2.empty.description': '尝试调整时间范围或筛选条件',
        'channelMonitorV2.loadFailed': '加载失败',
        'channelMonitorV2.detailLoadFailed': '明细加载失败',
        'channelMonitorV2.metrics.ttftValue': '首 Token {value}',
        'channelMonitorV2.metrics.cacheRateValue': '缓存率 {value}',
        'channelMonitorV2.metrics.tps': '每秒 Token',
        'channelMonitorV2.metrics.rpm': 'RPM',
        'channelMonitorV2.metrics.tpsDetail': 'TPM ÷ 60',
        'channelMonitorV2.metrics.rpmDetail': '每分钟请求数',
        'channelMonitorV2.matrix.emptyTitle': '当前窗口无矩阵数据',
        'channelMonitorV2.bootstrap.title': '正在补齐历史监控数据',
        'channelMonitorV2.bootstrap.description': '后台聚合中',
        'common.loading': '加载中',
        'common.refresh': '刷新',
      }
      return (map[key] || key).replace(/\{(\w+)\}/g, (_, name) => String(params?.[name] ?? ''))
    },
    }),
  }
})

const { stub } = vi.hoisted(() => ({
  stub: (name: string, template: string) => defineComponent({ name, template }),
}))

vi.mock('@/components/layout/AppLayout.vue', () => ({ default: stub('AppLayout', '<main><slot /></main>') }))
vi.mock('@/components/icons/Icon.vue', () => ({ default: stub('Icon', '<span aria-hidden="true" />') }))
vi.mock('@/components/common/LoadingSpinner.vue', () => ({ default: stub('LoadingSpinner', '<span />') }))
vi.mock('@/components/common/Select.vue', () => ({ default: stub('Select', '<div><slot /></div>') }))
vi.mock('@/features/channel-monitor-v2/FilterMultiSelect.vue', () => ({ default: stub('FilterMultiSelect', '<button type="button"><slot /></button>') }))
vi.mock('@/features/channel-monitor-v2/MonitorRankBadge.vue', () => ({ default: stub('MonitorRankBadge', '<span />') }))
vi.mock('@/features/channel-monitor-v2/MonitorTrendChart.vue', () => ({ default: stub('MonitorTrendChart', '<div data-test="trend-chart" />') }))
vi.mock('@/features/channel-monitor-v2/RelayPulseMatrix.vue', () => ({ default: stub('RelayPulseMatrix', '<div data-test="matrix" />') }))
vi.mock('@/features/channel-monitor-v2/MetricCell.vue', () => ({
  default: defineComponent({
    name: 'MetricCell',
    props: { label: String, value: String, state: String },
    setup: (props) => () => h('div', { 'data-test': 'metric-cell', 'data-state': props.state || 'neutral' }, `${props.label}|${props.value}`),
  }),
}))

import ChannelStatusV2View from '../ChannelStatusV2View.vue'

function metric(requestCount: number) {
  return {
    success_requests: requestCount,
    error_requests: 0,
    request_count: requestCount,
    token_count: 100,
    rpm: requestCount ? 1 : 0,
    tpm: requestCount ? 60 : 0,
    error_rate: 0,
    cache_rate: 0.5,
    cache_rate_numerator: 5,
    cache_rate_denominator: 10,
    ttft: { sample_count: requestCount, p50_ms: 300, p95_ms: 600, avg_ms: 350 },
    duration: { sample_count: requestCount, p50_ms: 500, p95_ms: 800, avg_ms: 550 },
  }
}

function payload(requestCount = 0, score: number | null = null) {
  const health = {
    overall: score == null ? 'unknown' : score < 50 ? 'critical' : 'healthy',
    error_rate: score == null ? 'unknown' : score < 50 ? 'critical' : 'healthy',
    ttft: score == null ? 'unknown' : 'healthy',
    cache: score == null ? 'unknown' : 'healthy',
    score,
    error_rate_score: score,
    ttft_score: score,
    cache_score: score,
    minimum_sample: 20,
  } as const
  const metrics = metric(requestCount)
  return {
    config: { refresh_interval_seconds: 300, health_thresholds: { minimum_sample: 20 }, enabled: true },
    coverage: {
      requested_start: '2026-08-18T00:00:00Z', requested_end: '2026-08-19T00:00:00Z',
      coverage_start: '2026-08-18T00:00:00Z', data_through: '2026-08-18T23:59:00Z',
      computed_at: '2026-08-18T23:59:00Z', aggregation_lag_seconds: 60, coverage_complete: true, bucket_seconds: 300,
    },
    metrics,
    health,
    trend: [],
  }
}

function matrixPayload(requestCount = 0, score: number | null = null) {
  const snapshot = payload(requestCount, score)
  return { coverage: snapshot.coverage, group_by: 'platform_group', items: [] }
}

function mountView() {
  return mount(ChannelStatusV2View)
}

describe('ChannelStatusV2View operations layout', () => {
  beforeEach(() => {
    routeQuery.query = {}
    Object.values(apiMocks).forEach((mock) => mock.mockReset())
    apiMocks.getDimensions.mockResolvedValue({ platforms: [], groups: [], models: [] })
    apiMocks.getSnapshot.mockResolvedValue(payload())
    apiMocks.getMatrix.mockResolvedValue(matrixPayload())
    apiMocks.getModels.mockResolvedValue({ items: [] })
    apiMocks.getErrors.mockResolvedValue({ items: [] })
    apiMocks.getUsers.mockResolvedValue({ items: [] })
  })

  it('defaults to 24h and keeps detail endpoints lazy until expanded', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(apiMocks.getSnapshot).toHaveBeenCalledWith(expect.objectContaining({ range: '24h' }), false, expect.any(AbortSignal))
    expect(apiMocks.getMatrix).toHaveBeenCalledWith(expect.objectContaining({ range: '24h' }), 'platform_group', false, expect.any(AbortSignal))
    expect(wrapper.find('[data-test="range-1h"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('1小时')
    expect(wrapper.text()).not.toContain('channelMonitorV2.ranges.1h')
    expect(wrapper.find('[data-test="range-90m"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="range-30d"]').exists()).toBe(false)
    expect(apiMocks.getModels).not.toHaveBeenCalled()
    expect(apiMocks.getErrors).not.toHaveBeenCalled()
    expect(apiMocks.getUsers).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="monitor-details-toggle"]').attributes('aria-expanded')).toBe('false')

    await wrapper.get('[data-test="monitor-details-toggle"]').trigger('click')
    await flushPromises()
    expect(apiMocks.getModels).toHaveBeenCalledTimes(1)

    await wrapper.get('[data-test="monitor-details-tab-errors"]').trigger('click')
    await flushPromises()
    expect(apiMocks.getErrors).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('uses neutral copy for no traffic and keeps scored critical state red', async () => {
    apiMocks.getSnapshot.mockResolvedValue(payload(0, null))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('已就绪·暂无流量')
    expect(wrapper.text()).not.toContain('100.0%')
    expect(wrapper.find('[data-test="metric-cell"]').attributes('data-state')).toBe('neutral')

    apiMocks.getSnapshot.mockResolvedValue(payload(30, 20))
    await wrapper.get('button[title="刷新"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).not.toContain('待观察')
    expect(wrapper.findAll('[data-test="metric-cell"]').some((cell) => cell.attributes('data-state') === 'critical')).toBe(true)
    wrapper.unmount()
  })

  it('preserves a valid deep-linked range', async () => {
    routeQuery.query = { range: '7d' }
    const wrapper = mountView()
    await flushPromises()
    expect(apiMocks.getSnapshot).toHaveBeenCalledWith(expect.objectContaining({ range: '7d' }), false, expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('opens detailed analysis for a legacy tab deep link', async () => {
    routeQuery.query = { tab: 'errors' }
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.get('[data-test="monitor-details-toggle"]').attributes('aria-expanded')).toBe('true')
    expect(apiMocks.getErrors).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })
})
