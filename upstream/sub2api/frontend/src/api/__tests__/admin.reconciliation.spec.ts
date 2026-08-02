import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))

vi.mock('@/api/client', () => ({
  apiClient: { get },
  buildGatewayUrl: (path: string) => path,
}))

import reconciliationAPI from '@/api/admin/reconciliation'

describe('admin reconciliation operations API', () => {
  beforeEach(() => get.mockReset())

  it('passes the selected scope to summary and daily history endpoints', async () => {
    const summary = { total_attempts: 3, matched_attempts: 2, pending_attempts: 1, conflict_attempts: 0, coverage_known: true, upstream_cost: '0.02', user_charge: '1.00', paper_profit: '0.98', profit_margin: '0.98' }
    const daily = { items: [{ day: '2026-07-24', upstream_cost: '0.02', user_charge: '1.00', paper_profit: '0.98', currency: 'USD' }] }
    get.mockResolvedValueOnce({ data: summary }).mockResolvedValueOnce({ data: daily })
    const scope = { group_id: 3, timezone: 'Asia/Shanghai' }

    await expect(reconciliationAPI.operations(scope)).resolves.toEqual(summary)
    await expect(reconciliationAPI.history(scope)).resolves.toEqual(daily)
    expect(get).toHaveBeenNthCalledWith(1, '/relay-ops/api/reconciliation/operations', { params: scope })
    expect(get).toHaveBeenNthCalledWith(2, '/relay-ops/api/reconciliation/operations/history', { params: scope })
  })

  it('rejects an HTML fallback instead of displaying fake empty accounting data', async () => {
    get.mockResolvedValueOnce({ data: '<!doctype html><html></html>' })
    await expect(reconciliationAPI.operations()).rejects.toThrow('经营数据返回了无效数据')
  })

  it('rejects malformed history lists instead of crashing the drawer', async () => {
    get.mockResolvedValueOnce({ data: { items: null } })
    await expect(reconciliationAPI.history()).rejects.toThrow('历史按日返回了无效列表')
  })
})
