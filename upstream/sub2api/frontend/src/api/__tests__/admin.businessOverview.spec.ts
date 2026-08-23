import { describe, expect, it } from 'vitest'
import { normalizeBusinessOverview } from '../admin/businessOverview'

describe('business overview normalization', () => {
  it('preserves unknown financial values as null', () => {
    const report = normalizeBusinessOverview({ summary: { revenue_cny: null, gross_profit_cny: null, gross_margin: null }, cash_and_balance: { cash_recharge_cny: null } })
    expect(report.summary.revenue_cny).toBeNull()
    expect(report.summary.gross_profit_cny).toBeNull()
    expect(report.cash_and_balance.cash_recharge_cny).toBeNull()
    expect(report.quota_unit_label).toContain('不是美元')
  })

  it('normalizes snake-case trend and group rows', () => {
    const report = normalizeBusinessOverview({ trend: [{ date: '2026-08-24', cash_recharge_cny: '12.5' }], groups: [{ group_id: null, group_name: '未归组', unassigned: true, upstream_cost_cny: 2 }] })
    expect(report.trend[0]).toMatchObject({ date: '2026-08-24', cash_recharge_cny: 12.5, paid_consumption_cny: 0 })
    expect(report.groups[0]).toMatchObject({ group_name: '未归组', unassigned: true, upstream_cost_cny: 2 })
  })
})
