import { useCallback, useEffect, useState } from 'react'
import {
  createThemeOverride,
  nextThemeBoundary,
  themeForBeijingTime,
  type Theme,
} from '../domain/themeSchedule'
import {
  applyHomepageTheme,
  clearThemeOverride,
  readThemeOverride,
  writeThemeOverride,
} from '../themeBootstrap'

function initialTheme(): Theme {
  const applied = document.documentElement.dataset.theme
  return applied === 'light' || applied === 'dark'
    ? applied
    : themeForBeijingTime(new Date())
}

export function useHomepageTheme() {
  const [theme, setTheme] = useState<Theme>(initialTheme)

  useEffect(() => {
    let boundaryTimer = 0

    const reconcile = () => {
      window.clearTimeout(boundaryTimer)
      const now = new Date()
      const override = readThemeOverride(now)
      if (!override) clearThemeOverride()
      const resolved = override?.theme ?? themeForBeijingTime(now)
      setTheme(resolved)
      applyHomepageTheme(resolved)

      const delay = Math.max(0, nextThemeBoundary(now).getTime() - now.getTime())
      boundaryTimer = window.setTimeout(reconcile, delay + 20)
    }

    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') reconcile()
    }

    reconcile()
    document.addEventListener('visibilitychange', onVisibilityChange)
    window.addEventListener('focus', reconcile)

    return () => {
      window.clearTimeout(boundaryTimer)
      document.removeEventListener('visibilitychange', onVisibilityChange)
      window.removeEventListener('focus', reconcile)
    }
  }, [])

  const toggleTheme = useCallback(() => {
    const now = new Date()
    const nextTheme: Theme = theme === 'dark' ? 'light' : 'dark'
    writeThemeOverride(createThemeOverride(nextTheme, now))
    setTheme(nextTheme)
    applyHomepageTheme(nextTheme)
  }, [theme])

  return { theme, toggleTheme }
}
