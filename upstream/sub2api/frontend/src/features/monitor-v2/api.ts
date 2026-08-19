import { apiClient } from '@/api/client'
import {
  MONITOR_V2_CONTRACT_VERSION,
  type MonitorV2Group,
  type MonitorV2GroupStatus,
  type MonitorV2Metric,
  type MonitorV2MetricState,
  type MonitorV2RefreshIntervalSeconds,
  type MonitorV2Snapshot,
  type MonitorV2TimelinePoint,
  type MonitorV2Window,
} from './types'

export class MonitorV2ContractError extends Error {
  constructor(message: string) {
    super(`Monitor V2 contract error: ${message}`)
    this.name = 'MonitorV2ContractError'
  }
}

const WINDOWS = new Set<MonitorV2Window>(['24h', '7d', '30d'])
const REFRESH_INTERVALS = new Set<MonitorV2RefreshIntervalSeconds>([0, 30, 60, 300, 600])
const METRIC_STATES = new Set<MonitorV2MetricState>([
  'available',
  'insufficient_data',
  'not_provided',
])
const GROUP_STATUSES = new Set<MonitorV2GroupStatus>([
  'operational',
  'unavailable',
])
const MAX_GROUPS = 100
const MAX_TIMELINE_POINTS = 64
const MAX_TEXT_LENGTH = 256
const PEAK_TIME_PATTERN = /^(?:[01]\d|2[0-3]):[0-5]\d$/

function record(value: unknown, path: string): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new MonitorV2ContractError(`${path} must be an object`)
  }
  return value as Record<string, unknown>
}

function text(value: unknown, path: string, allowEmpty = false): string {
  if (typeof value !== 'string' || (!allowEmpty && value.trim() === '')) {
    throw new MonitorV2ContractError(`${path} must be a string`)
  }
  if ([...value].length > MAX_TEXT_LENGTH) {
    throw new MonitorV2ContractError(`${path} must be at most ${MAX_TEXT_LENGTH} characters`)
  }
  return value
}

function finiteNumber(value: unknown, path: string, minimum = 0): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < minimum) {
    throw new MonitorV2ContractError(`${path} must be a finite number >= ${minimum}`)
  }
  return value
}

function integer(value: unknown, path: string, minimum = 0): number {
  const parsed = finiteNumber(value, path, minimum)
  if (!Number.isInteger(parsed)) {
    throw new MonitorV2ContractError(`${path} must be an integer`)
  }
  return parsed
}

function boolean(value: unknown, path: string): boolean {
  if (typeof value !== 'boolean') {
    throw new MonitorV2ContractError(`${path} must be a boolean`)
  }
  return value
}

function metric(
  value: unknown,
  path: string
): MonitorV2Metric {
  const source = record(value, path)
  for (const legacy of ['success_count', 'eligible_count', 'request_count']) {
    if (Object.prototype.hasOwnProperty.call(source, legacy)) {
      throw new MonitorV2ContractError(`${path}.${legacy} is not supported`)
    }
  }
  const state = text(source.state, `${path}.state`) as MonitorV2MetricState
  if (!METRIC_STATES.has(state)) {
    throw new MonitorV2ContractError(`${path}.state is unsupported`)
  }
  const sampleCount = integer(source.sample_count, `${path}.sample_count`)
  let metricValue: number | null = null
  if (source.value !== null) {
    metricValue = finiteNumber(source.value, `${path}.value`)
  }
  if (state === 'available' && metricValue === null) {
    throw new MonitorV2ContractError(`${path}.value is required when available`)
  }
  if (state === 'available' && sampleCount === 0) {
    throw new MonitorV2ContractError(`${path}.sample_count must be positive when available`)
  }
  if (state !== 'available' && metricValue !== null) {
    throw new MonitorV2ContractError(`${path}.value must be null when unavailable`)
  }
  if (state === 'not_provided' && sampleCount !== 0) {
    throw new MonitorV2ContractError(`${path}.sample_count must be zero when not provided`)
  }
  return {
    state,
    value: metricValue,
    sample_count: sampleCount,
  }
}

function timelinePoint(value: unknown, path: string): MonitorV2TimelinePoint {
  const source = record(value, path)
  for (const legacy of ['state', 'value', 'success_count', 'eligible_count']) {
    if (Object.prototype.hasOwnProperty.call(source, legacy)) {
      throw new MonitorV2ContractError(`${path}.${legacy} is not supported`)
    }
  }
  const status = text(source.status, `${path}.status`) as MonitorV2GroupStatus
  if (!GROUP_STATUSES.has(status)) throw new MonitorV2ContractError(`${path}.status is unsupported`)
  const bucketStart = text(source.bucket_start, `${path}.bucket_start`)
  if (Number.isNaN(Date.parse(bucketStart))) {
    throw new MonitorV2ContractError(`${path}.bucket_start must be RFC3339`)
  }
  return {
    bucket_start: bucketStart,
    status,
    latency_ms: source.latency_ms == null ? null : finiteNumber(source.latency_ms, `${path}.latency_ms`),
  }
}

function group(value: unknown, path: string): MonitorV2Group {
  const source = record(value, path)
  const status = text(source.status, `${path}.status`) as MonitorV2GroupStatus
  if (!GROUP_STATUSES.has(status)) {
    throw new MonitorV2ContractError(`${path}.status is unsupported`)
  }
  if (!Array.isArray(source.timeline)) {
    throw new MonitorV2ContractError(`${path}.timeline must be an array`)
  }
  if (source.timeline.length > MAX_TIMELINE_POINTS) {
    throw new MonitorV2ContractError(
      `${path}.timeline must contain at most ${MAX_TIMELINE_POINTS} points`
    )
  }
  for (const legacy of ['models', 'cache_hit', 'is_flagship', 'ttft_p95', 'tps', 'latency', 'latency_p95']) {
    if (Object.prototype.hasOwnProperty.call(source, legacy)) {
      throw new MonitorV2ContractError(`${path}.${legacy} is not supported`)
    }
  }
  const peakRateEnabled = boolean(source.peak_rate_enabled, `${path}.peak_rate_enabled`)
  const peakStart = text(source.peak_start ?? '', `${path}.peak_start`, true)
  const peakEnd = text(source.peak_end ?? '', `${path}.peak_end`, true)
  const peakRateMultiplier = finiteNumber(
    source.peak_rate_multiplier ?? 0,
    `${path}.peak_rate_multiplier`
  )
  if (peakRateEnabled) {
    if (!PEAK_TIME_PATTERN.test(peakStart)) {
      throw new MonitorV2ContractError(`${path}.peak_start must use HH:MM`)
    }
    if (!PEAK_TIME_PATTERN.test(peakEnd)) {
      throw new MonitorV2ContractError(`${path}.peak_end must use HH:MM`)
    }
    const startMinutes = Number(peakStart.slice(0, 2)) * 60 + Number(peakStart.slice(3))
    const endMinutes = Number(peakEnd.slice(0, 2)) * 60 + Number(peakEnd.slice(3))
    if (startMinutes >= endMinutes) {
      throw new MonitorV2ContractError(`${path}.peak_end must be after peak_start`)
    }
  }
  return {
    id: integer(source.id, `${path}.id`, 1),
    name: text(source.name, `${path}.name`),
    platform: text(source.platform, `${path}.platform`, true),
    rate_multiplier: finiteNumber(source.rate_multiplier, `${path}.rate_multiplier`),
    peak_rate_enabled: peakRateEnabled,
    peak_start: peakStart,
    peak_end: peakEnd,
    peak_rate_multiplier: peakRateMultiplier,
    status,
    availability: metric(source.availability, `${path}.availability`),
    ttft: metric(source.ttft, `${path}.ttft`),
    average_latency: metric(source.average_latency, `${path}.average_latency`),
    timeline: source.timeline.map((point, index) =>
      timelinePoint(point, `${path}.timeline[${index}]`)
    ),
  }
}

export function validateMonitorV2Snapshot(value: unknown): MonitorV2Snapshot {
  const source = record(value, 'snapshot')
  if (source.contract_version !== MONITOR_V2_CONTRACT_VERSION) {
    throw new MonitorV2ContractError('unsupported contract_version')
  }
  const window = text(source.window, 'window') as MonitorV2Window
  if (!WINDOWS.has(window)) {
    throw new MonitorV2ContractError('window is unsupported')
  }
  const refreshIntervalSeconds = integer(source.refresh_interval_seconds, 'refresh_interval_seconds')
  if (!REFRESH_INTERVALS.has(refreshIntervalSeconds as MonitorV2RefreshIntervalSeconds)) {
    throw new MonitorV2ContractError('refresh_interval_seconds is unsupported')
  }
  const generatedAt = text(source.generated_at, 'generated_at')
  if (Number.isNaN(Date.parse(generatedAt))) {
    throw new MonitorV2ContractError('generated_at must be RFC3339')
  }
  if (!Array.isArray(source.groups)) {
    throw new MonitorV2ContractError('groups must be an array')
  }
  if (source.groups.length > MAX_GROUPS) {
    throw new MonitorV2ContractError(`groups must contain at most ${MAX_GROUPS} items`)
  }
  const groups = source.groups.map((entry, index) => group(entry, `groups[${index}]`))
  const ids = new Set<number>()
  for (const item of groups) {
    if (ids.has(item.id)) {
      throw new MonitorV2ContractError(`duplicate group id ${item.id}`)
    }
    ids.add(item.id)
  }
  return {
    contract_version: MONITOR_V2_CONTRACT_VERSION,
    window,
    refresh_interval_seconds: refreshIntervalSeconds as MonitorV2RefreshIntervalSeconds,
    generated_at: generatedAt,
    groups,
  }
}

export async function getMonitorV2Snapshot(
  window: MonitorV2Window,
  signal?: AbortSignal
): Promise<MonitorV2Snapshot> {
  const { data } = await apiClient.get<unknown>('/monitor-v2', {
    params: { window },
    signal,
  })
  return validateMonitorV2Snapshot(data)
}
