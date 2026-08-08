export type ReadMode = 'legacy_only' | 'shadow_building' | 'dual_read_comparing' | 'external_primary' | 'legacy_retired'

const normalize = (value: unknown): ReadMode => {
  if (value === 'shadow_building' || value === 'dual_read_comparing' || value === 'external_primary' || value === 'legacy_retired') return value
  return 'legacy_only'
}

export const externalizationFlags = {
  monitor: normalize(import.meta.env.VITE_EXTERNALIZATION_MONITOR_MODE),
  profitability: normalize(import.meta.env.VITE_EXTERNALIZATION_PROFITABILITY_MODE),
  accounting: normalize(import.meta.env.VITE_EXTERNALIZATION_ACCOUNTING_MODE),
  reconciliation: normalize(import.meta.env.VITE_EXTERNALIZATION_RECONCILIATION_MODE),
}

