import { describe, expect, it, vi } from 'vitest'
import { apiClient } from '../client'
import { getReport, setOAuthCost, setTodayOverride } from '../admin/accountFinancial'

vi.mock('../client', () => ({ apiClient: { get: vi.fn(), put: vi.fn() } }))
const nativeAmounts = { requests: 3, tokens: 120, cost: 0.000123, user_cost: 0.000456, profit: 0.000333, margin: 0.7302631579 }
describe('admin account financial API', () => {
  it('normalizes native values and old aliases', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({ data: { generated_at: 'x', range: 'today', currency: 'USD', summary: nativeAmounts, accounts: [], groups: [], user_unconsumed_balance_cny: 9 } } as never)
    await expect(getReport({ range: 'today' })).resolves.toMatchObject({ currency: 'USD', summary: nativeAmounts, user_unconsumed_balance_cny: 9 })
  })
  it('normalizes PascalCase and derives missing values', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({ data: { GeneratedAt: 'x', Range: '7d', Summary: { Requests: 2, Tokens: 5, RevenueCNY: 10, CostCNY: 4, Margin: .6 }, UserBalanceCNY: 12 } } as never)
    const report = await getReport({ range: '7d' })
    expect(report.summary).toMatchObject({ requests: 2, tokens: 5, user_cost: 10, cost: 4, profit: 6, margin: .6 })
    vi.mocked(apiClient.get).mockResolvedValueOnce({ data: { summary: { user_cost: 0, cost: 2 } } } as never)
    await expect(getReport({ range: '24h' })).resolves.toMatchObject({ summary: { profit: -2, margin: null } })
  })
  it('preserves legacy ProfitCNY instead of deriving a different profit', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({ data: { summary: { RevenueCNY: 10, CostCNY: 4, ProfitCNY: 99 } } } as never)
    await expect(getReport({ range: 'today' })).resolves.toMatchObject({ summary: { user_cost: 10, cost: 4, profit: 99 } })
  })
  it('writes only explicit administrator today values', async () => {
    vi.mocked(apiClient.put).mockResolvedValue({ data: {} } as never)
    await setOAuthCost(7, { business_date: '2026-08-13', cost_cny: 5 })
    await setTodayOverride(7, { business_date: '2026-08-13', revenue_cny: 8 })
    expect(apiClient.put).toHaveBeenNthCalledWith(1, '/admin/accounts/7/financial/oauth-cost', { business_date: '2026-08-13', cost_cny: 5 })
    expect(apiClient.put).toHaveBeenNthCalledWith(2, '/admin/accounts/7/financial/today-override', { business_date: '2026-08-13', revenue_cny: 8 })
  })
})
