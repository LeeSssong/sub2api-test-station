import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getCodexRadarCommunity } = vi.hoisted(() => ({ getCodexRadarCommunity: vi.fn() }))
vi.mock('../codexRadarCommunity', async () => ({
  ...(await vi.importActual<typeof import('../codexRadarCommunity')>('../codexRadarCommunity')),
  getCodexRadarCommunity,
}))

import CodexRadarCommunityMatrix from '../CodexRadarCommunityMatrix.vue'

const comprehensive = { model: 'gpt-5.6-sol', effort: 'low', samples: 422, iq: 78.29, average_cost_usd: 1.8, average_duration_minutes: 12.1, software_samples: 336, visual_samples: 86, software_iq: 78.12, visual_iq: 78.46 }
const fixture = {
  generated_at: '2026-08-19T05:00:00Z', stale: false,
  tabs: [
    { key: 'comprehensive', source_updated_at: '2026-08-19T04:22:43Z', points: [comprehensive] },
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
