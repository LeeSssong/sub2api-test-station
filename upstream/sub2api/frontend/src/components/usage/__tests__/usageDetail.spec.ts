import { describe, expect, it } from 'vitest'
import type { AdminUsageLog, UsageLog } from '@/types'
import {
  effectivePerMillion,
  hasAdminUsageFields,
  confirmedUpstreamActualCost,
  confirmedProfit,
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

  it('uses confirmed native actual cost and backend profit without client-side estimation', () => {
    const evidence = {
      upstream_actual_cost: '0.004',
      profit: '0.00288',
      status: 'confirmed',
    }

    expect(confirmedUpstreamActualCost(evidence)).toBe(0.004)
    expect(confirmedProfit(evidence)).toBe(0.00288)
  })

  it('accepts a confirmed zero upstream cost as real data', () => {
    const evidence = {
      upstream_actual_cost: '0',
      profit: '0.00688',
      status: 'confirmed',
    }

    expect(confirmedUpstreamActualCost(evidence)).toBe(0)
    expect(confirmedProfit(evidence)).toBe(0.00688)
  })

  it('returns null for unavailable or malformed backend cost results', () => {
    expect(confirmedUpstreamActualCost({ upstream_actual_cost: null, profit: null, status: 'unavailable' })).toBeNull()
    expect(confirmedProfit({ upstream_actual_cost: null, profit: null, status: 'unavailable' })).toBeNull()
    expect(confirmedUpstreamActualCost({ upstream_actual_cost: 'not-a-number', profit: '0.1', status: 'confirmed' })).toBeNull()
    expect(confirmedProfit({ upstream_actual_cost: 'not-a-number', profit: '0.1', status: 'confirmed' })).toBeNull()
    expect(confirmedUpstreamActualCost({ upstream_actual_cost: '0.004', profit: 'not-a-number', status: 'confirmed' })).toBe(0.004)
    expect(confirmedProfit({ upstream_actual_cost: '0.004', profit: 'not-a-number', status: 'confirmed' })).toBeNull()
    expect(confirmedUpstreamActualCost({ upstream_actual_cost: '0.004', profit: '0.1', status: 'matched' })).toBeNull()
    expect(confirmedProfit({ upstream_actual_cost: '0.004', profit: '0.1', status: 'matched' })).toBeNull()
  })

  it('keeps upstream actual cost visible when backend profit is missing', () => {
    const evidence = {
      upstream_actual_cost: '0.0032',
      profit: null,
      status: 'confirmed',
    }

    expect(confirmedUpstreamActualCost(evidence)).toBe(0.0032)
    expect(confirmedProfit(evidence)).toBeNull()
  })
})
