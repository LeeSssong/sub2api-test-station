import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, post } }))

import announcementsAPI from '@/api/announcements'

describe('announcement compatible transport', () => {
  beforeEach(() => vi.resetAllMocks())

  it('reads announcements through the compatible endpoint and preserves unread filtering', async () => {
    const announcements = [{ id: 7, title: 'Test announcement' }]
    get.mockResolvedValue({ data: announcements })
    expect(await announcementsAPI.list(true)).toEqual(announcements)
    expect(get).toHaveBeenCalledWith('/notices', { params: { unread_only: 1 } })
  })

  it('marks the selected announcement read through the same compatible endpoint', async () => {
    post.mockResolvedValue({ data: { message: 'ok' } })
    expect(await announcementsAPI.markRead(7)).toEqual({ message: 'ok' })
    expect(post).toHaveBeenCalledWith('/notices/7/read')
  })

  it('propagates read failures so the store can retry', async () => {
    const error = new Error('unavailable')
    get.mockRejectedValue(error)
    await expect(announcementsAPI.list()).rejects.toBe(error)
  })
})
