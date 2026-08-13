import type { AdminUsageLog, UsageLog, UserUsageDetail } from '@/types'

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
