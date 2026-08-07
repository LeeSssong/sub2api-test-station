import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { get },
  buildGatewayUrl: (path: string) => path,
}))

import reconciliationAPI from '@/api/admin/reconciliation'

describe('admin reconciliation API', () => {
  beforeEach(() => get.mockReset())

  it('keeps relay-ops cost guard 401s inside the relay auth boundary', async () => {
    const response = { status: 'unknown', group_multiplier: 0.8, required_sample_count: 6 }
    get.mockResolvedValueOnce({ data: response })

    await expect(reconciliationAPI.costGuard({ account_id: 21, group_id: 3, group_multiplier: 0.8 })).resolves.toEqual(response)
    expect(get).toHaveBeenCalledWith('/relay-ops/api/reconciliation/cost-guard', {
      params: { account_id: 21, group_id: 3, group_multiplier: 0.8 },
      skipSessionRecovery: true,
    })
  })
})
