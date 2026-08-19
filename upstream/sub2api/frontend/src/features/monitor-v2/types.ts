export const MONITOR_V2_CONTRACT_VERSION = '6' as const

export type MonitorV2RefreshIntervalSeconds = 0 | 30 | 60 | 300 | 600

export type MonitorV2Window = '24h' | '7d' | '30d'
export type MonitorV2MetricState = 'available' | 'insufficient_data' | 'not_provided'
export type MonitorV2GroupStatus =
  | 'operational'
  | 'unavailable'
export interface MonitorV2Metric {
  state: MonitorV2MetricState
  value: number | null
  sample_count: number
}

export interface MonitorV2TimelinePoint {
  bucket_start: string
  status: MonitorV2GroupStatus
  latency_ms: number | null
}

export interface MonitorV2Group {
  id: number
  name: string
  platform: string
  rate_multiplier: number
  peak_rate_enabled: boolean
  peak_start: string
  peak_end: string
  peak_rate_multiplier: number
  is_flagship: boolean
  status: MonitorV2GroupStatus
  ttft: MonitorV2Metric
  ttft_p95: MonitorV2Metric
  tps: MonitorV2Metric
  latency: MonitorV2Metric
  latency_p95: MonitorV2Metric
  timeline: MonitorV2TimelinePoint[]
}

export interface MonitorV2Snapshot {
  contract_version: typeof MONITOR_V2_CONTRACT_VERSION
  window: MonitorV2Window
  refresh_interval_seconds: MonitorV2RefreshIntervalSeconds
  generated_at: string
  groups: MonitorV2Group[]
}
