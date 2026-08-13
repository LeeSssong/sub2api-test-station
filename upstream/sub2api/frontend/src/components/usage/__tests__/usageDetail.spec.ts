import { describe, expect, it } from 'vitest'
import type { AdminUsageLog, UsageLog } from '@/types'
import {
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
