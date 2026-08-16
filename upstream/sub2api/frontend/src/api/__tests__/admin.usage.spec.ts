import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post },
}))

import { adminUsageAPI } from '@/api/admin/usage'
import type { AdminUsageLog } from '@/types'

describe('admin usage API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('uses local cost exception endpoints for listing and reviews', async () => {
    get.mockResolvedValueOnce({ data: { items: [], total: 0, page: 1, page_size: 20 } })
    post
      .mockResolvedValueOnce({ data: { usage_log_id: 11, created: true } })
      .mockResolvedValueOnce({ data: [{ usage_log_id: 11, created: true }] })
      .mockResolvedValueOnce({ data: { cutoff: 99, matched: 2, updated: 2, skipped: 0 } })

    await adminUsageAPI.listCostExceptions({ account_id: 42, evidence_status: 'unavailable', page: 1 })
    await adminUsageAPI.reviewOne(11, { manual_cost_cny: 1.25 })
    await adminUsageAPI.reviewSelected({ usage_log_ids: [11, 12] })
    await adminUsageAPI.reviewFiltered({ filter: { account_id: 42 }, max_usage_log_id: 99 })

    expect(get).toHaveBeenCalledWith('/admin/usage/cost-exceptions', {
      params: { account_id: 42, evidence_status: 'unavailable', page: 1 },
      signal: undefined,
    })
    expect(post).toHaveBeenNthCalledWith(1, '/admin/usage/cost-exceptions/11/review', { manual_cost_cny: 1.25 })
    expect(post).toHaveBeenNthCalledWith(2, '/admin/usage/cost-exceptions/review-selected', { usage_log_ids: [11, 12] })
    expect(post).toHaveBeenNthCalledWith(3, '/admin/usage/cost-exceptions/review-filtered', {
      filter: { account_id: 42 },
      max_usage_log_id: 99,
    })
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

  it('reads the local administrator evidence projection without retaining the legacy upstream client', async () => {
    const cost = {
      usage_log_id: 42,
      evidence_status: 'confirmed',
      reason_code: '',
      normalized_cost_cny: 0.004,
      review_id: null,
      review_cost_cny: null,
    }
    get.mockResolvedValue({ data: cost })

    const result = adminUsageAPI.getCostEvidence(42)

    expect(get).toHaveBeenCalledWith('/admin/usage/42/upstream-cost')
    await expect(result).resolves.toEqual(cost)
    expect(adminUsageAPI).not.toHaveProperty('getUpstreamCost')
  })

  it.each([
    {
      name: 'normalizes PascalCase responses',
      response: {
        UsageLogID: 42,
        Source: 'newapi',
        EvidenceStatus: 'confirmed',
        ReasonCode: 'matched',
        NormalizedCostCNY: 0.004123456789,
        ReviewID: 7,
        ReviewCostCNY: 0.003,
      },
      expected: {
        usage_log_id: 42,
        source: 'newapi',
        evidence_status: 'confirmed',
        reason_code: 'matched',
        normalized_cost_cny: 0.004123456789,
        review_id: 7,
        review_cost_cny: 0.003,
      },
    },
    {
      name: 'keeps snake_case responses unchanged',
      response: {
        usage_log_id: 43,
        source: 'sub',
        evidence_status: 'confirmed_zero',
        reason_code: 'zero',
        normalized_cost_cny: 0,
        review_id: null,
        review_cost_cny: null,
      },
      expected: {
        usage_log_id: 43,
        source: 'sub',
        evidence_status: 'confirmed_zero',
        reason_code: 'zero',
        normalized_cost_cny: 0,
        review_id: null,
        review_cost_cny: null,
      },
    },
    {
      name: 'prefers snake_case when both names are present',
      response: {
        usage_log_id: 44,
        UsageLogID: 999,
        source: '',
        Source: 'legacy-source',
        evidence_status: null,
        EvidenceStatus: 'legacy-status',
        reason_code: 'snake-reason',
        ReasonCode: 'legacy-reason',
        normalized_cost_cny: null,
        NormalizedCostCNY: 99,
        review_id: null,
        ReviewID: 999,
        review_cost_cny: 0,
        ReviewCostCNY: 99,
      },
      expected: {
        usage_log_id: 44,
        source: '',
        evidence_status: null,
        reason_code: 'snake-reason',
        normalized_cost_cny: null,
        review_id: null,
        review_cost_cny: 0,
      },
    },
    {
      name: 'preserves missing fields for empty and non-object responses',
      response: {},
      expected: {},
    },
    {
      name: 'preserves missing fields for primitive responses',
      response: 'unexpected',
      expected: {},
    },
    {
      name: 'preserves missing fields for array responses',
      response: [],
      expected: {},
    },
  ])('$name', async ({ response, expected }) => {
    get.mockResolvedValue({ data: response })

    await expect(adminUsageAPI.getCostEvidence(42)).resolves.toEqual(expected)
  })

  it('safely downgrades a non-object response without inventing fields', async () => {
    get.mockResolvedValue({ data: null })

    await expect(adminUsageAPI.getCostEvidence(42)).resolves.toEqual({})
  })

  it('rethrows network errors unchanged', async () => {
    const error = new Error('upstream unavailable')
    get.mockRejectedValue(error)

    await expect(adminUsageAPI.getCostEvidence(42)).rejects.toBe(error)
  })
})
