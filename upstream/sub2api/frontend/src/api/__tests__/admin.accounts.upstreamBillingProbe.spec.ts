import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put }
}))

import {
  getUpstreamBillingProbeSettings,
  probeUpstreamBilling,
  probeUpstreamBillingBatch,
  setUpstreamBillingProbeEnabled,
  updateProcurementCost,
  updateUpstreamBillingProbeSettings,
} from '@/api/admin/accounts'

describe('admin account upstream billing probe API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('reads and updates global settings', async () => {
    const settings = { enabled: true, interval_minutes: 30 }
    get.mockResolvedValueOnce({ data: settings })
    put.mockResolvedValueOnce({ data: settings })

    await expect(getUpstreamBillingProbeSettings()).resolves.toEqual(settings)
    await expect(updateUpstreamBillingProbeSettings(settings)).resolves.toEqual(settings)
    expect(get).toHaveBeenCalledWith('/admin/accounts/upstream-billing-probe/settings')
    expect(put).toHaveBeenCalledWith('/admin/accounts/upstream-billing-probe/settings', settings)
  })

  it('uses dedicated account and batch endpoints', async () => {
    const result = { account_id: 7, snapshot: { status: 'unsupported' } }
    put.mockResolvedValueOnce({ data: {} })
    post.mockResolvedValueOnce({ data: result })
    post.mockResolvedValueOnce({ data: { results: [result] } })

    await setUpstreamBillingProbeEnabled(7, true)
    await expect(probeUpstreamBilling(7)).resolves.toEqual(result)
    await expect(probeUpstreamBillingBatch([7])).resolves.toEqual([result])

    expect(put).toHaveBeenCalledWith('/admin/accounts/7/upstream-billing-probe', { enabled: true })
    expect(post).toHaveBeenNthCalledWith(1, '/admin/accounts/7/upstream-billing-probe')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/accounts/upstream-billing-probe/batch', { account_ids: [7] })
  })

  it('uses the explicit cost-dialog session idempotency key', async () => {
    put.mockResolvedValueOnce({ data: { account_id: 7 } })
    await expect(updateProcurementCost(7, 4, 60, 'procurement-session-1')).resolves.toEqual({ account_id: 7 })
    expect(put).toHaveBeenCalledWith('/admin/accounts/7', {
      procurement_cost_cny: 4,
      estimated_usable_quota_usd: 60,
    }, { headers: { 'Idempotency-Key': 'procurement-session-1' } })
  })
})
