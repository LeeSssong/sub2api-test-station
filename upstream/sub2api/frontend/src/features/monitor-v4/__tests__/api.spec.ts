import { describe, expect, it } from 'vitest'
import { validateMonitorV4Snapshot } from '../api'

const group = {
  id: 1, name: 'Primary', platform: 'openai', rate_multiplier: 0.3,
  success_rate: 75, request_count: 20, success_count: 15, real_request_count: 15, real_success_count: 14,
  probe_fallback_bucket_count: 5, probe_fallback_request_count: 5,
  ttft_p95_ms: 120, ttft_sample_count: 12, latency_p95_ms: 900, latency_sample_count: 12,
  cache_hit_rate: 0.4,
  source_updated_at: '2026-08-25T00:00:00Z', current_operational: true,
}

describe('monitor v4 contract', () => {
  it('accepts request-weighted metrics and nullable cache hit rates', () => {
    const snapshot = validateMonitorV4Snapshot({ contract_version: '2', window: '7d', refresh_interval_seconds: 60, generated_at: '2026-08-25T00:00:00Z', groups: [group, { ...group, id: 2, request_count: 0, success_count: 0, real_request_count: 0, real_success_count: 0, probe_fallback_bucket_count: 0, probe_fallback_request_count: 0, success_rate: null, ttft_p95_ms: null, ttft_sample_count: 0, latency_p95_ms: null, latency_sample_count: 0, cache_hit_rate: null }] })
    expect(snapshot.groups[1].success_rate).toBeNull()
    expect(snapshot.groups[0].cache_hit_rate).toBe(0.4)
  })

  it('rejects out-of-range success rate and old contract', () => {
    expect(() => validateMonitorV4Snapshot({ contract_version: '2', window: '7d', refresh_interval_seconds: 60, generated_at: '2026-08-25T00:00:00Z', groups: [{ ...group, success_rate: 101 }] })).toThrow()
    expect(() => validateMonitorV4Snapshot({ contract_version: '2', window: '7d', refresh_interval_seconds: 60, generated_at: '2026-08-25T00:00:00Z', groups: [{ ...group, request_count: 21 }] })).toThrow()
    expect(() => validateMonitorV4Snapshot({ contract_version: '2', window: '7d', refresh_interval_seconds: 60, generated_at: '2026-08-25T00:00:00Z', groups: [{ ...group, cache_hit_rate: 1.1 }] })).toThrow()
    expect(() => validateMonitorV4Snapshot({ contract_version: '1', window: '7d', refresh_interval_seconds: 60, generated_at: '2026-08-25T00:00:00Z', groups: [group] })).toThrow()
  })
})
