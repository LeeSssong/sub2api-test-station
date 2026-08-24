import { describe, expect, it } from 'vitest'
import { validateMonitorV4Snapshot } from '../api'

const group = {
  id: 1, name: 'Primary', platform: 'openai', rate_multiplier: 0.3,
  availability: 85, availability_bucket_count: 17, total_bucket_count: 20,
  ttft_p95_ms: 120, latency_p95_ms: 900, sample_count: 12,
  source_updated_at: '2026-08-25T00:00:00Z', current_operational: true, is_fallback_metric: false,
}

describe('monitor v4 contract', () => {
  it('accepts concrete p95 metrics and zero samples', () => {
    const snapshot = validateMonitorV4Snapshot({ contract_version: '1', window: '7d', refresh_interval_seconds: 60, generated_at: '2026-08-25T00:00:00Z', groups: [group, { ...group, id: 2, sample_count: 0, ttft_p95_ms: 0, latency_p95_ms: 0 }] })
    expect(snapshot.groups[1].sample_count).toBe(0)
  })

  it('rejects out-of-range availability', () => {
    expect(() => validateMonitorV4Snapshot({ contract_version: '1', window: '7d', refresh_interval_seconds: 60, generated_at: '2026-08-25T00:00:00Z', groups: [{ ...group, availability: 101 }] })).toThrow()
  })
})
