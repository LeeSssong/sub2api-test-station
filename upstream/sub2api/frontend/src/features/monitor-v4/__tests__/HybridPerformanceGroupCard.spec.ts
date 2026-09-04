import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import HybridPerformanceGroupCard from '../HybridPerformanceGroupCard.vue'

const componentSource = readFileSync('src/features/monitor-v4/HybridPerformanceGroupCard.vue', 'utf8')

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, args?: Record<string, unknown>) => key === 'channelMonitorV2.hybrid.multiplier' ? `倍率：${args?.value}x` : key === 'channelMonitorV2.hybrid.successRate' ? '成功率' : key === 'channelMonitorV2.hybrid.sampleCount' ? `基于 ${args?.count} 次调用` : key }) }))

const group = { id: 1, name: 'Primary', platform: 'openai', rate_multiplier: 0.3, success_rate: 85, request_count: 20, success_count: 17, real_request_count: 15, real_success_count: 14, probe_fallback_bucket_count: 5, probe_fallback_request_count: 5, ttft_p95_ms: 120, ttft_sample_count: 12, latency_p95_ms: 900, latency_sample_count: 12, cache_hit_rate: 0.4, source_updated_at: '2026-08-25T00:00:00Z', current_operational: true }

describe('HybridPerformanceGroupCard', () => {
  it('uses threshold tones and the approved labels', () => {
    const wrapper = mount(HybridPerformanceGroupCard, { props: { group }, global: { mocks: { $t: (key: string, args?: Record<string, unknown>) => key === 'channelMonitorV2.hybrid.multiplier' ? `倍率：${args?.value}x` : key === 'channelMonitorV2.hybrid.successRate' ? '成功率' : key === 'channelMonitorV2.hybrid.sampleCount' ? `基于 ${args?.count} 次调用` : key } } })
    expect(wrapper.find('.hybrid-card--green').exists()).toBe(true)
    expect(wrapper.get('[data-test="multiplier"]').text()).toBe('倍率：0.3x')
    expect(wrapper.get('.hybrid-ring__center span').text()).toBe('成功率')
    expect(wrapper.get('[data-test="sample-count"]').text()).toBe('基于 20 次调用')
    expect(wrapper.get('header [data-test="multiplier"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="ring"]').classes()).not.toContain('orbit')
    expect(wrapper.text()).not.toContain('综合成功')
    expect(wrapper.text()).not.toContain('真实请求成功')
    expect(wrapper.text()).not.toContain('探测补足')
    expect(wrapper.text()).not.toContain('空桶')
  })

  it('formats latency metrics in seconds with two decimal places', () => {
    const wrapper = mount(HybridPerformanceGroupCard, {
      props: { group: { ...group, ttft_p95_ms: 11202, latency_p95_ms: 14129 } },
    })

    expect(wrapper.get('[data-test="ttft-p95"]').text()).toBe('11.20 s')
    expect(wrapper.get('[data-test="latency-p95"]').text()).toBe('14.13 s')
    expect(wrapper.get('[data-test="cache-hit-rate"]').text()).toBe('40.0%')
  })

  it('shows an explicit empty cache metric when there are no successful real requests', () => {
    const wrapper = mount(HybridPerformanceGroupCard, { props: { group: { ...group, cache_hit_rate: null } } })
    expect(wrapper.get('[data-test="cache-hit-rate"]').text()).toBe('--')
  })

  it('keeps the center percentage static while the ring only breathes', () => {
    const wrapper = mount(HybridPerformanceGroupCard, { props: { group }, global: { mocks: { $t: (key: string, args?: Record<string, unknown>) => key === 'channelMonitorV2.hybrid.multiplier' ? `倍率：${args?.value}x` : key === 'channelMonitorV2.hybrid.successRate' ? '成功率' : key === 'channelMonitorV2.hybrid.sampleCount' ? `基于 ${args?.count} 次调用` : key } } })
    expect(wrapper.get('[data-test="success-rate"]').text()).toBe('85%')
    expect(wrapper.find('[data-test="ring"]').classes()).not.toContain('rotate')
    expect(wrapper.find('[data-test="success-rate"]').classes()).not.toContain('animate-spin')
  })

  it('uses a layered dark surface and stronger breathing glow', () => {
    expect(componentSource).toContain('--ring-surface: #122136')
    expect(componentSource).toContain('color: #dce8f5')
    expect(componentSource).toContain('animation: hybrid-breathe 2.8s ease-in-out infinite')
    expect(componentSource).toContain('0 0 48px')
  })

  it('targets dark overrides from the document theme ancestor', () => {
    expect(componentSource).toContain(':global(.dark) .hybrid-card')
    expect(componentSource).toContain(':global(.dark) .hybrid-metric strong')
  })

  it.each([
    [85, 'green'],
    [50, 'amber'],
    [49.9, 'red'],
  ])('maps success rate %s to the %s tone', (success_rate, tone) => {
    const wrapper = mount(HybridPerformanceGroupCard, {
      props: { group: { ...group, success_rate } },
    })
    expect(wrapper.find(`.hybrid-card--${tone}`).exists()).toBe(true)
  })
})
