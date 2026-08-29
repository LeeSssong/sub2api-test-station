import { describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get } }))

import { getCodexRadarCommunity, parseCodexRadarCommunity } from '../codexRadarCommunity'

const point = { model: 'gpt-5.6-sol', effort: 'low', samples: 422, iq: 78.29, average_cost_usd: 1.8, average_duration_minutes: 12.1, software_samples: 336, visual_samples: 86, software_iq: 78.12, visual_iq: 78.46 }
const fixture = {
  generated_at: '2026-08-19T05:00:00Z', stale: false,
  tabs: [
    { key: 'comprehensive', source_updated_at: '2026-08-19T04:22:43Z', points: [point] },
    { key: 'software', source_updated_at: '2026-08-19T04:22:43Z', points: [{ ...point, samples: 336 }] },
    { key: 'visual', source_updated_at: '2026-08-19T04:45:30Z', points: [{ ...point, samples: 86 }] },
  ],
}

describe('CodexRadar community DTO', () => {
  it('parses the fixed three-tab matrix', () => {
    const parsed = parseCodexRadarCommunity(fixture)
    expect(parsed.tabs.map((tab) => tab.key)).toEqual(['comprehensive', 'software', 'visual'])
    expect(parsed.tabs[0].points[0]).toMatchObject({ samples: 422, iq: 78.29, software_samples: 336, visual_samples: 86 })
  })

  it('fails closed on tab drift, duplicate points, invalid numbers, or missing composite fields', () => {
    expect(() => parseCodexRadarCommunity({ ...fixture, tabs: fixture.tabs.slice(0, 2) })).toThrow()
    expect(() => parseCodexRadarCommunity({ ...fixture, tabs: fixture.tabs.map((tab, index) => index ? tab : { ...tab, points: [point, point] }) })).toThrow()
    expect(() => parseCodexRadarCommunity({ ...fixture, tabs: fixture.tabs.map((tab, index) => index ? tab : { ...tab, points: [{ ...point, iq: -1 }] }) })).toThrow()
    expect(() => parseCodexRadarCommunity({ ...fixture, tabs: fixture.tabs.map((tab, index) => index ? tab : { ...tab, points: [{ ...point, software_iq: undefined }] }) })).toThrow()
  })

  it('uses only the fixed native proxy endpoint', async () => {
    get.mockResolvedValueOnce({ data: fixture })
    await getCodexRadarCommunity()
    expect(get).toHaveBeenCalledWith('/public/codexradar/community', { signal: undefined })
  })

  it('accepts partial tabs with an empty unavailable tab', () => {
    const parsed = parseCodexRadarCommunity({
      ...fixture,
      source_status: 'partial',
      tabs: [
        { key: 'comprehensive', source_updated_at: '', status: 'unavailable', error_code: 'NO_SHARED_MODEL_EFFORTS', points: [] },
        fixture.tabs[1],
        { ...fixture.tabs[2], status: 'unavailable', error_code: 'SOURCE_UNAVAILABLE', source_updated_at: '', points: [] },
      ],
    })
    expect(parsed.tabs[0]).toMatchObject({ status: 'unavailable', error_code: 'NO_SHARED_MODEL_EFFORTS', points: [] })
    expect(parsed.tabs[2].source_updated_at).toBe('')
  })
})
