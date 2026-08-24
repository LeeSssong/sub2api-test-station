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
  const availability = number(source.availability, `${path}.availability`)
  if (availability > 100) throw new MonitorV4ContractError(`${path}.availability must be <= 100`)
  const total = number(source.total_bucket_count, `${path}.total_bucket_count`, true)
  const available = number(source.availability_bucket_count, `${path}.availability_bucket_count`, true)
  if (available > total) throw new MonitorV4ContractError(`${path}.availability buckets exceed total`)
  const sourceUpdatedAt = source.source_updated_at == null || source.source_updated_at === '' ? null : text(source.source_updated_at, `${path}.source_updated_at`)
  if (typeof source.current_operational !== 'boolean' || typeof source.is_fallback_metric !== 'boolean') throw new MonitorV4ContractError(`${path} status flags are invalid`)
  return {
    id, name, platform,
    rate_multiplier: number(source.rate_multiplier, `${path}.rate_multiplier`),
    availability, availability_bucket_count: available, total_bucket_count: total,
    ttft_p95_ms: number(source.ttft_p95_ms, `${path}.ttft_p95_ms`),
    latency_p95_ms: number(source.latency_p95_ms, `${path}.latency_p95_ms`),
    sample_count: number(source.sample_count, `${path}.sample_count`, true),
    source_updated_at: sourceUpdatedAt,
    current_operational: source.current_operational,
    is_fallback_metric: source.is_fallback_metric,
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
