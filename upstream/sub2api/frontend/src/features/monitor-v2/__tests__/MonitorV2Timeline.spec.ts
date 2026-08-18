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
  it('renders successful probes with the fixed optimistic bar shape', () => {
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
    expect(bar.classes()).toContain('bg-teal-500')
    expect(bar.attributes('style')).toContain('height: 75%')
  })

  it('uses the same optimistic bar shape for failed and empty buckets', () => {
    const wrapper = mount(MonitorV2Timeline, {
      props: {
        points: [
          {
            bucket_start: '2026-07-30T08:00:00Z',
            state: 'available',
            value: 0,
            success_count: 0,
            eligible_count: 1,
            latency_ms: 12_000,
          },
          {
            bucket_start: '2026-07-30T09:00:00Z',
            state: 'insufficient_data',
            value: null,
            success_count: 0,
            eligible_count: 0,
            latency_ms: null,
          },
        ],
      },
    })

    const bars = wrapper.findAll('[role="img"] span[title]')
    expect(bars).toHaveLength(2)
    for (const bar of bars) {
      expect(bar.classes()).toContain('bg-teal-500')
      expect(bar.attributes('style')).toContain('height: 75%')
    }
  })
})
