import { useCallback, useEffect, useState } from 'react'
import { useReducedMotionPreference } from './useReducedMotion'

interface EntryOrigin {
  x: string
  y: string
}

export function useHeroEntry() {
  const reduced = useReducedMotionPreference()
  const [started, setStarted] = useState(reduced)
  const [origin, setOrigin] = useState<EntryOrigin>({ x: '50%', y: '42%' })

  const start = useCallback(() => setStarted(true), [])
  const updateOrigin = useCallback((x: number, y: number) => {
    setOrigin({ x: `${Math.round(x)}px`, y: `${Math.round(y)}px` })
  }, [])

  useEffect(() => {
    if (reduced) {
      setStarted(true)
      return
    }
    const coarse = window.innerWidth <= 680 || window.matchMedia?.('(pointer: coarse)').matches === true
    const timeout = window.setTimeout(start, coarse ? 180 : 1400)
    const startFromIntent = () => start()
    window.addEventListener('scroll', startFromIntent, { passive: true, once: true })
    window.addEventListener('keydown', startFromIntent, { once: true })
    return () => {
      window.clearTimeout(timeout)
      window.removeEventListener('scroll', startFromIntent)
      window.removeEventListener('keydown', startFromIntent)
    }
  }, [reduced, start])

  return { reduced, started, origin, start, updateOrigin }
}
