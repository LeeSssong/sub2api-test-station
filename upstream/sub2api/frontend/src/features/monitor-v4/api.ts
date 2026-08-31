import { apiClient } from '@/api/client'
import { MONITOR_V4_CONTRACT_VERSION, type MonitorV4Group, type MonitorV4RefreshIntervalSeconds, type MonitorV4Snapshot, type MonitorV4Window } from './types'

export class MonitorV4ContractError extends Error {
  constructor(message: string) { super(`Monitor V4 contract error: ${message}`); this.name = 'MonitorV4ContractError' }
}

const WINDOWS = new Set<MonitorV4Window>(['24h', '7d', '30d'])
const REFRESH = new Set<MonitorV4RefreshIntervalSeconds>([0, 30, 60, 300, 600])
const MAX_GROUPS = 100

function object(value: unknown, path: string): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) throw new MonitorV4ContractError(`${path} must be an object`)
  return value as Record<string, unknown>
}
function number(value: unknown, path: string, integer = false): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0 || (integer && !Number.isInteger(value))) throw new MonitorV4ContractError(`${path} must be a non-negative number`)
  return value
}
function nullableNumber(value: unknown, path: string): number | null {
  if (value === null) return null
  return number(value, path)
}
function text(value: unknown, path: string): string {
  if (typeof value !== 'string' || !value.trim() || Number.isNaN(Date.parse(value))) throw new MonitorV4ContractError(`${path} must be RFC3339 text`)
  return value
}
function group(value: unknown, path: string): MonitorV4Group {
  const source = object(value, path)
  const id = number(source.id, `${path}.id`, true)
  if (id <= 0) throw new MonitorV4ContractError(`${path}.id must be positive`)
  const name = typeof source.name === 'string' && source.name.trim() ? source.name : (() => { throw new MonitorV4ContractError(`${path}.name is required`) })()
  const platform = typeof source.platform === 'string' ? source.platform : ''
  const successRate = nullableNumber(source.success_rate, `${path}.success_rate`)
  if (successRate !== null && successRate > 100) throw new MonitorV4ContractError(`${path}.success_rate must be <= 100`)
  const requestCount = number(source.request_count, `${path}.request_count`, true)
  const successCount = number(source.success_count, `${path}.success_count`, true)
  const realRequestCount = number(source.real_request_count, `${path}.real_request_count`, true)
  const realSuccessCount = number(source.real_success_count, `${path}.real_success_count`, true)
  const probeFallbackBucketCount = number(source.probe_fallback_bucket_count, `${path}.probe_fallback_bucket_count`, true)
  const probeFallbackRequestCount = number(source.probe_fallback_request_count, `${path}.probe_fallback_request_count`, true)
  if (
    successCount > requestCount ||
    realSuccessCount > realRequestCount ||
    probeFallbackRequestCount !== probeFallbackBucketCount ||
    realRequestCount + probeFallbackRequestCount !== requestCount ||
    realSuccessCount > successCount ||
    successCount > realSuccessCount + probeFallbackRequestCount
  ) throw new MonitorV4ContractError(`${path} request counts are inconsistent`)
  if (successRate === null ? requestCount !== 0 : requestCount === 0) throw new MonitorV4ContractError(`${path}.success_rate does not match request_count`)
  const ttftP95 = nullableNumber(source.ttft_p95_ms, `${path}.ttft_p95_ms`)
  const latencyP95 = nullableNumber(source.latency_p95_ms, `${path}.latency_p95_ms`)
  const ttftSampleCount = number(source.ttft_sample_count, `${path}.ttft_sample_count`, true)
  const latencySampleCount = number(source.latency_sample_count, `${path}.latency_sample_count`, true)
  const cacheHitRate = nullableNumber(source.cache_hit_rate, `${path}.cache_hit_rate`)
  if ((ttftP95 === null) !== (ttftSampleCount === 0) || (latencyP95 === null) !== (latencySampleCount === 0)) throw new MonitorV4ContractError(`${path} P95 values do not match sample counts`)
  if (cacheHitRate !== null && cacheHitRate > 1) throw new MonitorV4ContractError(`${path}.cache_hit_rate must be <= 1`)
  const sourceUpdatedAt = source.source_updated_at == null || source.source_updated_at === '' ? null : text(source.source_updated_at, `${path}.source_updated_at`)
  if (typeof source.current_operational !== 'boolean') throw new MonitorV4ContractError(`${path} status flags are invalid`)
  return {
    id, name, platform,
    rate_multiplier: number(source.rate_multiplier, `${path}.rate_multiplier`),
    success_rate: successRate, request_count: requestCount, success_count: successCount,
    real_request_count: realRequestCount, real_success_count: realSuccessCount,
    probe_fallback_bucket_count: probeFallbackBucketCount, probe_fallback_request_count: probeFallbackRequestCount,
    ttft_p95_ms: ttftP95, ttft_sample_count: ttftSampleCount,
    latency_p95_ms: latencyP95, latency_sample_count: latencySampleCount,
    cache_hit_rate: cacheHitRate,
    source_updated_at: sourceUpdatedAt,
    current_operational: source.current_operational,
  }
}

export function validateMonitorV4Snapshot(value: unknown): MonitorV4Snapshot {
  const source = object(value, 'snapshot')
  if (source.contract_version !== MONITOR_V4_CONTRACT_VERSION) throw new MonitorV4ContractError('unsupported contract_version')
  const window = source.window as MonitorV4Window
  if (!WINDOWS.has(window)) throw new MonitorV4ContractError('window is unsupported')
  const refresh = number(source.refresh_interval_seconds, 'refresh_interval_seconds', true) as MonitorV4RefreshIntervalSeconds
  if (!REFRESH.has(refresh)) throw new MonitorV4ContractError('refresh interval is unsupported')
  const generatedAt = text(source.generated_at, 'generated_at')
  if (!Array.isArray(source.groups) || source.groups.length > MAX_GROUPS) throw new MonitorV4ContractError('groups must contain at most 100 items')
  const groups = source.groups.map((entry, index) => group(entry, `groups[${index}]`))
  const ids = new Set<number>(); groups.forEach(item => { if (ids.has(item.id)) throw new MonitorV4ContractError(`duplicate group id ${item.id}`); ids.add(item.id) })
  return { contract_version: MONITOR_V4_CONTRACT_VERSION, window, refresh_interval_seconds: refresh, generated_at: generatedAt, groups }
}

export async function getHybridPerformanceSnapshot(window: MonitorV4Window, signal?: AbortSignal): Promise<MonitorV4Snapshot> {
  const { data } = await apiClient.get<unknown>('/monitor-v4', { params: { window }, signal })
  return validateMonitorV4Snapshot(data)
}
