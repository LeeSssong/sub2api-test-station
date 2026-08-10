export type ReadMode = 'legacy_only' | 'shadow_building' | 'dual_read_comparing' | 'external_primary' | 'legacy_retired'
export type ExternalizationPage = 'monitor' | 'profitability' | 'accounting' | 'reconciliation'

export type ServerPageReadDecision = {
  page: ExternalizationPage
  requested_mode: ReadMode
  effective_mode: ReadMode
  use_external: boolean
  degraded: boolean
  reason: string
  report_set_id?: string
  run_id?: string
  operator?: string
  compared_at?: string
}

export type PageReadDecision = {
  requestedMode: ReadMode
  effectiveMode: ReadMode
  source: 'legacy' | 'external'
  degraded: boolean
  reason: string
  reportSetID?: string
  runID?: string
  operator?: string
  comparedAt?: string
}

export const normalizeReadMode = (value: unknown): ReadMode => {
  if (value === 'shadow') return 'shadow_building'
  if (value === 'shadow_building' || value === 'dual_read_comparing' || value === 'external_primary' || value === 'legacy_retired') return value
  return 'legacy_only'
}

export function resolveTrustedPageDecision(
  page: ExternalizationPage,
  decision?: ServerPageReadDecision,
): PageReadDecision {
  if (!decision || decision.page !== page) {
    return { requestedMode: 'legacy_only', effectiveMode: 'legacy_only', source: 'legacy', degraded: true, reason: 'trusted_decision_unavailable' }
  }
  const requestedMode = normalizeReadMode(decision.requested_mode)
  const effectiveMode = normalizeReadMode(decision.effective_mode)
  if (effectiveMode === 'shadow_building' || effectiveMode === 'dual_read_comparing') {
    return { requestedMode, effectiveMode, source: 'legacy', degraded: decision.degraded, reason: decision.reason }
  }
  if (effectiveMode === 'legacy_only' && !decision.use_external) {
    return { requestedMode, effectiveMode, source: 'legacy', degraded: decision.degraded, reason: decision.reason }
  }
  const comparedAt = Date.parse(decision.compared_at ?? '')
  if ((effectiveMode === 'external_primary' || effectiveMode === 'legacy_retired') && decision.use_external && !decision.degraded &&
      decision.report_set_id && decision.run_id && decision.operator && Number.isFinite(comparedAt)) {
    return {
      requestedMode, effectiveMode, source: 'external', degraded: false, reason: decision.reason,
      reportSetID: decision.report_set_id, runID: decision.run_id, operator: decision.operator, comparedAt: decision.compared_at,
    }
  }
  return { requestedMode, effectiveMode: 'legacy_only', source: 'legacy', degraded: true, reason: 'trusted_decision_invalid' }
}
