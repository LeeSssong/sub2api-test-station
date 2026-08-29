import { describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get } }))

import { getCodexRadarInsights, parseCodexRadarInsights } from '../codexRadar'

const fixture = {
  generated_at: '2026-08-19T01:59:16Z',
  source_updated_at: '2026-08-19T01:54:42Z',
  stale: false,
  recommendations: [
    { key: 'daily_development', title: '日常开发', rule: 'r1', items: [{ model: 'gpt-5.6-sol', effort: 'medium', iq: 90.12, average_duration_minutes: 17.53, average_cost_usd: 3.267475, rule: 'r1' }] },
    { key: 'hard_problems', title: '难题攻坚', rule: 'r2', items: [{ model: 'gpt-5.6-sol', effort: 'ultra', iq: 104.61, average_duration_minutes: 49.95, average_cost_usd: 22.362359, rule: 'r2' }] },
    { key: 'background_automation', title: '后台自动化', rule: 'r3', items: [{ model: 'gpt-5.6-luna', effort: 'xhigh', iq: 84.59, average_duration_minutes: 26.58, average_cost_usd: 0.318406, rule: 'r3' }] },
    { key: 'lobster_tasks', title: '跑龙虾类任务', rule: 'r4', items: [{ model: 'gpt-5.6-terra', effort: 'low', iq: 56.39, average_duration_minutes: 7.86, average_cost_usd: 0.461982, rule: 'r4' }] },
  ],
}

describe('CodexRadar DTO', () => {
  it('parses the four public recommendation categories', () => {
    const parsed = parseCodexRadarInsights(fixture)
    expect(parsed.recommendations.map((item) => item.key)).toEqual(['daily_development', 'hard_problems', 'background_automation', 'lobster_tasks'])
    expect(parsed.recommendations[0].items[0]).toMatchObject({ model: 'gpt-5.6-sol', effort: 'medium', iq: 90.12, average_duration_minutes: 17.53, average_cost_usd: 3.267475 })
  })

  it('fails closed when categories change', () => {
    expect(() => parseCodexRadarInsights({ ...fixture, recommendations: fixture.recommendations.slice(0, 3) })).toThrow()
  })

  it('uses only the fixed native proxy endpoint', async () => {
    get.mockResolvedValueOnce({ data: fixture })
    await getCodexRadarInsights()
    expect(get).toHaveBeenCalledWith('/public/codexradar/insights', { signal: undefined })
  })

  it('keeps empty categories as explicit empty states', () => {
    const parsed = parseCodexRadarInsights({
      ...fixture,
      source_status: 'fresh',
      recommendations: fixture.recommendations.map((item, index) => index === 0 ? { ...item, items: [], status: 'empty' } : item),
    })
    expect(parsed.recommendations[0]).toMatchObject({ status: 'empty', items: [] })
  })
})
