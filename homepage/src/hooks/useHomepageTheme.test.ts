import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { THEME_OVERRIDE_STORAGE_KEY } from '../themeBootstrap'
import { useHomepageTheme } from './useHomepageTheme'

describe('useHomepageTheme', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-29T03:00:00.000Z'))
    localStorage.clear()
    document.documentElement.dataset.theme = 'light'
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('toggles immediately and persists only until the next Beijing boundary', () => {
    const { result } = renderHook(() => useHomepageTheme())

    act(() => result.current.toggleTheme())

    expect(result.current.theme).toBe('dark')
    expect(document.documentElement).toHaveAttribute('data-theme', 'dark')
    expect(JSON.parse(localStorage.getItem(THEME_OVERRIDE_STORAGE_KEY) ?? '{}')).toEqual({
      theme: 'dark',
      expiresAt: Date.parse('2026-07-29T11:00:00.000Z'),
    })
  })

  it('clears the override and restores the schedule at the boundary', () => {
    const { result } = renderHook(() => useHomepageTheme())
    act(() => result.current.toggleTheme())

    act(() => {
      vi.setSystemTime(new Date('2026-07-29T11:00:00.000Z'))
      vi.runOnlyPendingTimers()
    })

    expect(result.current.theme).toBe('dark')
    expect(localStorage.getItem(THEME_OVERRIDE_STORAGE_KEY)).toBeNull()
  })

  it('recalibrates when the window regains focus after a missed boundary', () => {
    localStorage.setItem(THEME_OVERRIDE_STORAGE_KEY, JSON.stringify({
      theme: 'dark',
      expiresAt: Date.parse('2026-07-29T11:00:00.000Z'),
    }))
    const { result } = renderHook(() => useHomepageTheme())

    act(() => {
      vi.setSystemTime(new Date('2026-07-29T12:00:00.000Z'))
      window.dispatchEvent(new Event('focus'))
    })

    expect(result.current.theme).toBe('dark')
    expect(localStorage.getItem(THEME_OVERRIDE_STORAGE_KEY)).toBeNull()
  })
})
