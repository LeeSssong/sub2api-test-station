import { describe, expect, it, vi } from 'vitest'
import { apiClient } from '../client'
import { getReport, setOAuthCost, setTodayOverride } from '../admin/accountFinancial'

vi.mock('../client', () => ({ apiClient: { get: vi.fn(), put: vi.fn() } }))
describe('admin account financial API', () => {
  it('normalizes the Task 5 report and preserves the requested range', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({ data: { GeneratedAt: '2026-08-12T10:00:00Z', Range: 'today', Summary: { RevenueCNY: 10, CostCNY: 4, ProfitCNY: 6, Margin: .6, ExceptionCount: 3, AffectedRevenueCNY: 5 }, Accounts: [], UserBalanceCNY: 9 } } as never)
    await expect(getReport({ range: 'today' })).resolves.toMatchObject({ generated_at: '2026-08-12T10:00:00Z', summary: { revenue: 10, cost: 4, exception_count: 3 }, user_unconsumed_balance_cny: 9 })
    expect(apiClient.get).toHaveBeenCalledWith('/admin/operations/account-financial', { params: { range: 'today' } })
  })
  it('writes only explicit administrator today values', async () => {
    vi.mocked(apiClient.put).mockResolvedValue({ data: {} } as never)
    await setOAuthCost(7, { business_date: '2026-08-13', cost_cny: 5 })
    await setTodayOverride(7, { business_date: '2026-08-13', revenue_cny: 8 })
    expect(apiClient.put).toHaveBeenNthCalledWith(1, '/admin/accounts/7/financial/oauth-cost', { business_date: '2026-08-13', cost_cny: 5 })
    expect(apiClient.put).toHaveBeenNthCalledWith(2, '/admin/accounts/7/financial/today-override', { business_date: '2026-08-13', revenue_cny: 8 })
  })
})
