import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { getMonitorV2Snapshot } = vi.hoisted(() => ({
  getMonitorV2Snapshot: vi.fn(),
}))

vi.mock('../api', async () => {
  const actual = await vi.importActual<typeof import('../api')>('../api')
  return {
    ...actual,
    getMonitorV2Snapshot,
  }
})

const messages: Record<string, string> = {
  'monitorV2.title': '服务监控',
  'monitorV2.description': '查看当前可见分组的实时状态与调用质量',
  'monitorV2.updatedAt': '更新于 {time}',
  'monitorV2.refresh': '刷新',
  'monitorV2.overall.operational': '全部服务正常',
  'monitorV2.overall.unavailable': '服务不可用',
  'monitorV2.overall.noData': '等待监控数据',
  'monitorV2.window.24h': '24 小时',
  'monitorV2.window.7d': '7 天',
  'monitorV2.window.30d': '30 天',
  'monitorV2.status.operational': '运行中',
  'monitorV2.status.unavailable': '服务不可用',
  'monitorV2.availability': '可用性：{value}%',
  'monitorV2.availabilityNoData': '暂无可用率数据',
  'monitorV2.metric.ttft': '首字速度：',
  'monitorV2.metric.availability': '可用性：',
  'monitorV2.metric.averageLatency': '平均耗时：',
  'monitorV2.metric.samples': '基于 {count} 次调用',
  'monitorV2.metric.noSamples': '暂无调用样本',
  'monitorV2.metric.insufficient_data': '样本不足',
  'monitorV2.empty.title': '暂无可见分组',
  'monitorV2.empty.description': '管理员尚未开放可展示的服务分组。',
  'monitorV2.notes.metrics': '指标按所选时间范围汇总，样本不足时不显示推测值。',
  'monitorV2.notes.privacy': '普通用户仅展示公开分组，管理员展示全部启用分组；不包含账号、用户或请求内容。',
  'monitorV2.timeline.noData': '该时段暂无探测记录',
  'monitorV2.timeline.current': '当前',
  'monitorV2.freshness.latestProbe': '探测于 {time}',
  'monitorV2.freshness.noProbe': '暂无最新探测',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string | number>) => {
        let value = messages[key] ?? key
        for (const [name, replacement] of Object.entries(params ?? {})) {
          value = value.replace(`{${name}}`, String(replacement))
        }
        return value
      },
    }),
  }
})

import MonitorV2View from '../MonitorV2View.vue'
import type { MonitorV2Snapshot } from '../types'

const snapshot: MonitorV2Snapshot = {
  contract_version: '7',
  window: '7d',
  refresh_interval_seconds: 60,
  generated_at: '2026-07-29T12:00:00Z',
  groups: [
    {
      id: 7,
      name: 'OpenAI 组',
      platform: 'openai',
      rate_multiplier: 0.2,
      peak_rate_enabled: false,
      peak_start: '',
      peak_end: '',
      peak_rate_multiplier: 1,
      status: 'operational',
      availability: { state: 'available', value: 100, sample_count: 28 },
      ttft: { state: 'available', value: 420, sample_count: 9842 },
      average_latency: { state: 'available', value: 10000, sample_count: 9842 },
      source_updated_at: '2026-07-29T11:59:00Z',
      timeline: [
        {
          bucket_start: '2026-07-29T06:00:00Z',
          status: 'operational',
          latency_ms: 1320,
        },
      ],
    },
    {
      id: 8,
      name: 'Claude 公开组',
      platform: 'anthropic',
      rate_multiplier: 0.3,
      peak_rate_enabled: false,
      peak_start: '',
      peak_end: '',
      peak_rate_multiplier: 1,
      status: 'unavailable',
      availability: { state: 'available', value: 0, sample_count: 28 },
      ttft: { state: 'insufficient_data', value: null, sample_count: 0 },
      average_latency: { state: 'insufficient_data', value: null, sample_count: 0 },
      timeline: [],
    },
  ],
}

function mountView(initialSnapshot = snapshot) {
  const wrapper = mount(MonitorV2View, {
    props: { initialSnapshot },
    global: {
      stubs: {
        AppLayout: { template: '<main><slot /></main>' },
        CodexRadarRecommendations: true,
        Icon: { template: '<span aria-hidden="true" />' },
      },
    },
  })
  mountedWrappers.push(wrapper)
  return wrapper
}

const mountedWrappers: Array<ReturnType<typeof mount>> = []

describe('MonitorV2View', () => {
  beforeEach(() => {
    getMonitorV2Snapshot.mockReset()
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'visible',
    })
  })

  afterEach(() => {
    mountedWrappers.splice(0).forEach((wrapper) => wrapper.unmount())
    vi.useRealTimers()
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: 'visible',
    })
  })

  function setVisibility(state: 'hidden' | 'visible') {
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      value: state,
    })
    document.dispatchEvent(new Event('visibilitychange'))
  }

  it('renders native availability, latency metrics and the group multiplier', () => {
    const wrapper = mountView()

    expect(wrapper.get('[data-test="monitor-v2-page"]').classes()).toContain('max-w-[1500px]')
    expect(wrapper.text()).not.toContain('查看当前可见分组的实时状态与调用质量')
    expect(wrapper.text()).toContain('OpenAI 组')
    expect(wrapper.text()).toContain('可用性：100%')
    expect(wrapper.text()).toContain('0.2×')
    expect(wrapper.text()).toContain('首字速度：420 ms')
    expect(wrapper.text()).toContain('平均耗时：10 s')
    expect(wrapper.text()).toContain('探测于')
    expect(wrapper.text()).toContain('运行中')
    expect(wrapper.text()).toContain('服务不可用')
    for (const forbidden of [
      '9,842 / 9,910',
      '个模型',
      '查看模型',
      '收起模型',
      'gpt-5.4',
      'TTFT P95',
      '总延迟 P95',
      '880 ms',
      '2.4 s',
      '旗舰',
      'P95 表示',
      '指标按所选时间范围汇总',
      '普通用户仅展示公开分组',
      '状态：运行中',
      '40%',
      '99.31%',
      '缓存命中率',
      '有效调用',
      '真实请求',
      '服务波动',
    ]) {
      expect(wrapper.text()).not.toContain(forbidden)
    }
    const flagshipCard = wrapper.get('[data-test="monitor-group-7"]')
    expect(flagshipCard.get('[data-test="monitor-group-status"]').text()).toBe('运行中')
    expect(flagshipCard.get('[data-test="monitor-group-status"]').text()).not.toContain('可用性')
    expect(flagshipCard.classes()).toContain('transition-all')
    expect(flagshipCard.classes()).toContain('hover:bg-emerald-500/10')
    expect(flagshipCard.classes()).toContain('hover:-translate-y-0.5')
    expect(flagshipCard.classes()).toContain('focus-within:bg-emerald-500/10')
    expect(wrapper.text()).toContain('可用性：0%')
    const multiplier = wrapper.get('[data-test="monitor-rate-multiplier"]')
    expect(multiplier.text()).toContain('0.2×')
    expect(multiplier.classes()).toContain('bg-emerald-500/15')
    expect(multiplier.classes()).toContain('text-base')
    const unconfiguredCard = wrapper.findAll('article')[1]
    expect(unconfiguredCard.text()).not.toMatch(/\b0 ms\b/)
  })

  it('reloads the selected window and replaces the snapshot', async () => {
    getMonitorV2Snapshot.mockResolvedValue({
      ...snapshot,
      window: '24h',
      groups: [{ ...snapshot.groups[0], name: '24 小时结果' }],
    })
    const wrapper = mountView()

    await wrapper.get('[data-test="monitor-window-24h"]').trigger('click')
    await flushPromises()

    expect(getMonitorV2Snapshot).toHaveBeenCalledWith('24h', expect.any(AbortSignal))
    expect(wrapper.text()).toContain('24 小时结果')
  })

  it('does not render a manual refresh control', () => {
    expect(mountView().find('[data-test="monitor-refresh"]').exists()).toBe(false)
  })

  it('refreshes using the administrator interval', async () => {
    vi.useFakeTimers()
    getMonitorV2Snapshot.mockResolvedValue(snapshot)
    const wrapper = mountView()

    await vi.advanceTimersByTimeAsync(60_000)

    expect(getMonitorV2Snapshot).toHaveBeenCalledWith('7d', expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('does not schedule an interval when administrator disables refresh', async () => {
    vi.useFakeTimers()
    mountView({ ...snapshot, refresh_interval_seconds: 0 })

    await vi.advanceTimersByTimeAsync(600_000)

    expect(getMonitorV2Snapshot).not.toHaveBeenCalled()
  })

  it('pauses while hidden and refreshes once when visible again', async () => {
    vi.useFakeTimers()
    getMonitorV2Snapshot.mockResolvedValue(snapshot)
    const wrapper = mountView()

    setVisibility('hidden')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(getMonitorV2Snapshot).not.toHaveBeenCalled()

    setVisibility('visible')
    await flushPromises()

    expect(getMonitorV2Snapshot).toHaveBeenCalledTimes(1)
    expect(getMonitorV2Snapshot).toHaveBeenCalledWith('7d', expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('does not refresh after unmount', async () => {
    vi.useFakeTimers()
    getMonitorV2Snapshot.mockResolvedValue(snapshot)
    const wrapper = mountView()
    wrapper.unmount()

    await vi.advanceTimersByTimeAsync(60_000)

    expect(getMonitorV2Snapshot).not.toHaveBeenCalled()
  })

  it('applies a changed interval from the periodic response', async () => {
    vi.useFakeTimers()
    getMonitorV2Snapshot
      .mockResolvedValueOnce({ ...snapshot, refresh_interval_seconds: 300 })
      .mockResolvedValue(snapshot)
    const wrapper = mountView()

    await vi.advanceTimersByTimeAsync(60_000)
    expect(getMonitorV2Snapshot).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(60_000)
    expect(getMonitorV2Snapshot).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(240_000)
    expect(getMonitorV2Snapshot).toHaveBeenCalledTimes(2)
    expect(getMonitorV2Snapshot).toHaveBeenLastCalledWith('7d', expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('retries a failed periodic GET without switching to the fallback page', async () => {
    vi.useFakeTimers()
    getMonitorV2Snapshot
      .mockRejectedValueOnce(new Error('temporary read failure'))
      .mockResolvedValueOnce(snapshot)
    const wrapper = mountView()

    await vi.advanceTimersByTimeAsync(60_000)
    await flushPromises()
    expect(getMonitorV2Snapshot).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('fatal')).toBeUndefined()

    await vi.advanceTimersByTimeAsync(4_999)
    expect(getMonitorV2Snapshot).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    await flushPromises()
    expect(getMonitorV2Snapshot).toHaveBeenCalledTimes(2)
    expect(wrapper.emitted('fatal')).toBeUndefined()
  })

  it('renders an instructive empty state', () => {
    const wrapper = mountView({
      ...snapshot,
      groups: [],
    })

    expect(wrapper.text()).toContain('暂无可见分组')
    expect(wrapper.text()).toContain('管理员尚未开放可展示的服务分组。')
  })
})
