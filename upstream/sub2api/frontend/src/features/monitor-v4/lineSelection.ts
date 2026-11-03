import { apiClient } from '@/api/client'

/** Independent from mixed real/probe monitor and ranking metrics. */
export interface LineSelectionMetric {
  id?: number
  sla_request_count?: number | null
  sla_success_count?: number | null
  ttft_p50_ms?: number | null
  ttft_sample_count?: number
}
export interface LineSelectionSnapshot {
  contract_version: '1'
  window: '24h'
  window_start: string
  window_end: string
  generated_at: string
  groups: (LineSelectionMetric & { id: number })[]
}
function invalid(): never { throw new Error('线路近24小时真实统计契约无效') }
function count(value: unknown): value is number { return Number.isSafeInteger(value) && (value as number) >= 0 }
export function validateLineSelectionMetrics(value: unknown): LineSelectionSnapshot {
  if (!value || typeof value !== 'object') return invalid()
  const s = value as LineSelectionSnapshot
  const start = Date.parse(s.window_start), end = Date.parse(s.window_end), generated = Date.parse(s.generated_at)
  if (s.contract_version !== '1' || s.window !== '24h' || !Number.isFinite(start) || end - start !== 86400000 || !Number.isFinite(generated) || generated < end || !Array.isArray(s.groups) || s.groups.length > 100) return invalid()
  const ids = new Set<number>()
  for (const g of s.groups) {
    if (!g || !count(g.id) || g.id <= 0 || ids.has(g.id) || !count(g.sla_request_count) || !count(g.sla_success_count) || g.sla_success_count > g.sla_request_count || !count(g.ttft_sample_count) || g.ttft_sample_count > g.sla_success_count) return invalid()
    if (g.ttft_sample_count === 0 ? g.ttft_p50_ms !== null : typeof g.ttft_p50_ms !== 'number' || !Number.isFinite(g.ttft_p50_ms) || g.ttft_p50_ms < 0) return invalid()
    ids.add(g.id)
  }
  return s
}
export async function getLineSelectionMetrics(signal?: AbortSignal): Promise<LineSelectionSnapshot> {
  const { data } = await apiClient.get<unknown>('/monitor-v4/line-selection', { signal })
  return validateLineSelectionMetrics(data)
}
