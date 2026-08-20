import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getCodexRadarInsights } = vi.hoisted(() => ({ getCodexRadarInsights: vi.fn() }))
vi.mock('../codexRadar', async () => ({
  ...(await vi.importActual<typeof import('../codexRadar')>('../codexRadar')),
  getCodexRadarInsights,
}))

import CodexRadarRecommendations from '../CodexRadarRecommendations.vue'

const global = { stubs: { CodexRadarCommunityMatrix: true } }

const fixture = {
  generated_at: '2026-08-19T01:59:16Z', source_updated_at: '2026-08-19T01:54:42Z', stale: false,
  recommendations: [
    { key: 'daily_development', title: '日常开发', rule: '日常规则', items: [{ model: 'gpt-5.6-sol', effort: 'medium', iq: 90.12, average_duration_minutes: 17.53, average_cost_usd: 3.267475, rule: '日常规则' }] },
    { key: 'hard_problems', title: '难题攻坚', rule: '难题规则', items: [{ model: 'gpt-5.6-sol', effort: 'ultra', iq: 104.61, average_duration_minutes: 49.95, average_cost_usd: 22.362359, rule: '难题规则' }] },
    { key: 'background_automation', title: '后台自动化', rule: '后台规则', items: [{ model: 'gpt-5.6-luna', effort: 'xhigh', iq: 84.59, average_duration_minutes: 26.58, average_cost_usd: 0.318406, rule: '后台规则' }] },
    { key: 'lobster_tasks', title: '跑龙虾类任务', rule: '龙虾规则', items: [{ model: 'gpt-5.6-terra', effort: 'low', iq: 56.39, average_duration_minutes: 7.86, average_cost_usd: 0.461982, rule: '龙虾规则' }] },
  ],
}

describe('CodexRadarRecommendations', () => {
  beforeEach(() => getCodexRadarInsights.mockReset())

  it('renders the four original categories and metrics', async () => {
    getCodexRadarInsights.mockResolvedValue(fixture)
    const wrapper = mount(CodexRadarRecommendations, { global })
    await flushPromises()
    for (const value of ['站长推荐', '日常开发', '难题攻坚', '后台自动化', '跑龙虾类任务', 'Sol medium', 'IQ 90', '18 分钟', '$3.27', '日常规则']) expect(wrapper.text()).toContain(value)
    expect(wrapper.findAll('[data-radar-category]')).toHaveLength(4)
    const panel = wrapper.get('[data-test="codexradar-panel"]')
    expect(panel.classes()).toContain('sm:p-6')
    expect(panel.classes()).toContain('xl:p-7')
    expect(panel.classes()).toContain('bg-white')
    expect(panel.classes()).toContain('dark:bg-dark-950')
    expect(wrapper.get('[data-radar-category]').classes()).toContain('bg-emerald-50/70')
    expect(wrapper.get('[data-radar-category]').classes()).toContain('dark:bg-dark-900/90')
  })

  it('shows a compact unavailable state without breaking the page', async () => {
    getCodexRadarInsights.mockResolvedValue(null)
    const wrapper = mount(CodexRadarRecommendations, { global })
    await flushPromises()
    expect(wrapper.text()).toContain('站长推荐暂时不可用')
  })

  it('marks fallback snapshots', async () => {
    getCodexRadarInsights.mockResolvedValue({ ...fixture, stale: true })
    const wrapper = mount(CodexRadarRecommendations, { global })
    await flushPromises()
    expect(wrapper.text()).toContain('最近成功数据')
  })
})
