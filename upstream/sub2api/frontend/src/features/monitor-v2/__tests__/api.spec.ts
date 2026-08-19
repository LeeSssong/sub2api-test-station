import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/api/client', () => ({ apiClient: { get } }))

import { MonitorV2ContractError, getMonitorV2Snapshot, validateMonitorV2Snapshot } from '../api'

const metric = { state: 'available', value: 420, sample_count: 20 }
const validPayload = {
  contract_version: '6',
  refresh_interval_seconds: 300,
  window: '7d',
  generated_at: '2026-07-29T12:00:00Z',
  groups: [{
    id: 7,
    name: 'GPT-Pro',
    platform: 'openai',
    rate_multiplier: 1,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_flagship: true,
    status: 'operational',
    ttft: metric,
    ttft_p95: { ...metric, value: 880 },
    tps: { ...metric, value: 46.5 },
    latency: { ...metric, value: 1320 },
    latency_p95: { ...metric, value: 2400 },
    timeline: [{
      bucket_start: '2026-07-29T06:00:00Z',
      status: 'operational',
      latency_ms: 1320,
    }],
  }],
}

describe('Monitor V2 API contract', () => {
  beforeEach(() => get.mockReset())

  it('returns a validated version 6 snapshot without percentage fields', async () => {
    get.mockResolvedValue({ data: validPayload })

    const snapshot = await getMonitorV2Snapshot('7d')

    expect(get).toHaveBeenCalledWith('/monitor-v2', {
      params: { window: '7d' },
      signal: undefined,
    })
    expect(snapshot.groups[0]).toMatchObject({ name: 'GPT-Pro', is_flagship: true })
    expect(snapshot.groups[0].timeline[0].status).toBe('operational')
    expect(snapshot.groups[0]).not.toHaveProperty('availability')
    expect(snapshot.groups[0]).not.toHaveProperty('cache_hit')
    expect(snapshot.groups[0].timeline[0]).not.toHaveProperty('success_count')
  })

  it('rejects unsupported contract versions and non-binary states', () => {
    expect(() => validateMonitorV2Snapshot({ ...validPayload, contract_version: '5' }))
      .toThrow(MonitorV2ContractError)
    expect(() => validateMonitorV2Snapshot({
      ...validPayload,
      groups: [{ ...validPayload.groups[0], status: 'degraded' }],
    })).toThrow('status is unsupported')
  })

  it('rejects legacy percentage and request-result fields', () => {
    for (const legacy of [
      { availability: { state: 'available', value: 99, sample_count: 10, success_count: 9, eligible_count: 10 } },
      { cache_hit: { state: 'available', value: 40, sample_count: 10 } },
      { timeline: [{ ...validPayload.groups[0].timeline[0], success_count: 10 }] },
    ]) {
      expect(() => validateMonitorV2Snapshot({
        ...validPayload,
        groups: [{ ...validPayload.groups[0], ...legacy }],
      })).toThrow(MonitorV2ContractError)
    }
  })

  it('rejects invalid metrics, arrays, strings and refresh intervals', () => {
    expect(() => validateMonitorV2Snapshot({ ...validPayload, refresh_interval_seconds: 15 }))
      .toThrow('refresh_interval_seconds')
    expect(() => validateMonitorV2Snapshot({ ...validPayload, groups: {} }))
      .toThrow('groups')
    expect(() => validateMonitorV2Snapshot({
      ...validPayload,
      groups: [{ ...validPayload.groups[0], name: 'a'.repeat(257) }],
    })).toThrow('name')
    expect(() => validateMonitorV2Snapshot({
      ...validPayload,
      groups: [{ ...validPayload.groups[0], ttft: { ...metric, sample_count: 0 } }],
    })).toThrow('ttft.sample_count')
  })

  it('accepts configured refresh intervals', () => {
    for (const refreshIntervalSeconds of [0, 30, 60, 300, 600]) {
      expect(validateMonitorV2Snapshot({
        ...validPayload,
        refresh_interval_seconds: refreshIntervalSeconds,
      })).toMatchObject({ refresh_interval_seconds: refreshIntervalSeconds })
    }
  })

  it('rejects duplicate groups and oversized arrays', () => {
    expect(() => validateMonitorV2Snapshot({
      ...validPayload,
      groups: [validPayload.groups[0], validPayload.groups[0]],
    })).toThrow('duplicate group id')

    expect(() => validateMonitorV2Snapshot({
      ...validPayload,
      groups: Array.from({ length: 101 }, (_, index) => ({
        ...validPayload.groups[0],
        id: index + 1,
      })),
    })).toThrow('at most 100')

    expect(() => validateMonitorV2Snapshot({
      ...validPayload,
      groups: [{
        ...validPayload.groups[0],
        timeline: Array.from({ length: 65 }, (_, index) => ({
          ...validPayload.groups[0].timeline[0],
          bucket_start: new Date(Date.UTC(2026, 6, 1, 0, index)).toISOString(),
        })),
      }],
    })).toThrow('timeline')
  })

  it('requires P95 detail metrics', () => {
    const { ttft_p95: _ttftP95, ...groupWithoutTTFTP95 } = validPayload.groups[0]

    expect(() => validateMonitorV2Snapshot({
      ...validPayload,
      groups: [groupWithoutTTFTP95],
    })).toThrow('ttft_p95')
  })

  it('rejects incomplete enabled peak pricing rules', () => {
    expect(() => validateMonitorV2Snapshot({
      ...validPayload,
      groups: [{
        ...validPayload.groups[0],
        peak_rate_enabled: true,
        peak_start: '',
        peak_end: '',
      }],
    })).toThrow('peak_start')
  })
})
