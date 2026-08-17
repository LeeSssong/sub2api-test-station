import { describe, expect, it } from 'vitest'
import type { AdminUsageLog, UsageLog } from '@/types'
import {
  effectiveAccountCost,
  effectivePerMillion,
  hasAdminUsageFields,
} from '../usageDetail'

describe('usage detail projections', () => {
  it('calculates the effective price per million tokens', () => {
    expect(effectivePerMillion(0.005, 1000)).toBe(5)
  })

  it.each([
    [0.005, 0],
    [0.005, -1],
    [undefined, 1000],
    [Number.POSITIVE_INFINITY, 1000],
    [0.005, Number.NaN],
  ])('returns null when cost %s or token count %s cannot produce a price', (cost, tokens) => {
    expect(effectivePerMillion(cost, tokens)).toBeNull()
  })

  it('does not classify an ordinary usage record as an administrator record', () => {
    expect(hasAdminUsageFields({ id: 42 } as UsageLog)).toBe(false)
  })

  it('recognizes a record with safe administrator fields', () => {
    expect(hasAdminUsageFields({ id: 42, channel_id: 7 } as AdminUsageLog)).toBe(true)
  })
})

describe('effectiveAccountCost', () => {
  it.each([
    {
      name: 'prefers an explicit account cost, including zero',
      input: { account_cost: 0.00331446, account_stats_cost: 0.01, total_cost: 0.02, account_rate_multiplier: 0.25 },
      expected: 0.00331446,
    },
    {
      name: 'falls back to account stats cost for historical rows',
      input: { account_cost: null, account_stats_cost: 0.01, total_cost: 0.02, account_rate_multiplier: 0.25 },
      expected: 0.0025,
    },
    {
      name: 'falls back to total cost when stats cost is absent',
      input: { account_cost: null, account_stats_cost: null, total_cost: 0.02, account_rate_multiplier: 0.25 },
      expected: 0.005,
    },
    {
      name: 'preserves zero multiplier as a valid value',
      input: { account_cost: null, account_stats_cost: 0.01, total_cost: 0.02, account_rate_multiplier: 0 },
      expected: 0,
    },
  ])('$name', ({ input, expected }) => {
    expect(effectiveAccountCost(input)).toBeCloseTo(expected)
  })

  it('returns null when every cost source is unavailable', () => {
    expect(effectiveAccountCost({
      account_cost: null,
      account_stats_cost: null,
      total_cost: null,
      account_rate_multiplier: null,
    })).toBeNull()
  })
})
