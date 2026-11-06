import { describe, expect, it } from 'vitest'
import type { Group } from '@/types'
import type { LineSelectionMetric } from '@/features/monitor-v4/lineSelection'
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
    const metrics = new Map<number, LineSelectionMetric>([[2, { tool_ids: ['codex'], has_samples:true,health:'healthy',availability_health:'healthy',error_rate:.03, ttft_p50_ms: 2160 } as LineSelectionMetric]])
    const result = buildLineOptions([group(1, 'anthropic'), {...group(2, 'anthropic'), tool_ids:['codex']}, group(3, 'openai', 'inactive')], {}, metrics, new Map([[2, 2]]), 'codex')
    expect(result.map(option => option.value)).toEqual([2])
    expect(result[0]).toMatchObject({ successLabel: '97.0%', ttftLabel: '2.2s' })
  })

  it('does not show zero linked keys when the count request has failed', () => {
    const result = buildLineOptions([group(1)], {}, new Map(), null, undefined)
    expect(result[0]).not.toHaveProperty('linkedCount')
  })
})

it('hides stale TTFT and separates expired data from no samples', () => {
 const now=Date.parse('2026-11-03T10:00:00Z')
 const m={has_samples:true,health:'healthy',availability_health:'healthy',error_rate:.1,ttft_p50_ms:0} as LineSelectionMetric
 expect(buildLineOptions([group(1)],{},new Map([[1,m]]),null,undefined,'2026-11-03T09:50:00Z',now)[0]).toMatchObject({statusLabel:'统计已过期',successLabel:'—',ttftLabel:'—'})
})
it.each([[0,'good'],[9999,'good'],[10000,'warn'],[30000,'slow'],[60000,'critical']])('uses request-history TTFT severity for %s', (ms,tone) => {
 const m={has_samples:true,health:'critical',availability_health:'critical',error_rate:1,ttft_p50_ms:ms} as LineSelectionMetric
 expect(buildLineOptions([group(1)],{},new Map([[1,m]]),null)[0]).toMatchObject({statusLabel:'异常',successLabel:'0.00%',ttftSeverity:tone})
})
it('preserves inactive unselectable reason independently of health', () => {
 expect(buildLineOptions([group(1,'openai','inactive')],{},new Map(),null)[0]).toMatchObject({disabled:true,unavailableReason:'线路已停用',statusLabel:'暂无数据'})
})

it('shows the same route-detail availability and status in both key entry points', () => {
 const m={has_samples:true,health:'healthy',availability_health:'healthy',error_rate:7/667,ttft_p50_ms:2160} as LineSelectionMetric
 for(const tool of [undefined,'codex']) {
  expect(buildLineOptions([group(1)],{},new Map([[1,m]]),new Map(),tool)[0]).toMatchObject({successLabel:'99.0%',statusLabel:'正常'})
 }
})

it('uses route-detail rates and independent overall/error health with redacted counts', () => {
 const metric = { has_samples: true, health: 'warning' as const, availability_health: 'healthy' as const, error_rate: .005, ttft_p50_ms: 5000 }
 expect(buildLineOptions([group(1)], {}, new Map([[1, metric]]), null)[0]).toMatchObject({ statusLabel: '波动', healthKind: 'warning', successLabel: '99.5%', successTone: 'green', ttftLabel: '5.0s' })
})
it('distinguishes no samples, unscored traffic, and real zero availability', () => {
 const base = { health: 'unknown' as const, availability_health: 'critical' as const, error_rate: 1, ttft_p50_ms: 0 }
 expect(buildLineOptions([group(1)], {}, new Map([[1, { ...base, has_samples: false }]]), null)[0]).toMatchObject({ statusLabel: '暂无数据', successLabel: '—', ttftLabel: '—' })
 expect(buildLineOptions([group(1)], {}, new Map([[1, { ...base, has_samples: true }]]), null)[0]).toMatchObject({ statusLabel: '样本不足', successLabel: '0.00%', ttftLabel: '0ms' })
})

it('shows each mode’s actual availability range and never labels probe duration as TTFT', () => {
 const v1:LineSelectionMetric={source:'v1',has_samples:true,health:'healthy',status_label:'正常',availability_percent:99.5,availability_label:'可用性 · 近7天',availability_title:'近7天主动探测可用率',ttft_p50_ms:null,ttft_title:'当前监控未提供首字统计'}
 const v3:LineSelectionMetric={...v1,source:'v3',health:'warning',status_label:'性能下降',availability_percent:0,availability_label:'可用性 · 近30天'}
 const result=buildLineOptions([group(1),group(2)],{},new Map([[1,v1],[2,v3]]),null)
 expect(result[0]).toMatchObject({statusLabel:'正常',successLabel:'99.50%',availabilityLabel:'可用性 · 近7天',ttftLabel:'—',ttftTitle:'当前监控未提供首字统计'})
 expect(result[1]).toMatchObject({statusLabel:'性能下降',successLabel:'0%',availabilityLabel:'可用性 · 近30天',ttftLabel:'—'})
})
it('keeps disabled/multiple-monitor explanations while expired and failed responses hide all old values', () => {
 const metric:LineSelectionMetric={source:'v3',has_samples:false,status_label:'多项监控，查看详情',availability_percent:null,ttft_p50_ms:null}
 expect(buildLineOptions([group(1)],{},new Map([[1,metric]]),null)[0]).toMatchObject({statusLabel:'多项监控，查看详情',successLabel:'—',ttftLabel:'—'})
 expect(buildLineOptions([group(1)],{},new Map([[1,metric]]),null,undefined,undefined,Date.now(),true)[0].statusLabel).toBe('统计读取失败')
})
