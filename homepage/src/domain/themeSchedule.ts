export type Theme = 'light' | 'dark'

export interface ThemeOverride {
  theme: Theme
  expiresAt: number
}

const BEIJING_OFFSET_MS = 8 * 60 * 60 * 1000

function beijingClock(now: Date) {
  return new Date(now.getTime() + BEIJING_OFFSET_MS)
}

export function themeForBeijingTime(now: Date): Theme {
  const hour = beijingClock(now).getUTCHours()
  return hour >= 6 && hour < 19 ? 'light' : 'dark'
}

export function nextThemeBoundary(now: Date): Date {
  const clock = beijingClock(now)
  const year = clock.getUTCFullYear()
  const month = clock.getUTCMonth()
  const day = clock.getUTCDate()
  const hour = clock.getUTCHours()

  if (hour < 6) {
    return new Date(Date.UTC(year, month, day, 6) - BEIJING_OFFSET_MS)
  }

  if (hour < 19) {
    return new Date(Date.UTC(year, month, day, 19) - BEIJING_OFFSET_MS)
  }

  return new Date(Date.UTC(year, month, day + 1, 6) - BEIJING_OFFSET_MS)
}

export function createThemeOverride(theme: Theme, now: Date): ThemeOverride {
  return {
    theme,
    expiresAt: nextThemeBoundary(now).getTime(),
  }
}

export function parseThemeOverride(raw: string | null, now: Date): ThemeOverride | null {
  if (!raw) return null

  try {
    const candidate: unknown = JSON.parse(raw)
    if (
      typeof candidate !== 'object'
      || candidate === null
      || !('theme' in candidate)
      || !('expiresAt' in candidate)
    ) {
      return null
    }

    const { theme, expiresAt } = candidate as Record<string, unknown>
    if (
      (theme !== 'light' && theme !== 'dark')
      || typeof expiresAt !== 'number'
      || !Number.isFinite(expiresAt)
      || expiresAt <= now.getTime()
    ) {
      return null
    }

    return { theme, expiresAt }
  } catch {
    return null
  }
}
