export const MONITOR_V2_CONTRACT_VERSION = '2' as const

export type MonitorV2Window = '24h' | '7d' | '30d'
export type MonitorV2MetricState = 'available' | 'insufficient_data' | 'not_provided'
export type MonitorV2GroupStatus =
  | 'operational'
  | 'degraded'
  | 'unavailable'
  | 'unconfigured'
  | 'insufficient_data'
export type MonitorV2ModelStatus = MonitorV2GroupStatus

export interface MonitorV2Metric {
  state: MonitorV2MetricState
  value: number | null
  sample_count: number
}

export interface MonitorV2Availability extends MonitorV2Metric {
  success_count: number
  eligible_count: number
}

export interface MonitorV2TimelinePoint {
  bucket_start: string
  state: MonitorV2MetricState
  value: number | null
  success_count: number
  eligible_count: number
}

export interface MonitorV2Model {
  name: string
  status: MonitorV2ModelStatus
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
  status: MonitorV2GroupStatus
  availability: MonitorV2Availability
  ttft: MonitorV2Metric
  ttft_p95: MonitorV2Metric
  tps: MonitorV2Metric
  latency: MonitorV2Metric
  latency_p95: MonitorV2Metric
  cache_hit: MonitorV2Metric
  timeline: MonitorV2TimelinePoint[]
  models: MonitorV2Model[]
}

export interface MonitorV2Snapshot {
  contract_version: typeof MONITOR_V2_CONTRACT_VERSION
  window: MonitorV2Window
  generated_at: string
  groups: MonitorV2Group[]
}
