import { beforeEach, describe, expect, it, vi } from 'vitest'
import { getReport } from '../accountFinancial'

const get = vi.hoisted(() => vi.fn())
vi.mock('../../client', () => ({ apiClient: { get } }))

describe('accountFinancial normalization', () => {
  beforeEach(() => { get.mockReset() })

  it('normalizes probe fields without coercing nullable query-error data to zero', async () => {
    get.mockResolvedValueOnce({
      data: {
        GeneratedAt: '2026-08-17T00:00:00Z',
        Range: '7d',
        ProbeDataError: true,
        ProbeErrorCode: 'probe_aggregate_unavailable',
        Summary: {
          Requests: 4,
          Tokens: 40,
          Cost: 1,
          UserCost: 2,
          Profit: 1,
          Margin: 0.5,
          ProbeRequests: null,
          ProbeTokens: null,
          ProbeCost: null,
          ProbeCostStatus: null,
        },
        Accounts: [],
        Groups: [],
        UserBalanceCNY: 12,
      },
    })

    const report = await getReport({ range: '7d' })
    expect(report.probe_data_error).toBe(true)
    expect(report.probe_error_code).toBe('probe_aggregate_unavailable')
    expect(report.summary.probe_requests).toBeNull()
    expect(report.summary.probe_tokens).toBeNull()
    expect(report.summary.probe_cost).toBeNull()
    expect(report.summary.probe_cost_status).toBeNull()
    expect(report.summary.cost).toBe(1)
  })

  it('normalizes snake-case probe values for summary, group, and account rows', async () => {
    const amount = {
      requests: 1,
      tokens: 2,
      cost: 0.1,
      user_cost: 0.2,
      profit: 0.1,
      margin: 0.5,
      probe_requests: 3,
      probe_tokens: 4,
      probe_cost: '0.0300000000',
      probe_cost_status: 'confirmed',
    }
    get.mockResolvedValueOnce({
      data: {
        generated_at: '2026-08-17T00:00:00Z',
        range: 'today',
        probe_data_error: false,
        probe_error_code: null,
        summary: amount,
        accounts: [{ id: 7, name: 'A', type: 'api_key', platform: 'sub', historical: false, amounts: amount }],
        groups: [{ id: 10, name: 'Pro', unassigned: false, historical: false, amounts: amount, accounts: [{ id: 7, name: 'A', type: 'api_key', platform: 'sub', historical: false, amounts: amount }] }],
        user_unconsumed_balance_cny: 12,
      },
    })

    const report = await getReport({ range: 'today' })
    expect(report.probe_data_error).toBe(false)
    expect(report.summary.probe_cost).toBe(0.03)
    expect(report.groups[0].amounts.probe_requests).toBe(3)
    expect(report.groups[0].accounts[0].amounts.probe_cost_status).toBe('confirmed')
  })
})
