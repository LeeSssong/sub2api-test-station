import {
  parseThemeOverride,
  themeForBeijingTime,
  type Theme,
  type ThemeOverride,
} from './domain/themeSchedule'

export const THEME_OVERRIDE_STORAGE_KEY = 'xingqiao-home-theme-override'
export const HOMEPAGE_THEME_EVENT = 'xingqiao-theme-change'

function isSupportPath(pathname: string) {
  return pathname === '/support' || pathname === '/support/'
}

export function readThemeOverride(now: Date): ThemeOverride | null {
  try {
    const raw = window.localStorage.getItem(THEME_OVERRIDE_STORAGE_KEY)
    const override = parseThemeOverride(raw, now)
    if (raw && !override) window.localStorage.removeItem(THEME_OVERRIDE_STORAGE_KEY)
    return override
  } catch {
    return null
  }
}

export function writeThemeOverride(override: ThemeOverride) {
  try {
    window.localStorage.setItem(THEME_OVERRIDE_STORAGE_KEY, JSON.stringify(override))
  } catch {
    // The theme still applies for this page session when storage is unavailable.
  }
}

export function clearThemeOverride() {
  try {
    window.localStorage.removeItem(THEME_OVERRIDE_STORAGE_KEY)
  } catch {
    // Storage can be unavailable in hardened browsing modes.
  }
}

export function resolveHomepageTheme(now: Date): Theme {
  return readThemeOverride(now)?.theme ?? themeForBeijingTime(now)
}

export function applyHomepageTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme
  document.documentElement.style.colorScheme = theme
  document.querySelector('meta[name="theme-color"]')?.setAttribute(
    'content',
    theme === 'dark' ? '#090b10' : '#f4f7fb',
  )
  window.dispatchEvent(new CustomEvent(HOMEPAGE_THEME_EVENT, { detail: { theme } }))
}

export function bootstrapHomepageTheme(pathname: string, now = new Date()): Theme | null {
  if (isSupportPath(pathname)) return null
  const theme = resolveHomepageTheme(now)
  applyHomepageTheme(theme)
  return theme
}
