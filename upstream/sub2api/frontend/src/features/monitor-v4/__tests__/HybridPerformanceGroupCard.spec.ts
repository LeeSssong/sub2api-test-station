import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import HybridPerformanceGroupCard from '../HybridPerformanceGroupCard.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, args?: Record<string, unknown>) => key === 'monitorV2.hybrid.multiplier' ? `倍率：${args?.value}x` : key === 'monitorV2.hybrid.sampleCount' ? `基于 ${args?.count} 次真实请求` : key }) }))

const group = { id: 1, name: 'Primary', platform: 'openai', rate_multiplier: 0.3, availability: 85, availability_bucket_count: 17, total_bucket_count: 20, ttft_p95_ms: 120, latency_p95_ms: 900, sample_count: 12, current_operational: true, is_fallback_metric: false }

describe('HybridPerformanceGroupCard', () => {
  it('uses threshold tones and the approved labels', () => {
    const wrapper = mount(HybridPerformanceGroupCard, { props: { group }, global: { mocks: { $t: (key: string, args?: Record<string, unknown>) => key === 'monitorV2.hybrid.multiplier' ? `倍率：${args?.value}x` : key === 'monitorV2.hybrid.sampleCount' ? `基于 ${args?.count} 次真实请求` : key } } })
    expect(wrapper.find('.hybrid-card--green').exists()).toBe(true)
    expect(wrapper.get('[data-test="multiplier"]').text()).toBe('倍率：0.3x')
    expect(wrapper.get('[data-test="sample-count"]').text()).toContain('基于 12 次真实请求')
    expect(wrapper.find('[data-test="ring"]').classes()).not.toContain('orbit')
  })
})
