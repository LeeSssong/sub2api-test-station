import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getCodexRadarCommunity } = vi.hoisted(() => ({ getCodexRadarCommunity: vi.fn() }))
vi.mock('../codexRadarCommunity', async () => ({
  ...(await vi.importActual<typeof import('../codexRadarCommunity')>('../codexRadarCommunity')),
  getCodexRadarCommunity,
}))

import CodexRadarCommunityMatrix from '../CodexRadarCommunityMatrix.vue'

const comprehensive = { model: 'gpt-5.6-sol', effort: 'low', samples: 422, iq: 78.29, average_cost_usd: 1.8, average_duration_minutes: 12.1, software_samples: 336, visual_samples: 86, software_iq: 78.12, visual_iq: 78.46 }
const point = (model: string, effort: string, iq: number) => ({ ...comprehensive, model, effort, iq })
const fixture = {
  generated_at: '2026-08-19T05:00:00Z', stale: false,
  tabs: [
    {
      key: 'comprehensive',
      source_updated_at: '2026-08-19T04:22:43Z',
      points: [
        point('gpt-5.6-sol', 'low', 78), point('gpt-5.6-sol', 'ultra', 105), point('gpt-5.6-sol', 'medium', 89),
        point('gpt-5.6-terra', 'low', 56), point('gpt-5.6-terra', 'ultra', 95), point('gpt-5.6-terra', 'max', 95),
        point('gpt-5.6-luna', 'low', 21), point('gpt-5.6-luna', 'max', 88),
        point('gpt-5.5', 'high', 88), point('gpt-5.5', 'xhigh', 94),
      ],
    },
    { key: 'software', source_updated_at: '2026-08-19T04:22:43Z', points: [{ ...comprehensive, model: 'gpt-5.6-terra', effort: 'max', samples: 336, iq: 96.43 }] },
    { key: 'visual', source_updated_at: '2026-08-19T04:45:30Z', points: [{ ...comprehensive, model: 'gpt-5.6-sol', effort: 'ultra', samples: 86, iq: 105.1 }] },
  ],
}

describe('CodexRadarCommunityMatrix', () => {
  beforeEach(() => getCodexRadarCommunity.mockReset())

  it('renders the approved community matrix and switches all three tabs', async () => {
    getCodexRadarCommunity.mockResolvedValue(fixture)
    const wrapper = mount(CodexRadarCommunityMatrix)
    await flushPromises()
    for (const value of ['综合智能', '软件工程能力', '视觉空间推理', '社区众测数据', 'Sol low', 'IQ 78', '422 份样本', '$1.80', '12.1 分钟']) expect(wrapper.text()).toContain(value)
    expect(wrapper.find('[data-community-scroll]').classes()).toContain('overflow-x-auto')
    expect(wrapper.get('[data-community-grid]').classes()).toContain('min-w-[1120px]')
    expect(wrapper.get('[data-community-tab=\"comprehensive\"]').classes()).toContain('bg-blue-50')
    expect(wrapper.get('[data-community-card]').classes()).toContain('bg-white')
    expect(wrapper.get('[data-community-card]').classes()).toContain('dark:bg-dark-900/95')

    const solCards = wrapper.get('[data-community-family="gpt-5.6-sol"]').findAll('[data-community-card]')
    expect(solCards.map((card) => card.attributes('data-effort'))).toEqual(['ultra', 'medium', 'low'])
    expect(solCards.map((card) => card.attributes('style'))).toEqual([
      expect.stringContaining('grid-column-start: 1'),
      expect.stringContaining('grid-column-start: 5'),
      expect.stringContaining('grid-column-start: 6'),
    ])
    expect(wrapper.get('[data-community-family="gpt-5.6-luna"] [data-effort="max"]').attributes('style')).toContain('grid-column-start: 2')
    expect(wrapper.get('[data-community-family="gpt-5.5"] [data-effort="xhigh"]').attributes('style')).toContain('grid-column-start: 3')
    expect(wrapper.get('[data-community-family="gpt-5.5"] [data-effort="high"]').attributes('style')).toContain('grid-column-start: 4')
    expect(wrapper.get('[data-community-family="gpt-5.6-sol"] [data-effort="low"]').attributes('style')).toContain('grid-column-start: 6')
    expect(wrapper.get('[data-community-family="gpt-5.6-terra"] [data-effort="low"]').attributes('style')).toContain('grid-column-start: 6')
    expect(wrapper.get('[data-community-family="gpt-5.6-luna"] [data-effort="low"]').attributes('style')).toContain('grid-column-start: 6')
    expect(wrapper.find('[data-community-placeholder]').exists()).toBe(false)
    await wrapper.get('[data-community-tab="software"]').trigger('click')
    expect(wrapper.text()).toContain('Terra max')
    expect(wrapper.text()).toContain('336 份样本')
    await wrapper.get('[data-community-tab="visual"]').trigger('click')
    expect(wrapper.text()).toContain('Sol ultra')
    expect(wrapper.text()).toContain('86 份样本')
  })

  it('marks stale snapshots and isolates unavailable state', async () => {
    getCodexRadarCommunity.mockResolvedValue({ ...fixture, stale: true })
    const stale = mount(CodexRadarCommunityMatrix)
    await flushPromises()
    expect(stale.text()).toContain('最近成功数据')

    getCodexRadarCommunity.mockRejectedValueOnce(new Error('down'))
    const failed = mount(CodexRadarCommunityMatrix)
    await flushPromises()
    expect(failed.text()).toContain('社区测试数据暂时不可用')
  })
})
