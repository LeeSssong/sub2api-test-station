import { describe, expect, it } from 'vitest'
import type { Group } from '@/types'
import type { MonitorV4Group } from '@/features/monitor-v4/types'
import { buildLineOptions } from '../lineOptions'

const group = (id: number, platform = 'openai', status: Group['status'] = 'active') =>
  ({ id, name: `Line ${id}`, platform, status, rate_multiplier: 1.2 }) as Group

describe('key line options', () => {
  it('prefers the current user rate and falls back to the configured group rate', () => {
    const result = buildLineOptions([group(1), group(2)], { 1: 0.8 }, new Map(), new Map(), undefined)
    expect(result.map(option => option.rateLabel)).toEqual(['0.8x倍率', '1.2x倍率'])
  })

  it('keeps missing or invalid rate unavailable rather than fabricating a multiplier', () => {
    const result = buildLineOptions([{ ...group(1), rate_multiplier: Number.NaN }], {}, new Map(), new Map(), undefined)
    expect(result[0]?.rateLabel).toBe('倍率暂不可用')
  })

  it('limits AI tool choices to active configurable lines for the selected tool', () => {
    const metrics = new Map<number, MonitorV4Group>([[2, { tool_ids: ['codex'], sla_request_count:100,sla_success_count:97,success_rate: 97, ttft_p50_ms: 2160 } as MonitorV4Group]])
    const result = buildLineOptions([group(1, 'anthropic'), {...group(2, 'anthropic'), tool_ids:['codex']}, group(3, 'openai', 'inactive')], {}, metrics, new Map([[2, 2]]), 'codex')
    expect(result.map(option => option.value)).toEqual([2])
    expect(result[0]).toMatchObject({ successLabel: '97%', ttftLabel: '2.16s' })
  })

  it('does not show zero linked keys when the count request has failed', () => {
    const result = buildLineOptions([group(1)], {}, new Map(), null, undefined)
    expect(result[0]).not.toHaveProperty('linkedCount')
  })
})

it('hides stale TTFT and separates expired data from no samples', () => {
 const now=Date.parse('2026-11-03T10:00:00Z')
 const m={sla_request_count:10,sla_success_count:9,ttft_p50_ms:0} as MonitorV4Group
 expect(buildLineOptions([group(1)],{},new Map([[1,m]]),null,undefined,'2026-11-03T09:50:00Z',now)[0]).toMatchObject({statusLabel:'统计已过期',successLabel:'—',ttftLabel:'—'})
})
it.each([[0,'good'],[9999,'good'],[10000,'warn'],[30000,'slow'],[60000,'critical']])('uses request-history TTFT severity for %s', (ms,tone) => {
 const m={sla_request_count:10,sla_success_count:0,ttft_p50_ms:ms} as MonitorV4Group
 expect(buildLineOptions([group(1)],{},new Map([[1,m]]),null)[0]).toMatchObject({statusLabel:'异常',successLabel:'0%',ttftSeverity:tone})
})
it('preserves inactive unselectable reason independently of health', () => {
 expect(buildLineOptions([group(1,'openai','inactive')],{},new Map(),null)[0]).toMatchObject({disabled:true,unavailableReason:'线路已停用',statusLabel:'暂无数据'})
})

it('shows the same SLA success rate and status in both key entry points', () => {
 const m={sla_request_count:667,sla_success_count:660,real_request_count:841,real_success_count:660,ttft_p50_ms:2160} as MonitorV4Group
 for(const tool of [undefined,'codex']) {
  expect(buildLineOptions([group(1)],{},new Map([[1,m]]),new Map(),tool)[0]).toMatchObject({successLabel:'98.95%',statusLabel:'正常运行'})
 }
})
