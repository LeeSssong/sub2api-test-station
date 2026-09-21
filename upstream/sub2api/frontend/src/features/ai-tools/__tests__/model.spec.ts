import { describe, expect, it } from 'vitest'
import { linkedCounts, configuredLines, availability, compareQuality, metricLabel } from '../model'
import type { ApiKey, Group } from '@/types'
import type { MonitorV4Group } from '@/features/monitor-v4/types'
const group = (id: number, status = 'active') => ({ id, name: `线路${id}`, status, platform: 'openai', rate_multiplier: 1 }) as Group
const key = (id: number, group_id: number, status = 'active', g?: Group) => ({ id, group_id, status, group: g }) as ApiKey
const now = Date.now()
const metric = (overrides = {}) => ({ current_operational: true, source_updated_at: new Date(now).toISOString(), success_rate: 98, request_count: 10, ttft_p50_ms: 1200, latency_p50_ms: 3000, ...overrides }) as MonitorV4Group

describe('AI线路真实口径', () => {
  it('counts disabled keys and keeps a line once', () => {
    const keys = [key(1,1),key(2,1,'inactive')]
    expect(linkedCounts(keys).get(1)).toBe(2)
    expect(configuredLines([group(1),group(2)],keys).map(g=>g.id)).toEqual([1])
  })
  it('retains an inactive linked group from native key relation', () => {
    expect(configuredLines([], [key(1,3,'inactive',group(3,'inactive'))])[0].status).toBe('inactive')
  })
  it('never treats missing or expired observations as available', () => {
    expect(availability(group(1),undefined,now).text).toBe('暂不可用')
    expect(availability(group(1),metric({source_updated_at:new Date(now-301000).toISOString()}),now).text).toBe('暂不可用')
    expect(availability(group(1),metric(),now).text).toBe('可用')
    expect(availability(group(1,'inactive'),metric(),now).text).toBe('停用')
  })
  it('sorts quality by availability, success, sample size and real P50', () => {
    const ms = new Map([[1,metric({request_count:2})],[2,metric({request_count:20})]])
    expect([group(1),group(2)].sort((a,b)=>compareQuality(a,b,ms,{},now))[0].id).toBe(2)
  })
  it('does not substitute P95 or a check value for missing P50', () => {
    expect(metricLabel(undefined)).toBe('—')
    expect(metricLabel(2160)).toBe('2.16s')
  })
})
