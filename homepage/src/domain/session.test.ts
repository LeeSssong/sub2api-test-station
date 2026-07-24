import { describe, expect, it, vi } from 'vitest'
import { resolveSession } from './session'

function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function makeStorage(initial: Record<string, string> = {}): Storage {
  const values = new Map(Object.entries(initial))

  return {
    get length() { return values.size },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => Array.from(values.keys())[index] ?? null,
    removeItem: (key) => { values.delete(key) },
    setItem: (key, value) => { values.set(key, value) },
  }
}

describe('resolveSession', () => {
  it('returns the native dashboard entry when no access token exists', async () => {
    const fetcher = vi.fn()

    await expect(resolveSession(makeStorage(), fetcher)).resolves.toEqual({
      kind: 'guest',
      ctaLabel: '立即开始',
      ctaHref: '/dashboard',
    })
    expect(fetcher).not.toHaveBeenCalled()
  })

  it('validates a cached user and returns the user dashboard', async () => {
    const storage = makeStorage({
      auth_token: 'user-token',
      auth_user: JSON.stringify({ id: 7, role: 'user', username: 'bridge-user' }),
    })
    const fetcher = vi.fn().mockResolvedValue(response({
      code: 0,
      data: { id: 7, role: 'user', username: 'bridge-user' },
    }))

    await expect(resolveSession(storage, fetcher)).resolves.toMatchObject({
      kind: 'user',
      ctaLabel: '进入控制台',
      ctaHref: '/dashboard',
    })
  })

  it('routes a validated administrator to the admin dashboard', async () => {
    const storage = makeStorage({ auth_token: 'admin-token' })
    const fetcher = vi.fn().mockResolvedValue(response({ id: 1, role: 'admin' }))

    await expect(resolveSession(storage, fetcher)).resolves.toMatchObject({
      kind: 'admin',
      ctaLabel: '进入控制台',
      ctaHref: '/admin/dashboard',
    })
  })

  it('refreshes once after a 401 and retries the identity request', async () => {
    const storage = makeStorage({
      auth_token: 'expired-token',
      refresh_token: 'refresh-token',
      auth_user: JSON.stringify({ id: 9, role: 'user' }),
      token_expires_at: '0',
    })
    const fetcher = vi.fn()
      .mockResolvedValueOnce(response({ message: 'expired' }, 401))
      .mockResolvedValueOnce(response({ code: 0, data: {
        access_token: 'new-token',
        refresh_token: 'new-refresh-token',
        expires_in: 3600,
      } }))
      .mockResolvedValueOnce(response({ code: 0, data: { id: 9, role: 'user' } }))

    const result = await resolveSession(storage, fetcher)

    expect(result.kind).toBe('user')
    expect(fetcher).toHaveBeenCalledTimes(3)
    expect(storage.getItem('auth_token')).toBe('new-token')
    expect(storage.getItem('refresh_token')).toBe('new-refresh-token')
    expect(Number(storage.getItem('token_expires_at'))).toBeGreaterThan(Date.now())
  })

  it('returns a guest and clears stale credentials when refresh fails', async () => {
    const storage = makeStorage({
      auth_token: 'expired-token',
      refresh_token: 'refresh-token',
      auth_user: JSON.stringify({ id: 9, role: 'user' }),
    })
    const fetcher = vi.fn()
      .mockResolvedValueOnce(response({ message: 'expired' }, 401))
      .mockResolvedValueOnce(response({ message: 'invalid refresh' }, 401))

    await expect(resolveSession(storage, fetcher)).resolves.toMatchObject({ kind: 'guest' })
    expect(storage.getItem('auth_token')).toBeNull()
    expect(storage.getItem('auth_user')).toBeNull()
  })

  it('retains a valid cached role when identity validation is unavailable', async () => {
    const storage = makeStorage({
      auth_token: 'cached-token',
      auth_user: JSON.stringify({ id: 3, role: 'admin' }),
    })
    const fetcher = vi.fn().mockRejectedValue(new TypeError('network unavailable'))

    await expect(resolveSession(storage, fetcher)).resolves.toMatchObject({
      kind: 'admin',
      ctaHref: '/admin/dashboard',
    })
  })
})
