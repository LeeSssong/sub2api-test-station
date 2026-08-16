import { apiClient } from '../client'
export type FinancialRange = 'today' | '24h' | '7d' | '31d'
export type ProbeCostStatus = 'confirmed' | 'incomplete' | 'unavailable'
export interface FinancialAmounts {
  requests: number
  tokens: number
  cost: number
  user_cost: number
  profit: number
  margin: number | null
  probe_requests: number | null
  probe_tokens: number | null
  probe_cost: number | null
  probe_cost_status: ProbeCostStatus | null
}
export interface FinancialAccount { id: number; name: string; type: string; platform: string; historical: boolean; amounts: FinancialAmounts }
export interface FinancialGroup { id: number; name: string; unassigned: boolean; historical: boolean; amounts: FinancialAmounts; accounts: FinancialAccount[] }
export interface AccountFinancialReport {
  generated_at: string
  range: FinancialRange
  currency: 'USD'
  probe_data_error: boolean
  probe_error_code: string | null
  summary: FinancialAmounts
  accounts: FinancialAccount[]
  groups: FinancialGroup[]
  user_unconsumed_balance_cny: number
}
export interface TodayOverridePayload { business_date: string; revenue_cny?: number; cost_cny?: number }
export interface OAuthCostPayload { business_date: string; cost_cny?: number }
const read = (r: Record<string, unknown>, snake: string, pascal: string) => r[snake] ?? r[pascal]
const numberValue = (v: unknown, fallback = 0) => { const n = typeof v === 'number' ? v : Number(v); return Number.isFinite(n) ? n : fallback }
const nullableNumber = (value: unknown) => value == null ? null : numberValue(value)
function amounts(value: unknown): FinancialAmounts {
  const r = (value && typeof value === 'object' ? value : {}) as Record<string, unknown>
  const user_cost = numberValue(read(r, 'user_cost', 'UserCost') ?? read(r, 'revenue', 'RevenueCNY'), 0)
  const cost = numberValue(read(r, 'cost', 'Cost') ?? read(r, 'expense', 'ExpenseCNY') ?? r.CostCNY)
  const profitValue = read(r, 'profit', 'Profit') ?? r.ProfitCNY
  const marginValue = read(r, 'margin', 'Margin')
  const hasMargin = Object.prototype.hasOwnProperty.call(r, 'margin') || Object.prototype.hasOwnProperty.call(r, 'Margin')
  const probeCostStatus = read(r, 'probe_cost_status', 'ProbeCostStatus')
  return {
    requests: numberValue(read(r, 'requests', 'Requests')),
    tokens: numberValue(read(r, 'tokens', 'Tokens')),
    cost,
    user_cost,
    profit: profitValue == null ? user_cost - cost : numberValue(profitValue),
    margin: hasMargin ? (marginValue == null ? null : numberValue(marginValue)) : (user_cost === 0 ? null : (user_cost - cost) / user_cost),
    probe_requests: nullableNumber(read(r, 'probe_requests', 'ProbeRequests')),
    probe_tokens: nullableNumber(read(r, 'probe_tokens', 'ProbeTokens')),
    probe_cost: nullableNumber(read(r, 'probe_cost', 'ProbeCost')),
    probe_cost_status: probeCostStatus == null ? null : String(probeCostStatus) as ProbeCostStatus,
  }
}
function account(value: unknown): FinancialAccount {
  const r = (value && typeof value === 'object' ? value : {}) as Record<string, unknown>
  return { id: numberValue(read(r, 'id', 'ID')), name: String(read(r, 'name', 'Name') ?? ''), type: String(read(r, 'type', 'Type') ?? ''), platform: String(read(r, 'platform', 'Platform') ?? ''), historical: Boolean(read(r, 'historical', 'Historical') ?? false), amounts: amounts(read(r, 'amounts', 'Amounts')) }
}
function normalize(raw: unknown, requested: FinancialRange): AccountFinancialReport {
  const root = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const rows = (value: unknown) => Array.isArray(value) ? value : []
  const groups = rows(read(root, 'groups', 'Groups')).map((item) => { const r = item as Record<string, unknown>; return { id: numberValue(read(r, 'id', 'ID')), name: String(read(r, 'name', 'Name') ?? ''), unassigned: Boolean(read(r, 'unassigned', 'Unassigned') ?? false), historical: Boolean(read(r, 'historical', 'Historical') ?? false), amounts: amounts(read(r, 'amounts', 'Amounts')), accounts: rows(read(r, 'accounts', 'Accounts')).map(account) } })
  const probeErrorCode = read(root, 'probe_error_code', 'ProbeErrorCode')
  return {
    generated_at: String(read(root, 'generated_at', 'GeneratedAt') ?? ''),
    range: String(read(root, 'range', 'Range') ?? requested) as FinancialRange,
    currency: 'USD',
    probe_data_error: Boolean(read(root, 'probe_data_error', 'ProbeDataError') ?? false),
    probe_error_code: probeErrorCode == null ? null : String(probeErrorCode),
    summary: amounts(read(root, 'summary', 'Summary')),
    accounts: rows(read(root, 'accounts', 'Accounts')).map(account),
    groups,
    user_unconsumed_balance_cny: numberValue(read(root, 'user_unconsumed_balance_cny', 'UserBalanceCNY')),
  }
}
export async function getReport(params: { range: FinancialRange }): Promise<AccountFinancialReport> {
  const { data } = await apiClient.get('/admin/operations/account-financial', { params })
  return normalize(data, params.range)
}
export async function setOAuthCost(accountId: number, payload: OAuthCostPayload) { const { data } = await apiClient.put(`/admin/accounts/${accountId}/financial/oauth-cost`, payload); return data }
export async function setTodayOverride(accountId: number, payload: TodayOverridePayload) { const { data } = await apiClient.put(`/admin/accounts/${accountId}/financial/today-override`, payload); return data }
export default { getReport, setOAuthCost, setTodayOverride }
