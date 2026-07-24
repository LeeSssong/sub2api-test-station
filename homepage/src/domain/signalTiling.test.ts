import { describe, expect, it } from 'vitest'
import { buildSignalTile } from './signalTiling'

describe('buildSignalTile', () => {
  it('repeats a measured segment far enough to cover a wide viewport with wrap overflow', () => {
    const tile = buildSignalTile('XQ SIGNAL   ', 320, 1920)

    expect(tile.repetitions).toBe(8)
    expect(tile.text).toBe('XQ SIGNAL   '.repeat(8))
    expect(tile.coveredWidth).toBeGreaterThanOrEqual(1920 + 640)
  })

  it('keeps enough overflow on a narrow viewport for seamless movement', () => {
    const tile = buildSignalTile('XQ SIGNAL   ', 320, 390)

    expect(tile.repetitions).toBe(4)
    expect(tile.coveredWidth).toBeGreaterThanOrEqual(390 + 640)
  })
})
