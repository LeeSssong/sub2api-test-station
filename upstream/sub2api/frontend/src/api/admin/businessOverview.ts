import { apiClient } from '../client'

export type BusinessOverviewRange = 'today' | '7d' | '30d' | 'month' | 'previous_month' | 'custom'
export type BusinessOverviewStatus = 'confirmed' | 'pending_split' | 'pending' | 'unavailable' | string

export interface BusinessOverviewSummary {
  revenue_status: BusinessOverviewStatus
  revenue_cny: number
  upstream_cost_cny: number
  gross_profit_cny: number
  gross_margin: number
  paid_consumption_q: number
  gift_consumption_q: number
  gift_upstream_cost_cny: number
  pending_split_count: number
  pending_cost_count: number
}

export interface BusinessOverviewCashBalance {
  cash_recharge_cny: number
  opening_paid_balance_cny: number
  paid_quota_issued_cny: number
  paid_consumption_cny: number
  closing_paid_balance_cny: number
  opening_gift_balance_q: number
  closing_gift_balance_q: number
  net_settlement_cny: number
  balance_reconciliation: { status: string; difference_cny: number; adjustments: string[] }
}

export interface BusinessOverviewReport {
  generated_at: string
  timezone: string
  start_date: string
  end_date: string
  currency: 'CNY'
  quota_unit: 'Q'
  quota_unit_label: string
  revenue_status: BusinessOverviewStatus
  summary: BusinessOverviewSummary
  cash_and_balance: BusinessOverviewCashBalance
  trend: Array<{ date: string; cash_recharge_cny: number; paid_consumption_cny: number; net_settlement_cny: number }>
  groups: Array<{
    group_id: number | null
    group_name: string
    unassigned: boolean
    model_count: number
    request_count: number
    configured_multiplier: number | null
    preset_upstream_multiplier: number | null
    preset_margin: number | null
    preset_status: string
    effective_upstream_multiplier: number | null
    revenue_cny: number | null
    upstream_cost_cny: number | null
    gross_profit_cny: number | null
    gross_margin: number | null
    revenue_status: BusinessOverviewStatus
  }>
}

const numberOrNull = (value: unknown): number | null => {
  if (value == null || value === '') return null
  const n = Number(value)
  return Number.isFinite(n) ? n : null
}
const numberOrZero = (value: unknown): number => numberOrNull(value) ?? 0
const objectValue = (value: unknown): Record<string, unknown> => (value && typeof value === 'object' ? value as Record<string, unknown> : {})
const value = (row: Record<string, unknown>, snake: string, pascal: string) => row[snake] ?? row[pascal]

export function normalizeBusinessOverview(raw: unknown): BusinessOverviewReport {
  const root = objectValue(raw)
  const summary = objectValue(value(root, 'summary', 'Summary'))
  const cash = objectValue(value(root, 'cash_and_balance', 'CashAndBalance'))
  const reconciliation = objectValue(value(cash, 'balance_reconciliation', 'BalanceReconciliation'))
  const rows = Array.isArray(value(root, 'trend', 'Trend')) ? value(root, 'trend', 'Trend') as unknown[] : []
  const groups = Array.isArray(value(root, 'groups', 'Groups')) ? value(root, 'groups', 'Groups') as unknown[] : []
  return {
    generated_at: String(value(root, 'generated_at', 'GeneratedAt') ?? ''),
    timezone: String(value(root, 'timezone', 'Timezone') ?? 'Asia/Shanghai'),
    start_date: String(value(root, 'start_date', 'StartDate') ?? ''),
    end_date: String(value(root, 'end_date', 'EndDate') ?? ''),
    currency: 'CNY',
    quota_unit: 'Q',
    quota_unit_label: String(value(root, 'quota_unit_label', 'QuotaUnitLabel') ?? '内部记账额度，不是美元'),
    revenue_status: String(value(root, 'revenue_status', 'RevenueStatus') ?? 'confirmed'),
    summary: {
      revenue_status: String(value(summary, 'revenue_status', 'RevenueStatus') ?? value(root, 'revenue_status', 'RevenueStatus') ?? 'confirmed'),
      revenue_cny: numberOrZero(value(summary, 'revenue_cny', 'RevenueCNY')),
      upstream_cost_cny: numberOrZero(value(summary, 'upstream_cost_cny', 'UpstreamCostCNY')),
      gross_profit_cny: numberOrZero(value(summary, 'gross_profit_cny', 'GrossProfitCNY')),
      gross_margin: numberOrZero(value(summary, 'gross_margin', 'GrossMargin')),
      paid_consumption_q: numberOrZero(value(summary, 'paid_consumption_q', 'PaidConsumptionQ')),
      gift_consumption_q: numberOrZero(value(summary, 'gift_consumption_q', 'GiftConsumptionQ')),
      gift_upstream_cost_cny: numberOrZero(value(summary, 'gift_upstream_cost_cny', 'GiftUpstreamCostCNY')),
      pending_split_count: numberOrZero(value(summary, 'pending_split_count', 'PendingSplitCount')),
      pending_cost_count: numberOrZero(value(summary, 'pending_cost_count', 'PendingCostCount')),
    },
    cash_and_balance: {
      cash_recharge_cny: numberOrZero(value(cash, 'cash_recharge_cny', 'CashRechargeCNY')),
      opening_paid_balance_cny: numberOrZero(value(cash, 'opening_paid_balance_cny', 'OpeningPaidBalanceCNY')),
      paid_quota_issued_cny: numberOrZero(value(cash, 'paid_quota_issued_cny', 'PaidQuotaIssuedCNY')),
      paid_consumption_cny: numberOrZero(value(cash, 'paid_consumption_cny', 'PaidConsumptionCNY')),
      closing_paid_balance_cny: numberOrZero(value(cash, 'closing_paid_balance_cny', 'ClosingPaidBalanceCNY')),
      opening_gift_balance_q: numberOrZero(value(cash, 'opening_gift_balance_q', 'OpeningGiftBalanceQ')),
      closing_gift_balance_q: numberOrZero(value(cash, 'closing_gift_balance_q', 'ClosingGiftBalanceQ')),
      net_settlement_cny: numberOrZero(value(cash, 'net_settlement_cny', 'NetSettlementCNY')),
      balance_reconciliation: {
        status: String(value(reconciliation, 'status', 'Status') ?? 'balanced'),
        difference_cny: numberOrZero(value(reconciliation, 'difference_cny', 'DifferenceCNY')),
        adjustments: Array.isArray(value(reconciliation, 'adjustments', 'Adjustments')) ? (value(reconciliation, 'adjustments', 'Adjustments') as unknown[]).map(String) : [],
      },
    },
    trend: rows.map((item) => {
      const row = objectValue(item)
      return { date: String(value(row, 'date', 'Date') ?? ''), cash_recharge_cny: numberOrZero(value(row, 'cash_recharge_cny', 'CashRechargeCNY')), paid_consumption_cny: numberOrZero(value(row, 'paid_consumption_cny', 'PaidConsumptionCNY')), net_settlement_cny: numberOrZero(value(row, 'net_settlement_cny', 'NetSettlementCNY')) }
    }),
    groups: groups.map((item) => {
      const row = objectValue(item)
      return {
        group_id: numberOrNull(value(row, 'group_id', 'GroupID')),
        group_name: String(value(row, 'group_name', 'GroupName') ?? ''),
        unassigned: Boolean(value(row, 'unassigned', 'Unassigned')),
        model_count: numberOrZero(value(row, 'model_count', 'ModelCount')),
        request_count: numberOrZero(value(row, 'request_count', 'RequestCount')),
        configured_multiplier: numberOrNull(value(row, 'configured_multiplier', 'ConfiguredMultiplier')),
        preset_upstream_multiplier: numberOrNull(value(row, 'preset_upstream_multiplier', 'PresetUpstreamMultiplier')),
        preset_margin: numberOrNull(value(row, 'preset_margin', 'PresetMargin')),
        preset_status: String(value(row, 'preset_status', 'PresetStatus') ?? 'unavailable'),
        effective_upstream_multiplier: numberOrNull(value(row, 'effective_upstream_multiplier', 'EffectiveUpstreamMultiplier')),
        revenue_cny: numberOrZero(value(row, 'revenue_cny', 'RevenueCNY')),
        upstream_cost_cny: numberOrZero(value(row, 'upstream_cost_cny', 'UpstreamCostCNY')),
        gross_profit_cny: numberOrZero(value(row, 'gross_profit_cny', 'GrossProfitCNY')),
        gross_margin: numberOrZero(value(row, 'gross_margin', 'GrossMargin')),
        revenue_status: String(value(row, 'revenue_status', 'RevenueStatus') ?? 'confirmed'),
      }
    }),
  }
}

export async function getReport(params: { range: BusinessOverviewRange; start_date?: string; end_date?: string; timezone?: string; group_id?: number }): Promise<BusinessOverviewReport> {
  const { data } = await apiClient.get('/admin/operations/business-overview', { params })
  return normalizeBusinessOverview(data)
}

export default { getReport }
