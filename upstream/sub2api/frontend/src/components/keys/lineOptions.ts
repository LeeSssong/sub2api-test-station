import type { Group } from '@/types'
import type { MonitorV4Group } from '@/features/monitor-v4/types'
import { metricLabel, toolIdsForGroup } from '@/features/ai-tools/model'
import { successRateTone } from '@/features/monitor-v4/successRate'

export interface LineOption {
  [key: string]: unknown
  value: number
  label: string
  group: Group
  description: string | null
  platform: Group['platform']
  rate: number | null
  rateLabel: string
  linkedCount: number | null
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
  return rate == null ? '倍率暂不可用' : (Number.isInteger(rate) ? rate.toFixed(1) : String(rate)) + '倍率'
}

export function buildLineOptions(
  groups: Group[],
  rates: Record<number, number>,
  metrics: Map<number, MonitorV4Group>,
  linkedCounts: Map<number, number> | null,
  toolId?: string
): LineOption[] {
  return groups
    .filter(group => !toolId || (group.status === 'active' && toolIdsForGroup(group, metrics.get(group.id)).includes(toolId)))
    .map(group => {
      const rate = resolveLineRate(group, rates)
      const metric = metrics.get(group.id)
      return {
        value: group.id,
        label: group.name,
        group,
        description: group.description,
        platform: group.platform,
        rate,
        rateLabel: formatLineRate(rate),
        linkedCount: linkedCounts?.get(group.id) ?? (linkedCounts ? 0 : null),
        statusLabel: group.status === 'active' ? '管理正常' : '已停用',
        successLabel: metric?.success_rate == null ? '—' : `${Number(metric.success_rate.toFixed(1))}%`,
        successTone: successRateTone(group.status === 'active' && metric?.request_count !== 0 ? metric?.success_rate : null),
        ttftLabel: metricLabel(metric?.ttft_p50_ms)
      }
    })
}
