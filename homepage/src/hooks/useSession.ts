import { useEffect, useState } from 'react'
import { getCachedSession, resolveSession, type SessionState } from '../domain/session'

export function useSession() {
  const [session, setSession] = useState<SessionState>(() => getCachedSession(localStorage))

  useEffect(() => {
    let active = true
    resolveSession(localStorage, fetch).then((next) => {
      if (active) setSession(next)
    })
    return () => { active = false }
  }, [])

  return session
}
