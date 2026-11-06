import { getMatrix, type HealthState } from '@/api/channelMonitorV2'
import { hasMonitorSamples } from '@/features/channel-monitor-v2/monitorCards'
import { getPublicSettings } from '@/api/auth'
import { list as getProbeMonitors } from '@/api/channelMonitor'
import { getStatus as getV3Status } from '@/api/channelMonitorV3'
import type { PublicSettings } from '@/types'

/** Key selectors follow the monitor implementation used by route details. */
export interface LineSelectionMetric {
  id?: number
  has_samples?: boolean
  health?: HealthState
  availability_health?: HealthState
  error_rate?: number
  ttft_p50_ms?: number | null
  status_label?: string
  availability_percent?: number | null
  availability_label?: string
  availability_title?: string
  ttft_title?: string
  source?: 'v1' | 'v3'
}
export interface LineSelectionSnapshot {
  generated_at: string
  groups: (LineSelectionMetric & { id: number })[]
}
const healthStates = new Set<HealthState>(['unknown', 'healthy', 'warning', 'critical'])
function invalid(): never { throw new Error('线路详情统计数据无效') }

export async function getLineSelectionMetrics(
  signal?: AbortSignal,
  groupIds: number[] = [],
  toolId?: string,
): Promise<LineSelectionSnapshot> {
  if (groupIds.some(id => !Number.isSafeInteger(id) || id <= 0)) return invalid()
  const ids = [...new Set(groupIds)]
  // An empty filter means all groups to the API. Never broaden an empty selector.
  if (!ids.length) return { generated_at: new Date().toISOString(), groups: [] }
  let settings = await getPublicSettings(signal)
  signal?.throwIfAborted()
  try { return await readMode(settings, ids, signal, toolId) }
  catch (error) {
    signal?.throwIfAborted()
    // A mode switch may race the settings read. Retry only that explicit case,
    // once, after re-reading settings; unrelated failures remain visible.
    if (!error || typeof error !== 'object' || !('code' in error) || !['CHANNEL_MONITOR_MODE_MISMATCH', 'CHANNEL_MONITOR_DISABLED'].includes(String(error.code))) throw error
    const latest = await getPublicSettings(signal)
    signal?.throwIfAborted()
    if (latest.channel_monitor_mode === settings.channel_monitor_mode && latest.channel_monitor_enabled === settings.channel_monitor_enabled) throw error
    settings = latest
    return readMode(settings, ids, signal, toolId)
  }
}

async function readMode(settings: PublicSettings, ids: number[], signal?: AbortSignal, toolId?: string): Promise<LineSelectionSnapshot> {
  if (settings.channel_monitor_enabled === false) return { generated_at: new Date().toISOString(), groups: ids.map(id => ({ id, has_samples: false, status_label: '监控未开启', availability_label: '可用性', availability_title: '监控未开启', ttft_title: '监控未开启' })) }
  switch (settings.channel_monitor_mode || 'v1') {
    case 'v1': case 'native_probe': return readV1(ids, signal, toolId)
    case 'v2': return readV2(ids, signal, toolId)
    case 'v3': return readV3(ids, signal, toolId)
    default: return invalid()
  }
}

async function readV2(ids: number[], signal?: AbortSignal, toolId?: string): Promise<LineSelectionSnapshot> {
  const matrix = await getMatrix({ range: '24h', platforms: [], groupIds: ids, models: [], toolId }, 'platform_group', false, signal)
  if (!matrix || !Array.isArray(matrix.items) || !Number.isFinite(Date.parse(matrix.coverage?.computed_at))) return invalid()
  const visible = new Set(ids), seen = new Set<number>()
  const groups: LineSelectionSnapshot['groups'] = []
  for (const row of matrix.items) {
    if (!row.group_id || !visible.has(row.group_id)) continue
    if (seen.has(row.group_id) || !row.metrics || !row.health || !healthStates.has(row.health.overall) || !healthStates.has(row.health.error_rate)) return invalid()
    seen.add(row.group_id)
    const sampled = hasMonitorSamples(row.metrics)
    const rate = row.metrics.error_rate
    const ttft = sampled ? row.metrics.ttft?.p50_ms ?? null : null
    if (sampled && (!Number.isFinite(rate) || rate < 0 || rate > 1)) return invalid()
    if (ttft !== null && (!Number.isFinite(ttft) || ttft < 0)) return invalid()
    groups.push({ id: row.group_id, has_samples: sampled, health: sampled ? row.health.overall : 'unknown', availability_health: sampled ? row.health.error_rate : 'unknown', error_rate: rate, ttft_p50_ms: ttft })
  }
  return { generated_at: matrix.coverage.computed_at, groups }
}

const probeHealth = { operational: 'healthy', degraded: 'warning', failed: 'critical', error: 'critical', '': 'unknown' } as const
const probeLabels = { operational: '正常', degraded: '降级', failed: '失败', error: '错误', '': '暂无数据' } as const
const v3Health = { operational: 'healthy', degraded: 'warning', down: 'critical', unknown: 'unknown' } as const
const v3Labels = { operational: '正常', degraded: '性能下降', down: '服务异常', unknown: '暂无近期数据' } as const
function percent(value: number | null | undefined): number | null {
  if (value == null) return null
  if (!Number.isFinite(value) || value < 0 || value > 100) return invalid()
  return value
}
function ambiguous(id: number, source: 'v1' | 'v3', label: string): LineSelectionSnapshot['groups'][number] {
  return { id, source, has_samples: false, status_label: '多项监控，查看详情', availability_percent: null, ttft_p50_ms: null, availability_label: label, availability_title: '同一线路对应多项监控，请在线路详情中分别查看', ttft_title: '同一线路对应多项监控，请在线路详情中分别查看' }
}

async function readV1(ids: number[], signal?: AbortSignal, toolId?: string): Promise<LineSelectionSnapshot> {
  const result = await getProbeMonitors({ signal, toolId })
  if (!result || !Array.isArray(result.items)) return invalid()
  const groups: LineSelectionSnapshot['groups'] = []
  for (const id of ids) {
    const rows = result.items.filter(item => item.group_id === id)
    if (!rows.length) continue
    if (rows.length > 1) { groups.push(ambiguous(id, 'v1', '可用性 · 近7天')); continue }
    const row = rows[0]
    if (!Object.prototype.hasOwnProperty.call(probeHealth, row.primary_status)) return invalid()
    groups.push({ id, source: 'v1', has_samples: !!row.primary_status || row.has_samples_7d === true, health: probeHealth[row.primary_status], status_label: probeLabels[row.primary_status], availability_percent: row.has_samples_7d === true ? percent(row.availability_7d) : null, availability_label: '可用性 · 近7天', availability_title: '近7天主动探测可用率，与线路详情一致', ttft_p50_ms: null, ttft_title: '当前监控未提供首字统计，探测耗时不等于首字响应时间' })
  }
  return { generated_at: new Date().toISOString(), groups }
}

async function readV3(ids: number[], signal?: AbortSignal, toolId?: string): Promise<LineSelectionSnapshot> {
  const result = await getV3Status(null, false, signal, toolId)
  if (!result || !Array.isArray(result.categories) || !Number.isFinite(Date.parse(result.generated_at)) || !['24h', '7d', '30d'].includes(result.availability_range)) return invalid()
  const rows = result.categories.flatMap(category => {
    if (!Array.isArray(category.components)) return invalid()
    return category.components
  })
  if (result.featured && !rows.some(row => row.id === result.featured!.id)) rows.push(result.featured)
  const range = { '24h': '近24小时', '7d': '近7天', '30d': '近30天' }[result.availability_range]
  const groups: LineSelectionSnapshot['groups'] = []
  for (const id of ids) {
    const matching = rows.filter(row => row.group_id === id)
    if (!matching.length) continue
    if (matching.length > 1) { groups.push(ambiguous(id, 'v3', `可用性 · ${range}`)); continue }
    const row = matching[0]
    if (!Object.prototype.hasOwnProperty.call(v3Health, row.status)) return invalid()
    const availability = percent(row.availability)
    groups.push({ id, source: 'v3', has_samples: row.status !== 'unknown' || availability !== null, health: v3Health[row.status], status_label: v3Labels[row.status], availability_percent: availability, availability_label: `可用性 · ${range}`, availability_title: `${range}组件可用率，与线路详情一致${row.model ? `（模型：${row.model}）` : ''}`, ttft_p50_ms: null, ttft_title: '当前监控未提供线路级首字统计，单个时段数据不代表整条线路' })
  }
  return { generated_at: result.generated_at, groups }
}
