import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get },
}))

import { adminUsageAPI } from '@/api/admin/usage'
import type { AdminUsageLog } from '@/types'

describe('admin usage API', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('fetches an administrator usage record by id and returns the record', async () => {
    const record: AdminUsageLog = {
      id: 42,
      user_id: 7,
      api_key_id: 9,
      account_id: 11,
      request_id: 'req-admin-42',
      model: 'gpt-5',
      group_id: 13,
      subscription_id: null,
      input_tokens: 100,
      output_tokens: 25,
      cache_creation_tokens: 0,
      cache_read_tokens: 50,
      cache_creation_5m_tokens: 0,
      cache_creation_1h_tokens: 0,
      input_cost: 0.001,
      output_cost: 0.002,
      cache_creation_cost: 0,
      cache_read_cost: 0.0001,
      total_cost: 0.0031,
      actual_cost: 0.0031,
      rate_multiplier: 1,
      long_context_billing_applied: false,
      billing_type: 1,
      request_type: 'sync',
      stream: false,
      duration_ms: 800,
      first_token_ms: 250,
      image_count: 0,
      image_size: null,
      image_input_size: null,
      image_output_size: null,
      image_size_source: null,
      image_size_breakdown: null,
      image_input_tokens: 0,
      image_input_cost: 0,
      image_output_tokens: 0,
      image_output_cost: 0,
      user_agent: null,
      cache_ttl_overridden: false,
      created_at: '2026-07-25T08:00:00Z',
      upstream_endpoint: '/v1/responses',
      upstream_model: 'gpt-5-2026-07-01',
      model_mapping_chain: 'gpt-5 -> gpt-5-2026-07-01',
      account_rate_multiplier: 0.8,
      account_stats_cost: 0.00248,
      account_cost: 0.00248,
      channel_id: 17,
      billing_tier: 'standard',
      account: {
        id: 11,
        name: 'primary-upstream',
      },
    }
    get.mockResolvedValue({ data: record })

    const result = adminUsageAPI.getById(42)

    expect(get).toHaveBeenCalledWith('/admin/usage/42')
    await expect(result).resolves.toEqual(record)
  })

  it('queries upstream actual cost through the local administrator usage endpoint', async () => {
    const cost = {
      usage_id: 42,
      local_request_id: 'req-admin-42',
      upstream_request_id: 'upstream-42',
      site_actual_cost: 0.00688,
      upstream_actual_cost: 0.004,
      profit: 0.00288,
      status: 'confirmed',
    }
    get.mockResolvedValue({ data: cost })

    const result = adminUsageAPI.getUpstreamCost(42)

    expect(get).toHaveBeenCalledWith('/admin/usage/42/upstream-cost')
    await expect(result).resolves.toEqual(cost)
  })
})
