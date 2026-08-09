import { describe, expect, it } from 'vitest'

import { normalizeReadMode, resolvePageReadDecision, type CutoverEvidence } from '../externalizationFlags'

const passedEvidence = (overrides: Partial<CutoverEvidence> = {}): CutoverEvidence => ({
  page: 'monitor',
  windows: ['minimum', 'default', 'maximum'],
  passed: true,
  fresh_until: '2026-08-10T09:10:00Z',
  contract_complete: true,
  permission_passed: true,
  export_passed: true,
  rollback_passed: true,
  degraded: false,
  evidence_ref: 'compare:monitor:42',
  ...overrides,
})

describe('externalization page read decisions', () => {
  it('normalizes the Task 5 shadow alias and fails closed for unknown modes', () => {
    expect(normalizeReadMode('shadow')).toBe('shadow_building')
    expect(normalizeReadMode('dual_read_comparing')).toBe('dual_read_comparing')
    expect(normalizeReadMode('unexpected')).toBe('legacy_only')
  })

  it('keeps shadow and dual-read pages on the legacy source', () => {
    expect(resolvePageReadDecision('monitor', 'shadow_building', undefined, new Date('2026-08-10T09:00:00Z'))).toMatchObject({
      source: 'legacy', effectiveMode: 'shadow_building', degraded: false,
    })
    expect(resolvePageReadDecision('monitor', 'dual_read_comparing', passedEvidence(), new Date('2026-08-10T09:00:00Z'))).toMatchObject({
      source: 'legacy', effectiveMode: 'dual_read_comparing', degraded: false,
    })
  })

  it('allows external primary only with page-matched, fresh, complete three-window evidence', () => {
    expect(resolvePageReadDecision('monitor', 'external_primary', passedEvidence(), new Date('2026-08-10T09:00:00Z'))).toMatchObject({
      source: 'external', effectiveMode: 'external_primary', degraded: false,
    })

    for (const evidence of [
      undefined,
      passedEvidence({ page: 'profitability' }),
      passedEvidence({ windows: ['minimum', 'default'] }),
      passedEvidence({ fresh_until: '2026-08-10T08:59:59Z' }),
      passedEvidence({ contract_complete: false }),
      passedEvidence({ permission_passed: false }),
      passedEvidence({ export_passed: false }),
      passedEvidence({ rollback_passed: false }),
      passedEvidence({ degraded: true }),
    ]) {
      expect(resolvePageReadDecision('monitor', 'external_primary', evidence, new Date('2026-08-10T09:00:00Z'))).toMatchObject({
        source: 'legacy', effectiveMode: 'legacy_only', degraded: true,
      })
    }
  })

  it('keeps legacy_retired unreachable without separate retirement evidence', () => {
    expect(resolvePageReadDecision('monitor', 'legacy_retired', passedEvidence(), new Date('2026-08-10T09:00:00Z'))).toMatchObject({
      source: 'legacy', effectiveMode: 'legacy_only', degraded: true,
    })
    expect(resolvePageReadDecision('monitor', 'legacy_retired', passedEvidence({
      retirement: { passed: true, evidence_ref: 'retirement:review:9', operator: 'operator@example.com', recorded_at: '2026-08-10T08:50:00Z' },
    }), new Date('2026-08-10T09:00:00Z'))).toMatchObject({
      source: 'external', effectiveMode: 'legacy_retired', degraded: false,
    })
  })
})
