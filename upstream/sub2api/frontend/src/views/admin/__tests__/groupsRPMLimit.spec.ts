import { describe, expect, it } from 'vitest'

import { normalizeGroupRPMLimit } from '@/views/admin/groupsRPMLimit'

describe('normalizeGroupRPMLimit', () => {
  it('submits a cleared numeric input as unlimited', () => {
    expect(normalizeGroupRPMLimit('')).toBe(0)
  })

  it('preserves a configured non-negative integer', () => {
    expect(normalizeGroupRPMLimit(120)).toBe(120)
  })
})
