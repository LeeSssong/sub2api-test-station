import { describe, expect, it } from 'vitest'
import { buildSignalRows, signalPalette } from './signalField'

describe('signalField', () => {
  it('builds a dense field with three ordered depth layers', () => {
    const rows = buildSignalRows({ width: 1440, height: 900, random: () => .42 })

    expect(new Set(rows.map((row) => row.layer))).toEqual(new Set(['far', 'mid', 'near']))
    expect(rows.length).toBeGreaterThanOrEqual(45)

    const far = rows.find((row) => row.layer === 'far')
    const mid = rows.find((row) => row.layer === 'mid')
    const near = rows.find((row) => row.layer === 'near')
    expect(far).toBeDefined()
    expect(mid).toBeDefined()
    expect(near).toBeDefined()
    expect(far!.fontSize).toBeLessThan(mid!.fontSize)
    expect(mid!.fontSize).toBeLessThan(near!.fontSize)
    expect(far!.alpha).toBeLessThan(mid!.alpha)
    expect(mid!.alpha).toBeLessThan(near!.alpha)
  })

  it('keeps highlighted active traffic sparse', () => {
    const rows = buildSignalRows({ width: 1440, height: 900, random: () => .42 })
    const active = rows.filter((row) => row.active)

    expect(active.length).toBeGreaterThan(0)
    expect(active.length).toBeLessThan(rows.length / 3)
  })

  it('uses distinct palettes tuned for light and dark surfaces', () => {
    const light = signalPalette('light')
    const dark = signalPalette('dark')

    expect(light).not.toEqual(dark)
    expect(light.active).not.toBe(light.near)
    expect(dark.active).not.toBe(dark.near)
  })
})
