import { describe, expect, it, vi } from 'vitest'
import { controlPlaneAPI } from '@/api/controlPlane'
import { apiClient } from '@/api/client'

vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn(), post: vi.fn() } }))

describe('control plane API', () => {
  it('uses same-origin routes and preserves freshness metadata', async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({ data: { items: [], freshness: { completeness: 'complete', calculation_version: 'v1' } } })
    const response = await controlPlaneAPI.monitor({ range: '24h' })
    expect(apiClient.get).toHaveBeenCalledWith('/xingqiao/accounts/monitor', { params: { range: '24h' } })
    expect(response.freshness?.calculation_version).toBe('v1')
  })

  it('requires an idempotency key for account refresh requests', async () => {
    vi.mocked(apiClient.post).mockResolvedValueOnce({ data: { account_id: 7, status: 'accepted' } })
    await controlPlaneAPI.refreshAccount(7, 'refresh-7')
    expect(apiClient.post).toHaveBeenCalledWith('/xingqiao/accounts/7/refresh', {}, { headers: { 'Idempotency-Key': 'refresh-7' } })
  })
})

