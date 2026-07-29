import { describe, expect, it } from 'vitest'
import {
  createThemeOverride,
  nextThemeBoundary,
  parseThemeOverride,
  themeForBeijingTime,
} from './themeSchedule'

describe('themeSchedule', () => {
  it.each([
    ['2026-07-28T21:59:59.000Z', 'dark'],
    ['2026-07-28T22:00:00.000Z', 'light'],
    ['2026-07-29T10:59:59.000Z', 'light'],
    ['2026-07-29T11:00:00.000Z', 'dark'],
  ] as const)('maps %s to %s in Beijing', (iso, expected) => {
    expect(themeForBeijingTime(new Date(iso))).toBe(expected)
  })

  it.each([
    ['2026-07-29T03:00:00.000Z', '2026-07-29T11:00:00.000Z'],
    ['2026-07-29T12:00:00.000Z', '2026-07-29T22:00:00.000Z'],
    ['2026-07-29T21:59:59.000Z', '2026-07-29T22:00:00.000Z'],
  ])('finds the next Beijing boundary after %s', (iso, boundary) => {
    expect(nextThemeBoundary(new Date(iso)).toISOString()).toBe(boundary)
  })

  it('expires a manual override at the next Beijing boundary', () => {
    const now = new Date('2026-07-29T03:00:00.000Z')

    expect(createThemeOverride('dark', now)).toEqual({
      theme: 'dark',
      expiresAt: Date.parse('2026-07-29T11:00:00.000Z'),
    })
  })

  it('restores a valid stored override', () => {
    const now = new Date('2026-07-29T03:00:00.000Z')
    const raw = JSON.stringify({ theme: 'dark', expiresAt: Date.parse('2026-07-29T11:00:00.000Z') })

    expect(parseThemeOverride(raw, now)).toEqual({
      theme: 'dark',
      expiresAt: Date.parse('2026-07-29T11:00:00.000Z'),
    })
  })

  it.each([
    null,
    'not-json',
    '{"theme":"sepia","expiresAt":1785322800000}',
    '{"theme":"light","expiresAt":"later"}',
    '{"theme":"light","expiresAt":1785322800000}',
  ])('rejects malformed or expired override %s', (raw) => {
    const now = new Date('2026-07-29T11:00:00.000Z')
    expect(parseThemeOverride(raw, now)).toBeNull()
  })
})
