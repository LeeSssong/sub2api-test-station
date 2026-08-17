import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsOpenAISchedulerExperienceCard from '../OpsOpenAISchedulerExperienceCard.vue'

const mockGetOpenAISchedulerExperience = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getOpenAISchedulerExperience: (...args: any[]) => mockGetOpenAISchedulerExperience(...args),
  },
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, any>) => {
        if (key.endsWith('.ratio') && params) return `${params.numerator} / ${params.denominator}`
        if (key.endsWith('.sampleSize') && params) return `sample ${params.count}`
        if (key.endsWith('.p95') && params) return `P95 ${params.value}`
        return key
      },
    }),
  }
})

const EmptyStateStub = defineComponent({
  name: 'EmptyState',
  props: {
    title: { type: String, default: '' },
    description: { type: String, default: '' },
  },
  template: '<div class="empty-state">{{ title }}|{{ description }}</div>',
})

const rate = (numerator: number, denominator: number, value: number | null, status = 'ok') => ({
  numerator,
  denominator,
  value,
  status,
})

const sampleResponse = {
  start_time: '2026-08-17T08:00:00Z',
  end_time: '2026-08-17T09:00:00Z',
  generated_at: '2026-08-17T09:00:01Z',
  latest_event_at: '2026-08-17T08:59:58Z',
  sample_size: 10,
  metrics: {
    auto_recovery_rate: rate(8, 10, 0.8),
    average_attempts: { sample_size: 10, value: 1.4, p95: 2, status: 'ok' },
    repeated_bad_account_rate: rate(1, 10, 0.1),
    retry_budget_exhausted_rate: rate(2, 10, 0.2),
    sticky_kept_rate: rate(6, 10, 0.6),
    sticky_escape_rate: rate(4, 10, 0.4),
    top_k_filtered_rate: rate(15, 30, 0.5),
    ttft_report_eligible_rate: rate(3, 10, 0.3),
  },
}

function mountCard(props: Record<string, unknown> = {}) {
  return mount(OpsOpenAISchedulerExperienceCard, {
    props: {
      timeRange: '1h',
      platformFilter: 'openai',
      groupIdFilter: 7,
      refreshToken: 0,
      ...props,
    },
    global: {
      stubs: {
        EmptyState: EmptyStateStub,
      },
    },
  })
}

describe('OpsOpenAISchedulerExperienceCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('loads the shared dashboard filters and renders rates, attempts, ratios, P95, and freshness', async () => {
    mockGetOpenAISchedulerExperience.mockResolvedValue(sampleResponse)

    const wrapper = mountCard()
    await flushPromises()

    expect(mockGetOpenAISchedulerExperience).toHaveBeenCalledWith(
      {
        time_range: '1h',
        platform: 'openai',
        group_id: 7,
      },
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    expect(wrapper.text()).toContain('80.0%')
    expect(wrapper.text()).toContain('1.40')
    expect(wrapper.text()).toContain('P95 2')
    expect(wrapper.text()).toContain('8 / 10')
    expect(wrapper.text()).toContain('sample 10')
    expect(wrapper.text()).toContain('2026')
    const runtimeWindow = wrapper.get('[data-test="scheduler-runtime-window"]').text()
    expect(runtimeWindow).toContain(new Date(sampleResponse.start_time).toLocaleString())
    expect(runtimeWindow).toContain(new Date(sampleResponse.end_time).toLocaleString())
  })

  it('keeps numerator and denominator visible for insufficient data without showing a misleading rate', async () => {
    mockGetOpenAISchedulerExperience.mockResolvedValue({
      ...sampleResponse,
      sample_size: 2,
      metrics: {
        ...sampleResponse.metrics,
        auto_recovery_rate: rate(1, 2, null, 'insufficient_data'),
      },
    })

    const wrapper = mountCard()
    await flushPromises()

    const metric = wrapper.get('[data-metric="auto-recovery"]')
    expect(metric.text()).toContain('1 / 2')
    expect(metric.text()).toContain('admin.ops.openaiSchedulerExperience.status.insufficientData')
    expect(metric.text()).not.toContain('50.0%')
  })

  it('shows a clear no-data state when the runtime ledger has no logical requests', async () => {
    const noDataRate = rate(0, 0, null, 'no_data')
    mockGetOpenAISchedulerExperience.mockResolvedValue({
      ...sampleResponse,
      latest_event_at: null,
      sample_size: 0,
      metrics: {
        auto_recovery_rate: noDataRate,
        average_attempts: { sample_size: 0, value: null, p95: null, status: 'no_data' },
        repeated_bad_account_rate: noDataRate,
        retry_budget_exhausted_rate: noDataRate,
        sticky_kept_rate: noDataRate,
        sticky_escape_rate: noDataRate,
        top_k_filtered_rate: noDataRate,
        ttft_report_eligible_rate: noDataRate,
      },
    })

    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.find('.empty-state').exists()).toBe(true)
    expect(wrapper.text()).toContain('admin.ops.openaiSchedulerExperience.empty')
  })

  it('contains failures inside the card and retries without removing dashboard siblings', async () => {
    mockGetOpenAISchedulerExperience
      .mockRejectedValueOnce(new Error('scheduler metrics unavailable'))
      .mockResolvedValueOnce(sampleResponse)

    const Host = defineComponent({
      components: { OpsOpenAISchedulerExperienceCard },
      template: `
        <div>
          <div data-test="sibling">throughput stays visible</div>
          <OpsOpenAISchedulerExperienceCard time-range="1h" :refresh-token="0" />
        </div>
      `,
    })
    const wrapper = mount(Host, {
      global: { stubs: { EmptyState: EmptyStateStub } },
    })
    await flushPromises()

    expect(wrapper.get('[data-test="sibling"]').text()).toBe('throughput stays visible')
    expect(wrapper.text()).toContain('scheduler metrics unavailable')

    await wrapper.get('[data-test="retry"]').trigger('click')
    await flushPromises()

    expect(mockGetOpenAISchedulerExperience).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('80.0%')
    expect(wrapper.get('[data-test="sibling"]').exists()).toBe(true)
  })

  it('uses a one-column-first bounded grid so a 390px viewport does not require horizontal scrolling', async () => {
    mockGetOpenAISchedulerExperience.mockResolvedValue(sampleResponse)

    const wrapper = mountCard()
    await flushPromises()

    expect(wrapper.get('[data-test="scheduler-experience-card"]').classes()).toEqual(
      expect.arrayContaining(['min-w-0', 'overflow-hidden'])
    )
    const grid = wrapper.get('[data-test="scheduler-metrics-grid"]')
    expect(grid.classes()).toEqual(expect.arrayContaining(['grid-cols-1', 'sm:grid-cols-2', 'xl:grid-cols-4']))
    expect(wrapper.findAll('[data-metric]').every((metric) => metric.classes().includes('min-w-0'))).toBe(true)
  })
})
