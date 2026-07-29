import { apiClient } from '@/api/client'
import {
  MONITOR_V2_CONTRACT_VERSION,
  type MonitorV2Availability,
  type MonitorV2Group,
  type MonitorV2GroupStatus,
  type MonitorV2Metric,
  type MonitorV2MetricState,
  type MonitorV2Model,
  type MonitorV2ModelStatus,
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
const METRIC_STATES = new Set<MonitorV2MetricState>([
  'available',
  'insufficient_data',
  'not_provided',
])
const GROUP_STATUSES = new Set<MonitorV2GroupStatus>([
  'operational',
  'degraded',
  'unavailable',
  'unconfigured',
  'insufficient_data',
])
const MODEL_STATUSES = new Set<MonitorV2ModelStatus>(GROUP_STATUSES)
const MAX_GROUPS = 100
const MAX_MODELS_PER_GROUP = 200
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
  path: string,
  percent = false
): MonitorV2Metric {
  const source = record(value, path)
  const state = text(source.state, `${path}.state`) as MonitorV2MetricState
  if (!METRIC_STATES.has(state)) {
    throw new MonitorV2ContractError(`${path}.state is unsupported`)
  }
  const sampleCount = integer(source.sample_count, `${path}.sample_count`)
  let metricValue: number | null = null
  if (source.value !== null) {
    metricValue = finiteNumber(source.value, `${path}.value`)
    if (percent && metricValue > 100) {
      throw new MonitorV2ContractError(`${path}.value must be between 0 and 100`)
    }
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

function availability(value: unknown, path: string): MonitorV2Availability {
  const source = record(value, path)
  const parsed = metric(source, path, true)
  const successCount = integer(source.success_count, `${path}.success_count`)
  const eligibleCount = integer(source.eligible_count, `${path}.eligible_count`)
  if (successCount > eligibleCount) {
    throw new MonitorV2ContractError(`${path}.success_count exceeds eligible_count`)
  }
  if (parsed.sample_count !== eligibleCount) {
    throw new MonitorV2ContractError(`${path}.sample_count must equal eligible_count`)
  }
  if (parsed.state === 'available') {
    const expected = (successCount / eligibleCount) * 100
    if (Math.abs((parsed.value ?? 0) - expected) > 0.0001) {
      throw new MonitorV2ContractError(`${path}.value disagrees with call counts`)
    }
  } else if (eligibleCount > 0) {
    throw new MonitorV2ContractError(`${path}.state disagrees with eligible_count`)
  }
  return {
    ...parsed,
    success_count: successCount,
    eligible_count: eligibleCount,
  }
}

function timelinePoint(value: unknown, path: string): MonitorV2TimelinePoint {
  const source = record(value, path)
  const state = text(source.state, `${path}.state`) as MonitorV2MetricState
  if (!METRIC_STATES.has(state)) {
    throw new MonitorV2ContractError(`${path}.state is unsupported`)
  }
  let timelineValue: number | null = null
  if (source.value !== null) {
    timelineValue = finiteNumber(source.value, `${path}.value`)
    if (timelineValue > 100) {
      throw new MonitorV2ContractError(`${path}.value must be between 0 and 100`)
    }
  }
  if (state === 'available' && timelineValue === null) {
    throw new MonitorV2ContractError(`${path}.value is required when available`)
  }
  if (state !== 'available' && timelineValue !== null) {
    throw new MonitorV2ContractError(`${path}.value must be null when unavailable`)
  }
  const successCount = integer(source.success_count, `${path}.success_count`)
  const eligibleCount = integer(source.eligible_count, `${path}.eligible_count`)
  if (successCount > eligibleCount) {
    throw new MonitorV2ContractError(`${path}.success_count exceeds eligible_count`)
  }
  if (state === 'available') {
    if (eligibleCount === 0) {
      throw new MonitorV2ContractError(`${path}.eligible_count must be positive when available`)
    }
    const expected = (successCount / eligibleCount) * 100
    if (Math.abs((timelineValue ?? 0) - expected) > 0.0001) {
      throw new MonitorV2ContractError(`${path}.value disagrees with call counts`)
    }
  } else if (eligibleCount > 0) {
    throw new MonitorV2ContractError(`${path}.state disagrees with eligible_count`)
  }
  const bucketStart = text(source.bucket_start, `${path}.bucket_start`)
  if (Number.isNaN(Date.parse(bucketStart))) {
    throw new MonitorV2ContractError(`${path}.bucket_start must be RFC3339`)
  }
  return {
    bucket_start: bucketStart,
    state,
    value: timelineValue,
    success_count: successCount,
    eligible_count: eligibleCount,
  }
}

function model(value: unknown, path: string): MonitorV2Model {
  const source = record(value, path)
  const status = text(source.status, `${path}.status`) as MonitorV2ModelStatus
  if (!MODEL_STATUSES.has(status)) {
    throw new MonitorV2ContractError(`${path}.status is unsupported`)
  }
  return {
    name: text(source.name, `${path}.name`),
    status,
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
  if (!Array.isArray(source.models)) {
    throw new MonitorV2ContractError(`${path}.models must be an array`)
  }
  if (source.models.length > MAX_MODELS_PER_GROUP) {
    throw new MonitorV2ContractError(
      `${path}.models must contain at most ${MAX_MODELS_PER_GROUP} models`
    )
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
    availability: availability(source.availability, `${path}.availability`),
    ttft: metric(source.ttft, `${path}.ttft`),
    ttft_p95: metric(source.ttft_p95, `${path}.ttft_p95`),
    tps: metric(source.tps, `${path}.tps`),
    latency: metric(source.latency, `${path}.latency`),
    latency_p95: metric(source.latency_p95, `${path}.latency_p95`),
    cache_hit: metric(source.cache_hit, `${path}.cache_hit`, true),
    timeline: source.timeline.map((point, index) =>
      timelinePoint(point, `${path}.timeline[${index}]`)
    ),
    models: source.models.map((entry, index) => model(entry, `${path}.models[${index}]`)),
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
