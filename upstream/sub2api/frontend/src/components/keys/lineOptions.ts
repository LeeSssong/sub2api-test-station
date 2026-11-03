import type { Group } from '@/types'
import type { LineSelectionMetric } from '@/features/monitor-v4/lineSelection'
import { firstTokenSeverity, type LatencySeverity } from '@/utils/latencyHealth'
import { metricLabel, toolIdsForGroup, routeCountHealth, routeHealthTone, routeSuccessLabel } from '@/features/ai-tools/model'
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
}

export function resolveLineRate(group: Group, rates: Record<number, number>): number | null {
  const userRate = rates[group.id]
  const rate = Number.isFinite(userRate) ? userRate : group.rate_multiplier
  return Number.isFinite(rate) ? rate : null
}

export function formatLineRate(rate: number | null): string {
  return formatMultiplierLabel(rate)
}

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
      const health = routeCountHealth(unavailable ? undefined : metric?.sla_success_count ?? undefined, unavailable ? undefined : metric?.sla_request_count ?? undefined)
      const ttft = !unavailable && metric?.ttft_p50_ms != null && Number.isFinite(metric.ttft_p50_ms) && metric.ttft_p50_ms >= 0 ? metric.ttft_p50_ms : null
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
        healthKind: health.kind,
        statusLabel: metricsError ? '统计读取失败' : expired ? '统计已过期' : health.text,
        successLabel: health.rate === null ? '—' : routeSuccessLabel(health),
        successTone: routeHealthTone(health),
        ttftLabel: metricLabel(ttft)
      }
    })
}
