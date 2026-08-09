import type { AdminUsageCostDetail, AdminUsageLog, UsageLog, UserUsageDetail } from '@/types'

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

function finiteCost(value: number | null | undefined): number | null {
  if (value == null) return null
  const numeric = Number(value)
  return Number.isFinite(numeric) ? numeric : null
}

export function confirmedUpstreamActualCost(
  evidence: Pick<AdminUsageCostDetail, 'status' | 'upstream_actual_cost' | 'profit'> | null | undefined,
): number | null {
  return evidence?.status === 'confirmed' ? finiteCost(evidence.upstream_actual_cost) : null
}

export function confirmedProfit(
  evidence: Pick<AdminUsageCostDetail, 'status' | 'profit' | 'upstream_actual_cost'> | null | undefined,
): number | null {
  if (evidence?.status !== 'confirmed') return null
  const upstreamCost = finiteCost(evidence.upstream_actual_cost)
  return upstreamCost == null ? null : finiteCost(evidence.profit)
}
