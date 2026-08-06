import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get },
}))

import {
  MonitorV2ContractError,
  getMonitorV2Snapshot,
  validateMonitorV2Snapshot,
} from '../api'

const validPayload = {
	contract_version: '5',
	refresh_interval_seconds: 300,
  window: '7d',
  generated_at: '2026-07-29T12:00:00Z',
  groups: [
    {
      id: 7,
      name: 'OpenAI 旗舰组',
      platform: 'openai',
      rate_multiplier: 0.2,
      peak_rate_enabled: false,
      peak_start: '',
      peak_end: '',
      peak_rate_multiplier: 1,
      status: 'operational',
      availability: {
        state: 'available',
        value: 99.5,
        sample_count: 200,
        success_count: 199,
        eligible_count: 200,
      },
      ttft: { state: 'available', value: 420, sample_count: 180 },
      ttft_p95: { state: 'available', value: 880, sample_count: 180 },
      tps: { state: 'available', value: 46.5, sample_count: 180 },
      latency: { state: 'available', value: 1320, sample_count: 199 },
      latency_p95: { state: 'available', value: 2400, sample_count: 199 },
      cache_hit: { state: 'available', value: 40, sample_count: 200 },
      timeline: [
        {
          bucket_start: '2026-07-29T06:00:00Z',
          state: 'available',
          value: 100,
          success_count: 12,
          eligible_count: 12,
        },
      ],
    },
  ],
}

describe('Monitor V2 API contract', () => {
  beforeEach(() => {
    get.mockReset()
  })

	it('returns a validated version 4 snapshot', async () => {
    get.mockResolvedValue({ data: validPayload })

    const snapshot = await getMonitorV2Snapshot('7d')

    expect(get).toHaveBeenCalledWith('/monitor-v2', {
      params: { window: '7d' },
      signal: undefined,
    })
    expect(snapshot.groups[0].name).toBe('OpenAI 旗舰组')
    expect(snapshot.groups[0].availability.eligible_count).toBe(200)
    expect(snapshot.groups[0].ttft_p95.value).toBe(880)
    expect(snapshot.groups[0].latency_p95.value).toBe(2400)
  })

	it('rejects an unsupported contract version', () => {
		expect(() =>
			validateMonitorV2Snapshot({
				...validPayload,
				contract_version: '2',
			})
		).toThrow(MonitorV2ContractError)
	})

	it('rejects unsupported refresh intervals and accepts configured values', () => {
		expect(() =>
			validateMonitorV2Snapshot({ ...validPayload, refresh_interval_seconds: 15 })
		).toThrow('refresh_interval_seconds is unsupported')

		for (const refreshIntervalSeconds of [0, 30, 60, 300, 600]) {
			expect(
				validateMonitorV2Snapshot({ ...validPayload, refresh_interval_seconds: refreshIntervalSeconds })
			).toMatchObject({ refresh_interval_seconds: refreshIntervalSeconds })
		}
	})

  it('rejects invalid metric ranges and impossible counts', () => {
    const invalidAvailability = {
      ...validPayload,
      groups: [
        {
          ...validPayload.groups[0],
          availability: {
            ...validPayload.groups[0].availability,
            value: 101,
          },
        },
      ],
    }
    expect(() => validateMonitorV2Snapshot(invalidAvailability)).toThrow('availability.value')

    const invalidCache = {
      ...validPayload,
      groups: [
        {
          ...validPayload.groups[0],
          cache_hit: {
            state: 'available',
            value: -1,
            sample_count: 200,
          },
        },
      ],
    }
    expect(() => validateMonitorV2Snapshot(invalidCache)).toThrow('cache_hit.value')
  })

  it('rejects duplicate group ids and malformed arrays', () => {
    expect(() =>
      validateMonitorV2Snapshot({
        ...validPayload,
        groups: [validPayload.groups[0], validPayload.groups[0]],
      })
    ).toThrow('duplicate group id')

    expect(() =>
      validateMonitorV2Snapshot({
        ...validPayload,
        groups: {},
      })
    ).toThrow('groups')
  })

  it('rejects a payload missing required P95 detail metrics', () => {
    const { ttft_p95: _ttftP95, ...groupWithoutTTFTP95 } = validPayload.groups[0]

    expect(() =>
      validateMonitorV2Snapshot({
        ...validPayload,
        groups: [groupWithoutTTFTP95],
      })
    ).toThrow('ttft_p95')
  })

  it('rejects legacy model information in a v4 payload', () => {
    expect(() =>
      validateMonitorV2Snapshot({
        ...validPayload,
        groups: [{ ...validPayload.groups[0], models: [] }],
      })
    ).toThrow(MonitorV2ContractError)
  })

  it('rejects oversized group arrays before mapping them', () => {
    expect(() =>
      validateMonitorV2Snapshot({
        ...validPayload,
        groups: Array.from({ length: 101 }, (_, index) => ({
          ...validPayload.groups[0],
          id: index + 1,
        })),
      })
    ).toThrow('at most 100')
  })

  it('rejects oversized timeline arrays', () => {
    expect(() =>
      validateMonitorV2Snapshot({
        ...validPayload,
        groups: [
          {
            ...validPayload.groups[0],
            timeline: Array.from({ length: 65 }, (_, index) => ({
              ...validPayload.groups[0].timeline[0],
              bucket_start: new Date(Date.UTC(2026, 6, 1, 0, index)).toISOString(),
            })),
          },
        ],
      })
    ).toThrow('timeline')
  })

  it('rejects oversized public strings', () => {
    expect(() =>
      validateMonitorV2Snapshot({
        ...validPayload,
        groups: [
          {
            ...validPayload.groups[0],
            name: 'a'.repeat(257),
          },
        ],
      })
    ).toThrow('name')
  })

  it('rejects available metrics without eligible samples', () => {
    expect(() =>
      validateMonitorV2Snapshot({
        ...validPayload,
        groups: [
          {
            ...validPayload.groups[0],
            ttft: { state: 'available', value: 420, sample_count: 0 },
          },
        ],
      })
    ).toThrow('ttft.sample_count')
  })

  it('rejects availability values that disagree with call counts', () => {
    expect(() =>
      validateMonitorV2Snapshot({
        ...validPayload,
        groups: [
          {
            ...validPayload.groups[0],
            availability: {
              state: 'available',
              value: 10,
              sample_count: 200,
              success_count: 199,
              eligible_count: 200,
            },
          },
        ],
      })
    ).toThrow('availability.value')
  })

  it('rejects incomplete enabled peak pricing rules', () => {
    expect(() =>
      validateMonitorV2Snapshot({
        ...validPayload,
        groups: [
          {
            ...validPayload.groups[0],
            peak_rate_enabled: true,
            peak_start: '',
            peak_end: '',
          },
        ],
      })
    ).toThrow('peak_start')
  })
})
