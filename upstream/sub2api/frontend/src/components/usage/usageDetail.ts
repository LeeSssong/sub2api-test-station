import type { AdminUsageLog, UsageLog, UserUsageDetail } from '@/types'

export type UsageDetailScope = 'user' | 'admin'

export interface EffectiveAccountCostInput {
  account_cost?: number | null
  account_stats_cost?: number | null
  total_cost?: number | null
  account_rate_multiplier?: number | null
}

/**
 * Resolve the Sub-native effective account cost for an admin usage detail.
 * Nullish numeric fields follow the persisted historical fallback; zero is
 * always a valid cost or multiplier and must not be treated as missing.
 */
export function effectiveAccountCost(input: EffectiveAccountCostInput): number | null {
  if (Number.isFinite(input.account_cost)) {
    return input.account_cost as number
  }

  const multiplier = Number.isFinite(input.account_rate_multiplier)
    ? input.account_rate_multiplier as number
    : 1
  const source = Number.isFinite(input.account_stats_cost)
    ? input.account_stats_cost
    : input.total_cost

  return Number.isFinite(source) ? (source as number) * multiplier : null
}

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
