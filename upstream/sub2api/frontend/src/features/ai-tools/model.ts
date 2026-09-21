import type { ApiKey, Group } from '@/types'
import type { MonitorV4Group } from '@/features/monitor-v4/types'
export const tools = [
  { id: 'codex', label: 'Codex', platform: 'openai', type: 'AI 编程' },
  { id: 'claude', label: 'Claude Code', platform: 'anthropic', type: 'AI 编程' },
  { id: 'grok', label: 'Grok', platform: 'grok', type: '通用 AI' },
  { id: 'deepseek', label: 'DeepSeek', platform: 'deepseek', type: '通用 AI' },
] as const
export type Tool = typeof tools[number]
export function toolIdsForGroup(group: Group, metric?: MonitorV4Group): string[] {
  if (metric?.tool_ids?.length) return metric.tool_ids
  const fallback = tools.find(tool => tool.platform === group.platform)
  return fallback ? [fallback.id] : []
}
export function linkedCounts(keys: ApiKey[]): Map<number, number> {
  const counts = new Map<number,number>()
  for (const key of keys) if (key.group_id) counts.set(key.group_id,(counts.get(key.group_id)||0)+1)
  return counts
}
export function configuredLines(groups: Group[], keys: ApiKey[]): Group[] {
  const byID = new Map(groups.map(group=>[group.id,group]))
  for (const key of keys) if (key.group && !byID.has(key.group.id)) byID.set(key.group.id,key.group)
  const counts = linkedCounts(keys)
  return [...byID.values()].filter(group=>counts.has(group.id))
}
export function availability(group: Group, metric?: MonitorV4Group, now = Date.now()) {
  if (group.status !== 'active') return { kind: 'muted', text: '停用', available: false }
  const observed = metric?.source_updated_at ? Date.parse(metric.source_updated_at) : NaN
  if (!metric || !Number.isFinite(observed) || observed>now || now-observed > 300000) return {kind:'muted',text:'暂不可用',available:false}
  return metric.current_operational ? {kind:'success',text:'可用',available:true} : {kind:'danger',text:'不可用',available:false}
}
export function compareQuality(a: Group,b: Group, metrics: Map<number,MonitorV4Group>, rates: Record<number,number>,now=Date.now()): number {
  const am=metrics.get(a.id),bm=metrics.get(b.id)
  return Number(availability(b,bm,now).available)-Number(availability(a,am,now).available)
    || (bm?.success_rate??-1)-(am?.success_rate??-1)
    || (bm?.request_count??0)-(am?.request_count??0)
    || (am?.ttft_p50_ms??Infinity)-(bm?.ttft_p50_ms??Infinity)
    || (am?.latency_p50_ms??Infinity)-(bm?.latency_p50_ms??Infinity)
    || (rates[a.id]??a.rate_multiplier)-(rates[b.id]??b.rate_multiplier) || a.id-b.id
}
export const metricLabel = (ms?: number|null) => ms == null ? '—' : `${(ms/1000).toFixed(2)}s`
export const providerIcon = (platform: string) => `/xingqiao/providers/${platform === 'grok' ? 'xai' : platform}.svg`
export const platformLabel = (platform: string) => ({openai:'OpenAI',anthropic:'Anthropic',grok:'xAI',deepseek:'DeepSeek'}[platform]||platform)
