import { describe, expect, it } from 'vitest'
import dashboard from '../locales/zh/dashboard'
import overview from '../locales/zh/admin/overview'

describe('Chinese multiplier labels', () => {
  it('uses the same suffix in dynamic monitor, pricing and group-selector messages', () => {
    expect(dashboard.monitorV2.peakRate).toContain('{rate}x倍率')
    expect(dashboard.modelPlaza.table.maxReasoningMultiplierBadge).toContain('{multiplier}x倍率')
    expect(dashboard.modelPlaza.table.timePricingRateHint).toContain('{rate}x倍率')
    expect(overview.groups.rateAndAccounts).toContain('{rate}x倍率')
  })
})
