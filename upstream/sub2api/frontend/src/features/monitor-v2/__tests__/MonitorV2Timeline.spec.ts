import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'monitorV2.timeline.noData': '暂无探测记录',
      'monitorV2.timeline.noDataBucket': '无探测数据',
      'monitorV2.timeline.noDataBucketLabel': '无探测数据桶',
      'monitorV2.status.operational': '运行中',
      'monitorV2.status.unavailable': '服务不可用',
    })[key] ?? key,
  }),
}))

import MonitorV2Timeline from '../MonitorV2Timeline.vue'

describe('MonitorV2Timeline', () => {
  it('shows a custom time-point tooltip and lifts the hovered operational bar', async () => {
    const wrapper = mount(MonitorV2Timeline, {
      props: {
        points: [{
          bucket_start: '2026-07-30T08:00:00Z',
          status: 'operational',
          latency_ms: null,
        }],
      },
    })

    const bar = wrapper.get('[data-timeline-point="0"]')
    const root = wrapper.get('[data-timeline-root]')
    expect(root.attributes('role')).toBe('group')
    expect(root.attributes('aria-hidden')).toBeUndefined()
    expect(bar.attributes('role')).toBe('img')
    expect(bar.attributes('aria-label')).toContain('2026-07-30')
    expect(bar.element.closest('[aria-hidden="true"]')).toBeNull()
    expect(bar.classes()).toContain('bg-emerald-400')
    expect(bar.classes()).toContain('h-5')
    expect(bar.classes()).toContain('w-[6px]')
    expect(bar.classes()).toContain('hover:-translate-y-1')

    await bar.trigger('mouseenter')

    const tooltip = wrapper.get('[data-timeline-tooltip]')
    expect(wrapper.get('[data-timeline-tooltip-row]').element.contains(tooltip.element)).toBe(true)
    expect(tooltip.classes()).not.toContain('top-[4.25rem]')
    expect(tooltip.classes()).toContain('min-w-[196px]')
    expect(tooltip.find('[data-timeline-tooltip-arrow]').classes()).toContain('-top-1')
    expect(tooltip.text()).toContain('UP')
    expect(tooltip.text()).toContain('2026-07-30')
    expect(tooltip.text()).toContain('运行中')

    await bar.trigger('blur')
    expect(wrapper.find('[data-timeline-tooltip]').exists()).toBe(false)
    await bar.trigger('focus')
    expect(wrapper.get('[data-timeline-tooltip]').text()).toContain('UP')
  })

  it('uses the unavailable color and DOWN tooltip for an unavailable service point', async () => {
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

    const bars = wrapper.findAll('[data-timeline-point]')
    expect(bars).toHaveLength(2)
    expect(bars[0].classes()).toContain('bg-red-400')
    expect(bars[0].classes()).toContain('h-5')

    await bars[0].trigger('mouseenter')
    expect(wrapper.get('[data-timeline-tooltip]').text()).toContain('DOWN')
    expect(wrapper.get('[data-timeline-tooltip]').text()).toContain('服务不可用')
  })

  it('keeps fixed-size dense timelines in a controlled inner scroller on narrow screens', () => {
    const wrapper = mount(MonitorV2Timeline, {
      props: {
        points: Array.from({ length: 64 }, (_, index) => ({
          bucket_start: `2026-07-30T${String(index % 24).padStart(2, '0')}:00:00Z`,
          status: 'operational' as const,
          latency_ms: null,
        })),
      },
    })

    const scroll = wrapper.get('[data-timeline-scroll]')
    const track = wrapper.get('[data-timeline-track]')
    const bar = wrapper.get('[data-timeline-point="0"]')
    expect(scroll.classes()).toContain('max-w-full')
    expect(scroll.classes()).toContain('overflow-x-auto')
    expect(track.classes()).toContain('min-w-max')
    expect(bar.classes()).toContain('shrink-0')
    expect(bar.classes()).toContain('w-[6px]')
    expect(track.classes()).toContain('gap-[5px]')
  })

  it('clamps the tooltip inside the visible timeline after horizontal scrolling', async () => {
    const wrapper = mount(MonitorV2Timeline, {
      props: {
        points: [
          { bucket_start: '2026-07-30T08:00:00Z', status: 'operational', latency_ms: 320 },
          { bucket_start: '2026-07-30T09:00:00Z', status: 'unavailable', latency_ms: 12_000 },
        ],
      },
    })
    const point = wrapper.get('[data-timeline-point="1"]')
    let pointLeft = 600
    vi.spyOn(wrapper.element, 'getBoundingClientRect').mockReturnValue({ left: 0, width: 320 } as DOMRect)
    vi.spyOn(point.element, 'getBoundingClientRect').mockImplementation(() => ({ left: pointLeft, width: 5 } as DOMRect))

    await point.trigger('mouseenter')
    expect(wrapper.get('[data-timeline-tooltip]').attributes('style')).toContain('left: 222px')
    expect(wrapper.get('[data-timeline-tooltip]').text()).toContain('2026-07-30 17:00:00')
    expect(wrapper.get('[data-timeline-tooltip]').text()).toContain('DOWN')

    pointLeft = -80
    await wrapper.get('[data-timeline-scroll]').trigger('scroll')
    expect(wrapper.get('[data-timeline-tooltip]').attributes('style')).toContain('left: 98px')
  })

  it('marks buckets without a probe result as explicit no-data buckets', async () => {
    const wrapper = mount(MonitorV2Timeline, {
      props: {
        points: [{
          bucket_start: '2026-07-30T10:00:00Z',
          status: 'unavailable',
          latency_ms: null,
        }],
      },
    })

    const point = wrapper.get('[data-timeline-point="0"]')
    expect(point.attributes('data-timeline-point-state')).toBe('no-data')
    expect(point.attributes('aria-label')).toContain('无探测数据桶')
    expect(point.classes()).toContain('border-dashed')

    await point.trigger('mouseenter')
    const tooltip = wrapper.get('[data-timeline-tooltip]')
    expect(tooltip.text()).toContain('NO DATA')
    expect(tooltip.text()).toContain('无探测数据')
    expect(tooltip.text()).not.toContain('DOWN')
  })
})
