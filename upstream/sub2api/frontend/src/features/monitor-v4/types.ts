export const MONITOR_V4_CONTRACT_VERSION = '1' as const
export type MonitorV4Window = '24h' | '7d' | '30d'
export type MonitorV4RefreshIntervalSeconds = 0 | 30 | 60 | 300 | 600

export interface MonitorV4Group {
  id: number
  name: string
  platform: string
  rate_multiplier: number
  availability: number
  availability_bucket_count: number
  total_bucket_count: number
  ttft_p95_ms: number
  latency_p95_ms: number
  sample_count: number
  source_updated_at?: string | null
  current_operational: boolean
  is_fallback_metric: boolean
}

export interface MonitorV4Snapshot {
  contract_version: typeof MONITOR_V4_CONTRACT_VERSION
  window: MonitorV4Window
  refresh_interval_seconds: MonitorV4RefreshIntervalSeconds
  generated_at: string
  groups: MonitorV4Group[]
}
