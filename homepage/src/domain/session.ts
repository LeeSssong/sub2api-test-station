export interface SessionUser {
  id?: number
  username?: string
  role: 'admin' | 'user'
  [key: string]: unknown
}

export type SessionState =
  | { kind: 'guest'; ctaLabel: '立即获取密钥'; ctaHref: '/register' }
  | { kind: 'user'; ctaLabel: '前往控制台'; ctaHref: '/dashboard'; user: SessionUser }
  | { kind: 'admin'; ctaLabel: '前往控制台'; ctaHref: '/admin/dashboard'; user: SessionUser }

type Fetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

interface TokenPair {
  access_token: string
  refresh_token: string
  expires_in: number
}

const GUEST: SessionState = {
  kind: 'guest',
  ctaLabel: '立即获取密钥',
  ctaHref: '/register',
}

function parseUser(value: unknown): SessionUser | null {
  if (!value || typeof value !== 'object') return null
  const source = value as Record<string, unknown>
  if (source.role !== 'admin' && source.role !== 'user') return null
  return source as SessionUser
}

function cachedUser(storage: Storage): SessionUser | null {
  const raw = storage.getItem('auth_user')
  if (!raw) return null

  try {
    return parseUser(JSON.parse(raw))
  } catch {
    return null
  }
}

function unwrap(value: unknown): unknown {
  if (!value || typeof value !== 'object') return value
  const source = value as Record<string, unknown>
  return source.code === 0 && 'data' in source ? source.data : value
}

async function readPayload(response: Response): Promise<unknown> {
  try {
    return unwrap(await response.json())
  } catch {
    return null
  }
}

function parseTokenPair(value: unknown): TokenPair | null {
  if (!value || typeof value !== 'object') return null
  const source = value as Record<string, unknown>
  if (
    typeof source.access_token !== 'string' || !source.access_token.trim()
    || typeof source.refresh_token !== 'string' || !source.refresh_token.trim()
    || typeof source.expires_in !== 'number' || !Number.isFinite(source.expires_in)
    || source.expires_in <= 0
  ) return null

  return {
    access_token: source.access_token.trim(),
    refresh_token: source.refresh_token.trim(),
    expires_in: source.expires_in,
  }
}

function stateFor(user: SessionUser): SessionState {
  return user.role === 'admin'
    ? { kind: 'admin', ctaLabel: '前往控制台', ctaHref: '/admin/dashboard', user }
    : { kind: 'user', ctaLabel: '前往控制台', ctaHref: '/dashboard', user }
}

function clearCredentials(storage: Storage) {
  for (const key of ['auth_token', 'refresh_token', 'auth_user', 'token_expires_at']) {
    storage.removeItem(key)
  }
}

async function getIdentity(fetcher: Fetcher, token: string): Promise<Response> {
  return fetcher('/api/v1/auth/me', {
    method: 'GET',
    headers: {
      Accept: 'application/json',
      Authorization: `Bearer ${token}`,
    },
  })
}

async function refreshAccessToken(fetcher: Fetcher, refreshToken: string): Promise<Response> {
  return fetcher('/api/v1/auth/refresh', {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ refresh_token: refreshToken }),
  })
}

export async function resolveSession(storage: Storage, fetcher: Fetcher): Promise<SessionState> {
  let accessToken = storage.getItem('auth_token')?.trim() ?? ''
  if (!accessToken) return GUEST

  const cached = cachedUser(storage)

  try {
    let identityResponse = await getIdentity(fetcher, accessToken)

    if (identityResponse.status === 401) {
      const refreshToken = storage.getItem('refresh_token')?.trim() ?? ''
      if (!refreshToken) {
        clearCredentials(storage)
        return GUEST
      }

      const refreshResponse = await refreshAccessToken(fetcher, refreshToken)
      if (!refreshResponse.ok) {
        clearCredentials(storage)
        return GUEST
      }

      const tokenPair = parseTokenPair(await readPayload(refreshResponse))
      if (!tokenPair) {
        clearCredentials(storage)
        return GUEST
      }

      accessToken = tokenPair.access_token
      storage.setItem('auth_token', tokenPair.access_token)
      storage.setItem('refresh_token', tokenPair.refresh_token)
      storage.setItem('token_expires_at', String(Date.now() + tokenPair.expires_in * 1000))
      identityResponse = await getIdentity(fetcher, accessToken)
    }

    if (!identityResponse.ok) {
      clearCredentials(storage)
      return GUEST
    }

    const user = parseUser(await readPayload(identityResponse))
    if (!user) {
      clearCredentials(storage)
      return GUEST
    }

    storage.setItem('auth_user', JSON.stringify(user))
    return stateFor(user)
  } catch {
    return cached ? stateFor(cached) : GUEST
  }
}

export function getCachedSession(storage: Storage): SessionState {
  if (!storage.getItem('auth_token')) return GUEST
  const user = cachedUser(storage)
  return user ? stateFor(user) : GUEST
}
