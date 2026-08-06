import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'monitorV2.timeline.noData': '暂无探测记录',
      'monitorV2.timeline.probeUnavailable': '探测完成（当前无可用模型）',
      'monitorV2.timeline.success': '成功',
      'monitorV2.timeline.failed': '失败',
    })[key] ?? key,
  }),
}))

import MonitorV2Timeline from '../MonitorV2Timeline.vue'

describe('MonitorV2Timeline', () => {
  it('renders unavailable probes as green successful short bars', () => {
    const wrapper = mount(MonitorV2Timeline, {
      props: {
        points: [{
          bucket_start: '2026-07-30T08:00:00Z',
          state: 'available',
          value: 100,
          success_count: 1,
          eligible_count: 1,
          latency_ms: null,
        }],
      },
    })

    const bar = wrapper.find('[role="img"] span[title*="成功"]')
    expect(bar.exists()).toBe(true)
    expect(bar.classes()).toContain('bg-emerald-500')
    expect(bar.attributes('style')).toContain('height: 40%')
  })

  it('keeps an empty bucket neutral instead of reporting a completed probe', () => {
    const wrapper = mount(MonitorV2Timeline, {
      props: {
        points: [{
          bucket_start: '2026-07-30T08:00:00Z',
          state: 'insufficient_data',
          value: null,
          success_count: 0,
          eligible_count: 0,
          latency_ms: null,
        }],
      },
    })

    const bar = wrapper.find('[role="img"] span[title*="暂无探测记录"]')
    expect(bar.exists()).toBe(true)
    expect(bar.classes()).toContain('bg-gray-300')
    expect(bar.attributes('style')).toContain('height: 20%')
  })
})
