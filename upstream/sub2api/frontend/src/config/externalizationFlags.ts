export type ReadMode = 'legacy_only' | 'shadow_building' | 'dual_read_comparing' | 'external_primary' | 'legacy_retired'
export type ExternalizationPage = 'monitor' | 'profitability' | 'accounting' | 'reconciliation'
export type ComparisonWindow = 'minimum' | 'default' | 'maximum'

export type RetirementEvidence = {
  passed: boolean
  evidence_ref: string
  operator: string
  recorded_at: string
}

export type CutoverEvidence = {
  page: ExternalizationPage
  windows: ComparisonWindow[]
  passed: boolean
  fresh_until: string
  contract_complete: boolean
  permission_passed: boolean
  export_passed: boolean
  rollback_passed: boolean
  degraded: boolean
  evidence_ref: string
  retirement?: RetirementEvidence
}

export type PageReadDecision = {
  requestedMode: ReadMode
  effectiveMode: ReadMode
  source: 'legacy' | 'external'
  degraded: boolean
  reason: string
}

const requiredWindows = new Set<ComparisonWindow>(['minimum', 'default', 'maximum'])

export const normalizeReadMode = (value: unknown): ReadMode => {
  if (value === 'shadow') return 'shadow_building'
  if (value === 'shadow_building' || value === 'dual_read_comparing' || value === 'external_primary' || value === 'legacy_retired') return value
  return 'legacy_only'
}

const evidencePasses = (page: ExternalizationPage, evidence: CutoverEvidence | undefined, now: Date): boolean => {
  if (!evidence || evidence.page !== page || !evidence.passed || !evidence.contract_complete ||
      !evidence.permission_passed || !evidence.export_passed || !evidence.rollback_passed ||
      evidence.degraded || !evidence.evidence_ref) return false
  const windows = new Set(evidence.windows)
  if (windows.size !== requiredWindows.size || [...requiredWindows].some((window) => !windows.has(window))) return false
  const freshUntil = Date.parse(evidence.fresh_until)
  return Number.isFinite(freshUntil) && now.getTime() <= freshUntil
}

const retirementPasses = (evidence: RetirementEvidence | undefined): boolean => {
  if (!evidence?.passed || !evidence.evidence_ref || !evidence.operator) return false
  return Number.isFinite(Date.parse(evidence.recorded_at))
}

export function resolvePageReadDecision(
  page: ExternalizationPage,
  requestedValue: unknown,
  evidence?: CutoverEvidence,
  now = new Date(),
): PageReadDecision {
  const requestedMode = normalizeReadMode(requestedValue)
  if (requestedMode === 'legacy_only') {
    return { requestedMode, effectiveMode: 'legacy_only', source: 'legacy', degraded: false, reason: 'legacy_default' }
  }
  if (requestedMode === 'shadow_building' || requestedMode === 'dual_read_comparing') {
    return { requestedMode, effectiveMode: requestedMode, source: 'legacy', degraded: false, reason: 'legacy_visible_during_comparison' }
  }
  if (!evidencePasses(page, evidence, now)) {
    return { requestedMode, effectiveMode: 'legacy_only', source: 'legacy', degraded: true, reason: 'comparison_gate_failed' }
  }
  if (requestedMode === 'legacy_retired' && !retirementPasses(evidence?.retirement)) {
    return { requestedMode, effectiveMode: 'legacy_only', source: 'legacy', degraded: true, reason: 'retirement_evidence_missing' }
  }
  return { requestedMode, effectiveMode: requestedMode, source: 'external', degraded: false, reason: 'comparison_gate_passed' }
}

const globalMode = import.meta.env.VITE_CONTROL_PLANE_READ_MODE

export const externalizationFlags: Record<ExternalizationPage, ReadMode> = {
  monitor: normalizeReadMode(import.meta.env.VITE_EXTERNALIZATION_MONITOR_MODE || import.meta.env.VITE_ACCOUNT_MONITOR_READ_MODE || globalMode),
  profitability: normalizeReadMode(import.meta.env.VITE_EXTERNALIZATION_PROFITABILITY_MODE || import.meta.env.VITE_ACCOUNT_PROFITABILITY_READ_MODE || globalMode),
  accounting: normalizeReadMode(import.meta.env.VITE_EXTERNALIZATION_ACCOUNTING_MODE || import.meta.env.VITE_USAGE_READ_MODE || globalMode),
  reconciliation: normalizeReadMode(import.meta.env.VITE_EXTERNALIZATION_RECONCILIATION_MODE || globalMode),
}

export const getExternalizationReadMode = (page: ExternalizationPage): ReadMode => externalizationFlags[page] ?? 'legacy_only'
