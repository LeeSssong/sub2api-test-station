import type { Group } from '@/types'
import type { LineSelectionMetric } from '@/features/monitor-v4/lineSelection'
import { firstTokenSeverity, type LatencySeverity } from '@/utils/latencyHealth'
import { toolIdsForGroup } from '@/features/ai-tools/model'
import { formatMonitorMs, formatMonitorPercent } from '@/features/channel-monitor-v2/monitorFormat'
import type { HealthState } from '@/api/channelMonitorV2'
import { formatMonitorV3Availability } from '@/features/channel-monitor-v3/monitorV3'
import { successRateTone } from '@/features/monitor-v4/successRate'
import { formatMultiplierLabel } from '@/utils/formatters'

export interface LineOption {
  [key: string]: unknown
  value: number
  label: string
  group: Group
  description: string | null
  platform: Group['platform']
  rate: number | null
  rateLabel: string
  disabled: boolean
  unavailableReason: string | null
  ttftSeverity: LatencySeverity | null
  healthKind: string
  statusLabel: string
  successLabel: string
  successTone: ReturnType<typeof successRateTone>
  ttftLabel: string
  availabilityLabel: string
  availabilityTitle: string
  ttftTitle: string
}

export function resolveLineRate(group: Group, rates: Record<number, number>): number | null {
  const userRate = rates[group.id]
  const rate = Number.isFinite(userRate) ? userRate : group.rate_multiplier
  return Number.isFinite(rate) ? rate : null
}

export function formatLineRate(rate: number | null): string {
  return formatMultiplierLabel(rate)
}

const healthLabels: Record<HealthState, string> = { healthy: '正常', warning: '波动', critical: '异常', unknown: '样本不足' }
const healthKinds = { healthy: 'success', warning: 'warning', critical: 'danger', unknown: 'unknown' } as const
const healthTones = { healthy: 'green', warning: 'amber', critical: 'red', unknown: 'muted' } as const

export function buildLineOptions(
  groups: Group[],
  rates: Record<number, number>,
  metrics: Map<number, LineSelectionMetric>,
  _linkedCounts: Map<number, number> | null,
  toolId?: string,
  metricsGeneratedAt?: string | null,
  now = Date.now(),
  metricsError = false
): LineOption[] {
  return groups
    .filter(group => !toolId || (group.status === 'active' && toolIdsForGroup(group).includes(toolId)))
    .map(group => {
      const rate = resolveLineRate(group, rates)
      const metric = metrics.get(group.id)
      const observed = metricsGeneratedAt ? Date.parse(metricsGeneratedAt) : NaN
      const expired = metricsGeneratedAt != null && (!Number.isFinite(observed) || observed > now || now - observed > 7 * 60 * 1000)
      const unavailable = metricsError || expired
      const sampled = !unavailable && metric?.has_samples === true
      const health = sampled ? metric.health ?? 'unknown' : 'unknown'
      const availability = sampled ? metric.availability_health ?? 'unknown' : 'unknown'
      const rateAvailable = sampled && typeof metric.error_rate === 'number' && Number.isFinite(metric.error_rate) && metric.error_rate >= 0 && metric.error_rate <= 1
      const sourcePercent = sampled && metric.availability_percent != null && Number.isFinite(metric.availability_percent) && metric.availability_percent >= 0 && metric.availability_percent <= 100 ? metric.availability_percent : null
      const ttft = sampled && metric?.ttft_p50_ms != null && Number.isFinite(metric.ttft_p50_ms) && metric.ttft_p50_ms >= 0 ? metric.ttft_p50_ms : null
      return {
        value: group.id,
        label: group.name,
        group,
        description: group.description,
        platform: group.platform,
        rate,
        rateLabel: formatLineRate(rate),
        disabled: group.status !== 'active',
        unavailableReason: group.status !== 'active' ? '线路已停用' : null,
        ttftSeverity: ttft === null ? null : firstTokenSeverity(ttft),
        healthKind: healthKinds[health],
        statusLabel: metricsError ? '统计读取失败' : expired ? '统计已过期' : metric?.status_label ?? (sampled ? healthLabels[health] : '暂无数据'),
        successLabel: sourcePercent !== null ? metric?.source === 'v1' ? `${sourcePercent.toFixed(2)}%` : formatMonitorV3Availability(sourcePercent) : rateAvailable ? formatMonitorPercent(1 - metric.error_rate!) : '—',
        successTone: healthTones[availability],
        ttftLabel: ttft === null ? '—' : formatMonitorMs(ttft),
        availabilityLabel: metric?.availability_label ?? '可用性',
        availabilityTitle: metric?.availability_title ?? (metric ? '近24小时可用性' : '当前监控暂无此线路统计'),
        ttftTitle: metric?.ttft_title ?? (metric ? '近24小时首字响应中位数（P50）' : '当前监控暂无此线路首字统计')
      }
    })
}
