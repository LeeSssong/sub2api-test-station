import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsDashboard from '../OpsDashboard.vue'

const {
  mockGetAdvancedSettings,
  mockGetMetricThresholds,
  mockGetDashboardSnapshotV2,
  mockGetThroughputTrend,
  mockGetLatencyHistogram,
  mockGetErrorDistribution,
  mockGetOpenAISchedulerExperience,
  mockRouterReplace,
  route
} = vi.hoisted(() => ({
  mockGetAdvancedSettings: vi.fn(),
  mockGetMetricThresholds: vi.fn(),
  mockGetDashboardSnapshotV2: vi.fn(),
  mockGetThroughputTrend: vi.fn(),
  mockGetLatencyHistogram: vi.fn(),
  mockGetErrorDistribution: vi.fn(),
  mockGetOpenAISchedulerExperience: vi.fn(),
  mockRouterReplace: vi.fn(),
  route: { query: {} as Record<string, string> }
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getAdvancedSettings: (...args: unknown[]) => mockGetAdvancedSettings(...args),
    getMetricThresholds: (...args: unknown[]) => mockGetMetricThresholds(...args),
    getDashboardSnapshotV2: (...args: unknown[]) => mockGetDashboardSnapshotV2(...args),
    getThroughputTrend: (...args: unknown[]) => mockGetThroughputTrend(...args),
    getLatencyHistogram: (...args: unknown[]) => mockGetLatencyHistogram(...args),
    getErrorDistribution: (...args: unknown[]) => mockGetErrorDistribution(...args),
    getOpenAISchedulerExperience: (...args: unknown[]) => mockGetOpenAISchedulerExperience(...args)
  },
  default: {
    getAdvancedSettings: (...args: unknown[]) => mockGetAdvancedSettings(...args),
    getMetricThresholds: (...args: unknown[]) => mockGetMetricThresholds(...args),
    getDashboardSnapshotV2: (...args: unknown[]) => mockGetDashboardSnapshotV2(...args),
    getThroughputTrend: (...args: unknown[]) => mockGetThroughputTrend(...args),
    getLatencyHistogram: (...args: unknown[]) => mockGetLatencyHistogram(...args),
    getErrorDistribution: (...args: unknown[]) => mockGetErrorDistribution(...args),
    getOpenAISchedulerExperience: (...args: unknown[]) => mockGetOpenAISchedulerExperience(...args)
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: vi.fn() }),
  useAdminSettingsStore: () => ({
    opsMonitoringEnabled: true,
    opsQueryModeDefault: 'auto',
    fetch: vi.fn().mockResolvedValue(undefined)
  })
}))

vi.mock('vue-router', () => ({
  useRoute: () => route,
  useRouter: () => ({ replace: mockRouterReplace })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('@vueuse/core', () => ({
  useDebounceFn: (fn: (...args: unknown[]) => unknown) => fn,
  useIntervalFn: () => ({ pause: vi.fn(), resume: vi.fn() })
}))

const DashboardHeaderStub = defineComponent({
  name: 'OpsDashboardHeader',
  emits: ['update:time-range', 'update:custom-time-range'],
  template: '<div data-test="ops-dashboard-header" />'
})

const TokenStatsStub = defineComponent({
  name: 'OpsOpenAITokenStatsCard',
  template: '<div data-test="token-stats-card">token stats</div>'
})

const schedulerResponse = {
  start_time: '2026-08-17T08:00:00Z',
  end_time: '2026-08-17T09:00:00Z',
  generated_at: '2026-08-17T09:00:01Z',
  latest_event_at: null,
  sample_size: 1,
  metrics: {
    auto_recovery_rate: { numerator: 1, denominator: 1, value: 1, status: 'ok' },
    average_attempts: { sample_size: 1, value: 1, p95: 1, status: 'ok' },
    repeated_bad_account_rate: { numerator: 0, denominator: 1, value: 0, status: 'ok' },
    retry_budget_exhausted_rate: { numerator: 0, denominator: 0, value: null, status: 'no_data' },
    sticky_kept_rate: { numerator: 0, denominator: 0, value: null, status: 'no_data' },
    sticky_escape_rate: { numerator: 0, denominator: 0, value: null, status: 'no_data' },
    top_k_filtered_rate: { numerator: 0, denominator: 0, value: null, status: 'no_data' },
    ttft_report_eligible_rate: { numerator: 0, denominator: 0, value: null, status: 'no_data' }
  }
}

const advancedSettings = (displayOpenAITokenStats: boolean) => ({
  data_retention: { cleanup_enabled: false, cleanup_schedule: '', error_log_retention_days: 0, minute_metrics_retention_days: 0, hourly_metrics_retention_days: 0 },
  aggregation: { aggregation_enabled: false },
  openai_account_quota_auto_pause: { default_threshold_5h: 0, default_threshold_7d: 0 },
  ignore_count_tokens_errors: false,
  ignore_context_canceled: false,
  ignore_no_available_accounts: false,
  ignore_invalid_api_key_errors: false,
  ignore_insufficient_balance_errors: false,
  display_openai_token_stats: displayOpenAITokenStats,
  display_alert_events: false,
  auto_refresh_enabled: false,
  auto_refresh_interval_seconds: 30
})

function mountDashboard() {
  return mount(OpsDashboard, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: { template: '<div><slot /></div>' },
        OpsDashboardHeader: DashboardHeaderStub,
        OpsDashboardSkeleton: true,
        OpsConcurrencyCard: true,
        OpsErrorDetailModal: true,
        OpsErrorDistributionChart: true,
        OpsErrorDetailsModal: true,
        OpsErrorTrendChart: true,
        OpsLatencyChart: true,
        OpsThroughputTrendChart: true,
        OpsSwitchRateTrendChart: true,
        OpsAlertEventsCard: true,
        OpsOpenAITokenStatsCard: TokenStatsStub,
        OpsSystemLogTable: true,
        OpsRequestDetailsModal: true,
        OpsSettingsDialog: true,
        OpsAlertRulesCard: true
      }
    }
  })
}

describe('OpsDashboard scheduler experience integration', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    route.query = {}
    mockRouterReplace.mockResolvedValue(undefined)
    mockGetMetricThresholds.mockResolvedValue({})
    mockGetDashboardSnapshotV2.mockResolvedValue({ overview: null, throughput_trend: { points: [] }, error_trend: { points: [] } })
    mockGetThroughputTrend.mockResolvedValue({ points: [] })
    mockGetLatencyHistogram.mockResolvedValue({ buckets: [] })
    mockGetErrorDistribution.mockResolvedValue({ total: 0, items: [] })
    mockGetOpenAISchedulerExperience.mockResolvedValue(schedulerResponse)
  })

  it('mounts the scheduler card after OpenAI token stats when token stats are enabled', async () => {
    mockGetAdvancedSettings.mockResolvedValue(advancedSettings(true))

    const wrapper = mountDashboard()
    await flushPromises()

    const tokenStats = wrapper.get('[data-test="token-stats-card"]').element
    const scheduler = wrapper.get('[data-test="scheduler-experience-card"]').element
    expect(tokenStats.compareDocumentPosition(scheduler) & Node.DOCUMENT_POSITION_FOLLOWING).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
  })

  it('keeps scheduler experience visible when the OpenAI token stats setting is disabled', async () => {
    mockGetAdvancedSettings.mockResolvedValue(advancedSettings(false))

    const wrapper = mountDashboard()
    await flushPromises()

    expect(wrapper.find('[data-test="token-stats-card"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="scheduler-experience-card"]').exists()).toBe(true)
  })

  it('passes exact custom range timestamps through to scheduler experience', async () => {
    mockGetAdvancedSettings.mockResolvedValue(advancedSettings(false))
    const startTime = '2026-08-17T08:00:00.000Z'
    const endTime = '2026-08-17T09:30:00.000Z'

    const wrapper = mountDashboard()
    await flushPromises()
    mockGetOpenAISchedulerExperience.mockClear()

    const header = wrapper.findComponent(DashboardHeaderStub)
    header.vm.$emit('update:time-range', 'custom')
    header.vm.$emit('update:custom-time-range', startTime, endTime)
    await flushPromises()

    expect(mockGetOpenAISchedulerExperience).toHaveBeenLastCalledWith(
      { start_time: startTime, end_time: endTime },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
  })
})
