import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => ({
      'monitorV2.status.operational': '运行中',
      'monitorV2.status.unavailable': '服务不可用',
      'monitorV2.metric.availability': '可用性：',
      'monitorV2.metric.ttft': '首字速度：',
      'monitorV2.metric.averageLatency': '平均耗时：',
      'monitorV2.freshness.latestProbe': '探测于',
      'monitorV2.freshness.noProbe': '暂无最新探测',
      'monitorV2.timeline.label': '时间线',
    })[key] ?? key,
  }),
}))

import MonitorV2GroupCard from '../MonitorV2GroupCard.vue'

describe('MonitorV2GroupCard', () => {
  it('renders a compact competitor-style service line with metrics and timeline slot', () => {
    const wrapper = mount(MonitorV2GroupCard, {
      props: {
        group: {
          id: 7,
          name: 'GPT-Pro',
          platform: 'openai',
          rate_multiplier: 0.3,
          peak_rate_enabled: false,
          peak_start: '',
          peak_end: '',
          peak_rate_multiplier: 1,
          status: 'operational',
          availability: { state: 'available', value: 100, sample_count: 28 },
          ttft: { state: 'available', value: 2220, sample_count: 28 },
          average_latency: { state: 'available', value: 3810, sample_count: 28 },
          source_updated_at: '2026-08-21T01:54:00Z',
          timeline: [{ bucket_start: '2026-08-21T01:00:00Z', status: 'operational', latency_ms: 2220 }],
        },
      },
      global: {
        stubs: {
          MonitorV2Timeline: { template: '<div data-test="timeline-slot" />' },
        },
      },
    })

    const article = wrapper.get('[data-test="monitor-group-7"]')
    expect(article.attributes('data-monitor-layout')).toBe('service-line')
    expect(article.classes()).toContain('dark:bg-[#0b1220]')
    expect(article.classes()).toContain('px-4')
    expect(wrapper.get('header').classes()).toContain('lg:grid-cols-[minmax(360px,0.95fr)_minmax(0,1.35fr)]')
    expect(wrapper.get('h2').classes()).toContain('text-lg')
    expect(wrapper.get('[data-test=monitor-availability-badge]').classes()).toContain('text-base')
    expect(wrapper.get('[data-test=monitor-group-status]').classes()).toContain('text-[11px]')
    expect(wrapper.get('p').classes()).toContain('text-[11px]')
    expect(wrapper.get('[data-test="monitor-group-status"]').text()).toBe('运行中')
    expect(wrapper.get('[data-test="monitor-availability-badge"]').text()).toBe('100%')
    expect(wrapper.get('[data-test="monitor-rate-multiplier"]').text()).toContain('0.3×')
    expect(wrapper.text()).toContain('首字速度：2.22 s')
    expect(wrapper.get('[data-test="timeline-slot"]').exists()).toBe(true)
  })
})
