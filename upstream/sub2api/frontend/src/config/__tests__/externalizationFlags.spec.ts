import { describe, expect, it } from 'vitest'

import { normalizeReadMode, resolveTrustedPageDecision, type ServerPageReadDecision } from '../externalizationFlags'

const serverDecision = (overrides: Partial<ServerPageReadDecision> = {}): ServerPageReadDecision => ({
  page: 'monitor',
  requested_mode: 'external_primary',
  effective_mode: 'external_primary',
  use_external: true,
  degraded: false,
  reason: 'comparison_gate_passed',
  report_set_id: 'set-monitor-42',
  run_id: 'run-monitor-42',
  operator: 'operator@example.com',
  compared_at: '2026-08-10T09:00:00Z',
  ...overrides,
})

describe('trusted externalization page decisions', () => {
  it('normalizes stored modes and fails closed for unknown values', () => {
    expect(normalizeReadMode('shadow')).toBe('shadow_building')
    expect(normalizeReadMode('dual_read_comparing')).toBe('dual_read_comparing')
    expect(normalizeReadMode('unexpected')).toBe('legacy_only')
  })

  it('uses an external source only for a page-matched server decision with immutable evidence identity', () => {
    expect(resolveTrustedPageDecision('monitor', serverDecision())).toMatchObject({
      source: 'external', effectiveMode: 'external_primary', degraded: false, reportSetID: 'set-monitor-42',
    })
    for (const decision of [
      undefined,
      serverDecision({ page: 'profitability' }),
      serverDecision({ effective_mode: 'legacy_only' }),
      serverDecision({ use_external: false }),
      serverDecision({ report_set_id: '' }),
      serverDecision({ run_id: '' }),
      serverDecision({ operator: '' }),
      serverDecision({ compared_at: '' }),
      serverDecision({ degraded: true }),
    ]) {
      expect(resolveTrustedPageDecision('monitor', decision)).toMatchObject({
        source: 'legacy', effectiveMode: 'legacy_only', degraded: true,
      })
    }
  })

  it('keeps server shadow and dual-read modes on legacy without browser-authored evidence', () => {
    expect(resolveTrustedPageDecision('monitor', serverDecision({ requested_mode: 'shadow_building', effective_mode: 'shadow_building', use_external: false, report_set_id: '', run_id: '', operator: '', compared_at: '' }))).toMatchObject({
      source: 'legacy', effectiveMode: 'shadow_building', degraded: false,
    })
  })
})
