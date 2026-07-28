import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  applyHomepageTheme,
  bootstrapHomepageTheme,
  THEME_OVERRIDE_STORAGE_KEY,
} from './themeBootstrap'

describe('homepage theme bootstrap', () => {
  beforeEach(() => {
    localStorage.clear()
    document.documentElement.removeAttribute('data-theme')
    document.documentElement.removeAttribute('style')
    document.head.innerHTML = '<meta name="theme-color" content="#090b10">'
  })

  it('applies the scheduled theme before the homepage renders', () => {
    expect(bootstrapHomepageTheme('/', new Date('2026-07-29T03:00:00.000Z'))).toBe('light')
    expect(document.documentElement).toHaveAttribute('data-theme', 'light')
    expect(document.documentElement.style.colorScheme).toBe('light')
    expect(document.querySelector('meta[name="theme-color"]')).toHaveAttribute('content', '#f4f7fb')
  })

  it('restores a valid manual override during the same schedule window', () => {
    localStorage.setItem(THEME_OVERRIDE_STORAGE_KEY, JSON.stringify({
      theme: 'dark',
      expiresAt: Date.parse('2026-07-29T11:00:00.000Z'),
    }))

    expect(bootstrapHomepageTheme('/', new Date('2026-07-29T03:00:00.000Z'))).toBe('dark')
    expect(document.documentElement).toHaveAttribute('data-theme', 'dark')
  })

  it('removes an invalid override and falls back to the schedule', () => {
    localStorage.setItem(THEME_OVERRIDE_STORAGE_KEY, 'broken')

    expect(bootstrapHomepageTheme('/', new Date('2026-07-29T03:00:00.000Z'))).toBe('light')
    expect(localStorage.getItem(THEME_OVERRIDE_STORAGE_KEY)).toBeNull()
  })

  it('does not apply the homepage theme to support', () => {
    expect(bootstrapHomepageTheme('/support', new Date('2026-07-29T03:00:00.000Z'))).toBeNull()
    expect(document.documentElement).not.toHaveAttribute('data-theme')
  })

  it('announces theme changes for Canvas consumers', () => {
    const listener = vi.fn()
    window.addEventListener('xingqiao-theme-change', listener)

    applyHomepageTheme('dark')

    expect(listener).toHaveBeenCalledOnce()
    expect((listener.mock.calls[0]?.[0] as CustomEvent).detail).toEqual({ theme: 'dark' })
    window.removeEventListener('xingqiao-theme-change', listener)
  })
})
