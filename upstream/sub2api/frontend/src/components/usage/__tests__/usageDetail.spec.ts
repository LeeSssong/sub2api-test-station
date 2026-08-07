import { describe, expect, it } from 'vitest'
import type { AdminUsageLog, UsageLog } from '@/types'
import {
  confirmedUpstreamActualCost,
  effectivePerMillion,
  grossMargin,
  hasAdminUsageFields,
  includedUpstreamCost,
  usageCostEvidenceState,
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

  it('uses native actual cost for confirmed evidence without substituting the estimate', () => {
    const evidence = {
      upstream_actual_cost: '0.004',
      upstream_standard_cost: '9.5',
      confidence: 'confirmed',
    }

    expect(usageCostEvidenceState(evidence)).toBe('confirmed')
    expect(confirmedUpstreamActualCost(evidence)).toBe(0.004)
    expect(includedUpstreamCost(evidence)).toBe(0.004)
    expect(grossMargin(0.00688, evidence)).toBeCloseTo(0.00288)
  })

  it('uses upstream standard cost only for explicitly estimated evidence', () => {
    const evidence = {
      upstream_actual_cost: '0',
      upstream_standard_cost: '0.0045',
      confidence: 'estimated',
    }

    expect(usageCostEvidenceState(evidence)).toBe('estimated')
    expect(confirmedUpstreamActualCost(evidence)).toBeNull()
    expect(includedUpstreamCost(evidence)).toBe(0.0045)
    expect(grossMargin(0.00688, evidence)).toBeCloseTo(0.00238)
  })

  it('uses owned-account allocation as estimated included cost', () => {
    const evidence = {
      upstream_actual_cost: '0',
      upstream_standard_cost: '0.0032',
      confidence: 'estimated',
    }

    expect(usageCostEvidenceState(evidence)).toBe('estimated')
    expect(confirmedUpstreamActualCost(evidence)).toBeNull()
    expect(includedUpstreamCost(evidence)).toBe(0.0032)
    expect(grossMargin(0.00688, evidence)).toBeCloseTo(0.00368)
  })

  it('keeps unconfirmed or malformed cost evidence pending', () => {
    for (const evidence of [
      { upstream_actual_cost: null, upstream_standard_cost: null, confidence: 'pending' },
      { upstream_actual_cost: 'not-a-number', upstream_standard_cost: '0.0045', confidence: 'confirmed' },
      { upstream_actual_cost: '0.004', upstream_standard_cost: 'not-a-number', confidence: 'estimated' },
    ]) {
      expect(usageCostEvidenceState(evidence)).toBe('pending')
      expect(includedUpstreamCost(evidence)).toBeNull()
      expect(grossMargin(0.00688, evidence)).toBeNull()
    }
  })
})
