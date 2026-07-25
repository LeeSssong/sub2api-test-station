import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put },
}))

import accountMonitorAPI from '@/api/admin/accountMonitor'

describe('admin account monitor API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('uses the native account monitor endpoints and preserves response projections', async () => {
    const projection = {
      schema_version: 1,
      observed_at: '2026-07-25T08:00:00Z',
      stale: false,
      settings: { interval_seconds: 300, updated_by: 1, updated_at: '2026-07-25T07:55:00Z' },
      accounts: [],
    }
    const settings = { interval_seconds: 60, updated_by: 1, updated_at: '2026-07-25T08:01:00Z' }
    const history = { items: [] }

    get
      .mockResolvedValueOnce({ data: projection })
      .mockResolvedValueOnce({ data: history })
    put.mockResolvedValueOnce({ data: settings })
    post
      .mockResolvedValueOnce({ data: { completed: 2 } })
      .mockResolvedValueOnce({ data: { account_id: 7, status: 'success' } })

    await expect(accountMonitorAPI.list()).resolves.toEqual(projection)
    await expect(accountMonitorAPI.updateSettings(60)).resolves.toEqual(settings)
    await expect(accountMonitorAPI.runAll()).resolves.toEqual({ completed: 2 })
    await expect(accountMonitorAPI.runOne(7)).resolves.toEqual({ account_id: 7, status: 'success' })
    await expect(accountMonitorAPI.history(7, 25)).resolves.toEqual(history)

    expect(get).toHaveBeenNthCalledWith(1, '/admin/account-monitors')
    expect(put).toHaveBeenCalledWith('/admin/account-monitors/settings', {
      interval_seconds: 60,
    })
    expect(post).toHaveBeenNthCalledWith(1, '/admin/account-monitors/run')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/account-monitors/7/run')
    expect(get).toHaveBeenNthCalledWith(2, '/admin/account-monitors/7/history', {
      params: { limit: 25 },
    })
  })
})
