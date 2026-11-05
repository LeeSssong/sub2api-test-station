import type { MonitorRange } from '@/api/channelMonitorV2'

export const monitorPeriods = (['90m', '24h', '7d', '30d'] as const).map(value => ({
  value, label: `channelMonitorV2.ranges.${value}`,
}))

export function parseMonitorRange(value: unknown): MonitorRange {
  if (value === '1h') return '90m' // Retain existing bookmarked URLs.
  return monitorPeriods.some(period => period.value === value) ? value as MonitorRange : '24h'
}
