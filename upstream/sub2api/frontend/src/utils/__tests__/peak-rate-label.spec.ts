import { describe, expect, it } from 'vitest'
import { formatPeakRateWindow } from '../peak-rate'

describe('peak multiplier display', () => {
  it('shows the configured rate with the common label and timezone', () => {
    expect(formatPeakRateWindow({ peak_rate_enabled: true, peak_start: '14:00', peak_end: '18:00', peak_rate_multiplier: 1.5 }, 'UTC+08:00'))
      .toBe('14:00-18:00 1.5x倍率 (UTC+08:00)')
  })
})
