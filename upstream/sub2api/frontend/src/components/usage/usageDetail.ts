import type { AdminUsageCostDetail, AdminUsageCostDecimal, AdminUsageLog, UsageLog, UserUsageDetail } from '@/types'

export type UsageDetailScope = 'user' | 'admin'

export function effectivePerMillion(
  cost: number | null | undefined,
  tokens: number | null | undefined,
): number | null {
  if (!Number.isFinite(cost) || !Number.isFinite(tokens) || (tokens ?? 0) <= 0) {
    return null
  }

  return (cost as number) * 1_000_000 / (tokens as number)
}

const adminUsageFields = [
  'upstream_model',
  'model_mapping_chain',
  'account_rate_multiplier',
  'account_stats_cost',
  'upstream_request_id',
  'channel_id',
  'billing_tier',
  'account',
] as const

export function hasAdminUsageFields(row: UsageLog | UserUsageDetail | AdminUsageLog): row is AdminUsageLog {
  return adminUsageFields.some((field) => Object.prototype.hasOwnProperty.call(row, field))
}

export type UsageCostEvidenceState = 'confirmed' | 'estimated' | 'pending'

type UsageCostEvidence = Pick<
  AdminUsageCostDetail,
  'confidence' | 'upstream_actual_cost' | 'upstream_standard_cost'
>

function finiteCost(value: AdminUsageCostDecimal | undefined): number | null {
  if (value == null || value === '') return null
  const numeric = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(numeric) ? numeric : null
}

export function usageCostEvidenceState(
  evidence: UsageCostEvidence | null | undefined,
): UsageCostEvidenceState {
  if (evidence?.confidence === 'confirmed' && finiteCost(evidence.upstream_actual_cost) != null) {
    return 'confirmed'
  }
  if (evidence?.confidence === 'estimated' && finiteCost(evidence.upstream_standard_cost) != null) {
    return 'estimated'
  }
  return 'pending'
}

export function confirmedUpstreamActualCost(
  evidence: UsageCostEvidence | null | undefined,
): number | null {
  return usageCostEvidenceState(evidence) === 'confirmed'
    ? finiteCost(evidence?.upstream_actual_cost)
    : null
}

export function includedUpstreamCost(
  evidence: UsageCostEvidence | null | undefined,
): number | null {
  const state = usageCostEvidenceState(evidence)
  if (state === 'confirmed') return finiteCost(evidence?.upstream_actual_cost)
  if (state === 'estimated') return finiteCost(evidence?.upstream_standard_cost)
  return null
}

export function grossMargin(
  siteActualCost: number | null | undefined,
  evidence: UsageCostEvidence | null | undefined,
): number | null {
  if (!Number.isFinite(siteActualCost)) return null
  const includedCost = includedUpstreamCost(evidence)
  return includedCost == null ? null : (siteActualCost as number) - includedCost
}
