export const MONITOR_V4_CONTRACT_VERSION = '2' as const
export type MonitorV4Window = '24h' | '7d' | '30d'
export type MonitorV4RefreshIntervalSeconds = 0 | 30 | 60 | 300 | 600

export interface MonitorV4Group {
  id: number
  name: string
  platform: string
  rate_multiplier: number
  success_rate: number | null
  request_count: number
  success_count: number
  real_request_count: number
  real_success_count: number
  probe_fallback_bucket_count: number
  probe_fallback_request_count: number
  ttft_p95_ms: number | null
  ttft_sample_count: number
  latency_p95_ms: number | null
  latency_sample_count: number
  cache_read_tokens_p95: number | null
  cache_read_tokens_sample_count: number
  source_updated_at: string | null
  current_operational: boolean
}

export interface MonitorV4Snapshot {
  contract_version: typeof MONITOR_V4_CONTRACT_VERSION
  window: MonitorV4Window
  refresh_interval_seconds: MonitorV4RefreshIntervalSeconds
  generated_at: string
  groups: MonitorV4Group[]
}
