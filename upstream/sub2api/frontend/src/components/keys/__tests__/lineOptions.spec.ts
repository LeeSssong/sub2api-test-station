import { describe, expect, it } from 'vitest'
import type { Group } from '@/types'
import type { MonitorV4Group } from '@/features/monitor-v4/types'
import { buildLineOptions } from '../lineOptions'

const group = (id: number, platform = 'openai', status: Group['status'] = 'active') =>
  ({ id, name: `Line ${id}`, platform, status, rate_multiplier: 1.2 }) as Group

describe('key line options', () => {
  it('prefers the current user rate and falls back to the configured group rate', () => {
    const result = buildLineOptions([group(1), group(2)], { 1: 0.8 }, new Map(), new Map(), undefined)
    expect(result.map(option => option.rateLabel)).toEqual(['0.8倍率', '1.2倍率'])
  })

  it('keeps missing or invalid rate unavailable rather than fabricating a multiplier', () => {
    const result = buildLineOptions([{ ...group(1), rate_multiplier: Number.NaN }], {}, new Map(), new Map(), undefined)
    expect(result[0]?.rateLabel).toBe('倍率暂不可用')
  })

  it('limits AI tool choices to active configurable lines for the selected tool', () => {
    const metrics = new Map<number, MonitorV4Group>([[2, { tool_ids: ['codex'], success_rate: 97, ttft_p50_ms: 2160 } as MonitorV4Group]])
    const result = buildLineOptions([group(1, 'anthropic'), group(2, 'anthropic'), group(3, 'openai', 'inactive')], {}, metrics, new Map([[2, 2]]), 'codex')
    expect(result.map(option => option.value)).toEqual([2])
    expect(result[0]).toMatchObject({ linkedCount: 2, successLabel: '97%', ttftLabel: '2.16s' })
  })

  it('does not show zero linked keys when the count request has failed', () => {
    const result = buildLineOptions([group(1)], {}, new Map(), null, undefined)
    expect(result[0]?.linkedCount).toBeNull()
  })
})
