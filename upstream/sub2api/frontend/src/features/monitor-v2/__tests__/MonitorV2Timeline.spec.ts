import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'monitorV2.timeline.noData': '暂无探测记录',
      'monitorV2.status.operational': '运行中',
      'monitorV2.status.unavailable': '服务不可用',
    })[key] ?? key,
  }),
}))

import MonitorV2Timeline from '../MonitorV2Timeline.vue'

describe('MonitorV2Timeline', () => {
  it('renders operational probes with the fixed optimistic bar shape', () => {
    const wrapper = mount(MonitorV2Timeline, {
      props: {
        points: [{
          bucket_start: '2026-07-30T08:00:00Z',
          status: 'operational',
          latency_ms: null,
        }],
      },
    })

    const bar = wrapper.find('[role="img"] span[title*="运行中"]')
    expect(bar.exists()).toBe(true)
    expect(bar.classes()).toContain('bg-emerald-400')
    expect(bar.classes()).toContain('h-6')
  })

  it('uses the unavailable color for an unavailable service point', () => {
    const wrapper = mount(MonitorV2Timeline, {
      props: {
        points: [
          {
            bucket_start: '2026-07-30T08:00:00Z',
            status: 'unavailable',
            latency_ms: 12_000,
          },
          {
            bucket_start: '2026-07-30T09:00:00Z',
            status: 'operational',
            latency_ms: null,
          },
        ],
      },
    })

    const bars = wrapper.findAll('[role="img"] span[title]')
    expect(bars).toHaveLength(2)
    expect(bars[0].classes()).toContain('bg-red-400')
    expect(bars[0].classes()).toContain('h-6')
    expect(bars[0].attributes('title')).toContain('服务不可用')
    expect(bars[0].attributes('title')).not.toContain('失败')
  })

  it('keeps dense timelines inside the card on narrow screens', () => {
    const wrapper = mount(MonitorV2Timeline, {
      props: {
        points: Array.from({ length: 64 }, (_, index) => ({
          bucket_start: `2026-07-30T${String(index % 24).padStart(2, '0')}:00:00Z`,
          status: 'operational' as const,
          latency_ms: null,
        })),
      },
    })

    const timeline = wrapper.find('[role="img"] > div')
    expect(timeline.classes()).toContain('min-w-0')
    expect(timeline.classes()).toContain('overflow-hidden')
    expect(wrapper.find('[role="img"] span').classes()).not.toContain('min-w-1')
  })
})
