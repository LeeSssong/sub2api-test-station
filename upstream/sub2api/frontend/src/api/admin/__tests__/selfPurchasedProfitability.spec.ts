import { beforeEach, describe, expect, it, vi } from 'vitest'
import { get } from '../selfPurchasedProfitability'

const apiGet = vi.hoisted(() => vi.fn())
vi.mock('../../client', () => ({ apiClient: { get: apiGet } }))

describe('selfPurchasedProfitability API', () => {
  beforeEach(() => apiGet.mockReset())

  it('passes the shared financial range to the native endpoint', async () => {
    apiGet.mockResolvedValueOnce({ data: { currency: 'CNY', summary: {}, rows: [] } })
    await get({ range: '7d' })
    expect(apiGet).toHaveBeenCalledWith('/admin/operations/self-purchased-profitability', { params: { range: '7d' } })
  })
})
