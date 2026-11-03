import * as model from '../model'
import { describe, expect, it } from 'vitest'
import { linkedCounts, configuredLines, routeHealth, compareQuality, metricLabel, toolIdsForGroup, rankWeightedRoutes } from '../model'
import type { ApiKey, Group } from '@/types'
import type { MonitorV4Group } from '@/features/monitor-v4/types'
const group = (id: number, status = 'active') => ({ id, name: `线路${id}`, status, platform: 'openai', rate_multiplier: 1 }) as Group
const key = (id: number, group_id: number, status = 'active', g?: Group) => ({ id, group_id, status, group: g }) as ApiKey
const availability = (_group: Group, ...args: Parameters<typeof routeHealth>) => routeHealth(...args)
const now = Date.now()
const metric = (overrides = {}) => ({ source_updated_at: new Date(now).toISOString(), success_rate: 98, request_count: 10, ttft_p50_ms: 1200, latency_p50_ms: 3000, ...overrides }) as MonitorV4Group

describe('AI线路真实口径', () => {
  it('counts disabled keys and keeps a line once', () => {
    const keys = [key(1,1),key(2,1,'inactive')]
    expect(linkedCounts(keys).get(1)).toBe(2)
    expect(configuredLines([group(1),group(2)],keys).map(g=>g.id)).toEqual([1])
  })
  it('retains an inactive linked group from native key relation', () => {
    expect(configuredLines([], [key(1,3,'inactive',group(3,'inactive'))])[0].status).toBe('inactive')
  })
  it('falls back to the group platform only when no explicit tool mapping exists', () => {
    const openaiGroup = group(1)
    const anthropicGroup = { ...group(2), platform: 'anthropic' } as Group
    expect(toolIdsForGroup(openaiGroup, metric({ tool_ids: [] }))).toEqual(['codex'])
    expect(toolIdsForGroup(anthropicGroup, metric({ tool_ids: [] }))).toEqual(['claude'])
    expect(toolIdsForGroup(openaiGroup, metric({ tool_ids: ['grok'] }))).toEqual(['grok'])
  })
  it('prefers current native tool mapping over the older monitor snapshot', () => {
    expect(toolIdsForGroup({ ...group(1), tool_ids: ['claude'] }, metric({ tool_ids: ['codex'] }))).toEqual(['claude'])
    expect(toolIdsForGroup({ ...group(1), tool_ids: [] }, metric({ tool_ids: ['claude'] }))).toEqual(['codex'])
  })
  it.each([[900,1000,'正常运行'],[8999,10000,'波动'],[700,1000,'波动'],[6999,10000,'异常'],[0,10,'异常']])('classifies real counts %s/%s without rounding', (success,requests,text) => {
    expect(availability(group(1),metric({sla_request_count:requests,sla_success_count:success}),now,new Date(now).toISOString()).text).toBe(text)
  })
  it('shows no data for zero real requests even when all probes succeeded', () => {
    expect(availability(group(1),metric({sla_request_count:0,sla_success_count:0}),now,new Date(now).toISOString()).text).toBe('暂无数据')
  })
  it('uses snapshot freshness, not the age of the last request or management status', () => {
    const m=metric({sla_request_count:10,sla_success_count:9,source_updated_at:new Date(now-1800000).toISOString()})
    expect(availability(group(1),m,now,new Date(now-30000).toISOString()).text).toBe('正常运行')
    expect(availability(group(1,'inactive'),m,now,new Date(now).toISOString()).text).toBe('正常运行')
    expect(availability(group(1),m,now,new Date(now-421000).toISOString()).text).toBe('暂无数据')
    expect(availability(group(1),undefined,now,new Date(now).toISOString()).text).toBe('暂无数据')
    expect(availability(group(1),m,now,new Date(now+1000).toISOString()).text).toBe('暂无数据')
  })
  it('aggregates counts instead of averaging line rates or counting probes', () => {
    expect(model.aggregateRouteHealth([metric({sla_request_count:1,sla_success_count:0}),metric({sla_request_count:99,sla_success_count:99})],now,new Date(now).toISOString()).text).toBe('正常运行')
    expect(model.aggregateRouteHealth([metric({sla_request_count:0,sla_success_count:0})],now,new Date(now).toISOString()).text).toBe('暂无数据')
  })
  it('sorts sampled routes by real success, sample size and P50, ignoring probe flags', () => {
    const ms = new Map([[1,metric({sla_request_count:2,sla_success_count:2})],[2,metric({sla_request_count:20,sla_success_count:20})]])
    expect([group(1),group(2)].sort((a,b)=>compareQuality(a,b,ms,{},now,new Date(now).toISOString()))[0].id).toBe(2)
  })
  it('ranks only active routes with complete 24h metrics using 50/30/20 weighted quality', () => {
    const routes = [group(1), group(2), group(3, 'inactive')]
    const ms = new Map([
      [1, metric({ sla_request_count: 100, sla_success_count: 100, cache_hit_rate: 0.2, ttft_p50_ms: 1000 })],
      [2, metric({ sla_request_count: 100, sla_success_count: 90, cache_hit_rate: 0.8, ttft_p50_ms: 500 })],
      [3, metric({ sla_request_count: 100, sla_success_count: 100, cache_hit_rate: 1, ttft_p50_ms: 1 })],
    ])
    const ranked = rankWeightedRoutes(routes, ms, now, new Date(now).toISOString())
    expect(ranked.map(item => item.group.id)).toEqual([2, 1])
    expect(ranked[0].score).toBeCloseTo(89)
  })
  it('excludes active routes when any weighted metric is unavailable', () => {
    const routes = [group(1), group(2)]
    const ms = new Map([
      [1, metric({ sla_request_count: 10, sla_success_count: 10, cache_hit_rate: null, ttft_p50_ms: 500 })],
      [2, metric({ sla_request_count: 10, sla_success_count: 10, cache_hit_rate: 0.5, ttft_p50_ms: null })],
    ])
    expect(rankWeightedRoutes(routes, ms, now, new Date(now).toISOString())).toEqual([])
  })
  it('gives equal TTFTs full speed points and sorts exact ties by ID', () => {
    const m = metric({sla_request_count:10, sla_success_count:9, cache_hit_rate:0, ttft_p50_ms:0})
    const ranked = rankWeightedRoutes([group(2),group(1)], new Map([[1,m],[2,m]]), now, new Date(now).toISOString())
    expect(ranked.map(item => item.group.id)).toEqual([1,2])
    expect(ranked[0].ttftScore).toBe(100)
    expect(ranked[0].score).toBe(65)
    expect(rankWeightedRoutes([group(1)],new Map([[1,m]]),now,new Date(now).toISOString())[0].score).toBe(65)
  })
  it('does not rank zero requests, invalid metrics, or stale snapshots', () => {
    const valid = metric({sla_request_count:10, sla_success_count:9, cache_hit_rate:.5, ttft_p50_ms:1000})
    for (const override of [{sla_request_count:0,sla_success_count:0},{cache_hit_rate:NaN},{cache_hit_rate:1.1},{ttft_p50_ms:-1},{ttft_p50_ms:Infinity}]) {
      expect(rankWeightedRoutes([group(1)],new Map([[1, {...valid,...override}]]),now,new Date(now).toISOString())).toEqual([])
    }
    expect(rankWeightedRoutes([group(1)],new Map([[1,valid]]),now,new Date(now-421000).toISOString())).toEqual([])
  })
  it('does not substitute P95 or a check value for missing P50', () => {
    expect(metricLabel(undefined)).toBe('—')
    expect(metricLabel(2160)).toBe('2.16s')
  })
})

 it('uses SLA counts for status, label and recommendation despite excluded errors', () => {
   const m=metric({real_request_count:841,real_success_count:660,sla_request_count:667,sla_success_count:660,cache_hit_rate:.5})
   const health=routeHealth(m,now,new Date(now).toISOString())
   expect(health.text).toBe('正常运行')
   expect(model.routeSuccessLabel(health)).toBe('98.95%')
   expect(rankWeightedRoutes([group(1)],new Map([[1,m]]),now,new Date(now).toISOString())[0].successRate).toBeCloseTo(98.9505247)
 })
 it('does not fall back to legacy counts when SLA is unavailable', () => {
   expect(routeHealth(metric({real_request_count:100,real_success_count:100})).text).toBe('暂无数据')
 })

 it('discovers Gemini through platform fallback and authoritative cross-platform mappings', () => {
   expect(model.tools.map(t=>t.id)).toEqual(['codex','claude','gemini','deepseek','grok'])
   expect(toolIdsForGroup({ ...group(5), platform: 'gemini' } as Group)).toEqual(['gemini'])
   expect(toolIdsForGroup({ ...group(6), tool_ids: ['gemini','codex'] })).toEqual(['gemini','codex'])
   expect(toolIdsForGroup({ ...group(5), platform: 'gemini', tool_ids: ['claude'] } as Group)).toEqual(['claude'])
 })

it('uses resolved platform classification and never restores a stale monitor override',()=>{
 expect(toolIdsForGroup({...group(1),tool_ids:['codex'],tool_mapping_source:'platform'} as Group,metric({tool_ids:['claude']}))).toEqual(['codex'])
 expect(toolIdsForGroup({...group(1),platform:'composite',tool_ids:[],tool_mapping_source:'unclassified'} as Group,metric({tool_ids:['claude']}))).toEqual([])
})
