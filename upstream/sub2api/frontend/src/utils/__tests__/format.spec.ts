import { describe, expect, it } from 'vitest'
import { formatCurrency, formatMoneyFixed, formatUsdMoney } from '@/utils/format'

describe('money formatters', () => {
  it('formats valid money values with exactly two fractional digits', () => {
    expect(formatMoneyFixed(90.5)).toBe('90.50')
    expect(formatMoneyFixed(0)).toBe('0.00')
    expect(formatMoneyFixed(0.001)).toBe('0.00')
    expect(formatMoneyFixed(-1.2)).toBe('-1.20')
  })

  it('does not turn unavailable values into a fabricated zero', () => {
    expect(formatMoneyFixed(null)).toBe('—')
    expect(formatMoneyFixed(undefined)).toBe('—')
    expect(formatMoneyFixed(Number.NaN)).toBe('—')
    expect(formatMoneyFixed(Number.POSITIVE_INFINITY)).toBe('—')
    expect(formatCurrency(null)).toBe('—')
  })

  it('omits the currency symbol when a value is unavailable', () => {
    expect(formatUsdMoney(undefined)).toBe('—')
    expect(formatUsdMoney(Number.NaN)).toBe('—')
    expect(formatUsdMoney(90.5)).toBe('$90.50')
  })
})
