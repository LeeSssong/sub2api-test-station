import { apiClient } from '../client'

export type FinancialRange = 'today' | '24h' | '7d' | '31d'
export interface FinancialAmounts { revenue: number; cost: number; profit: number; margin: number | null; exception_count: number; affected_revenue: number }
export interface FinancialAccount { id: number; name: string; type: string; platform: string; complete: boolean; amounts: FinancialAmounts; exception_count: number; affected_revenue: number }
export interface AccountFinancialReport { generated_at: string; range: FinancialRange; summary: FinancialAmounts; accounts: FinancialAccount[]; exception_count: number; affected_revenue: number; user_unconsumed_balance_cny: number }
export interface TodayOverridePayload { business_date: string; revenue_cny?: number; cost_cny?: number }
export interface OAuthCostPayload { business_date: string; cost_cny?: number }

const numberValue = (value: unknown, fallback = 0) => typeof value === 'number' ? value : Number(value ?? fallback)
const read = (record: Record<string, unknown>, snake: string, pascal: string) => record[snake] ?? record[pascal]
function amounts(value: unknown): FinancialAmounts {
  const r = (value && typeof value === 'object' ? value : {}) as Record<string, unknown>
  return { revenue: numberValue(read(r, 'revenue', 'RevenueCNY')), cost: numberValue(read(r, 'cost', 'CostCNY') ?? read(r, 'expense', 'ExpenseCNY')), profit: numberValue(read(r, 'profit', 'ProfitCNY')), margin: read(r, 'margin', 'Margin') == null ? null : numberValue(read(r, 'margin', 'Margin')), exception_count: numberValue(read(r, 'exception_count', 'ExceptionCount')), affected_revenue: numberValue(read(r, 'affected_revenue', 'AffectedRevenueCNY')) }
}
function normalize(raw: unknown, requested: FinancialRange): AccountFinancialReport {
  const root = (raw && typeof raw === 'object' ? raw : {}) as Record<string, unknown>
  const summary = amounts(read(root, 'summary', 'Summary'))
  const rows = (read(root, 'accounts', 'Accounts') as unknown[] | undefined) ?? []
  const accounts = Array.isArray(rows) ? rows.map((item) => {
    const r = item as Record<string, unknown>; return { id: numberValue(read(r, 'id', 'ID')), name: String(read(r, 'name', 'Name') ?? ''), type: String(read(r, 'type', 'Type') ?? ''), platform: String(read(r, 'platform', 'Platform') ?? ''), complete: Boolean(read(r, 'complete', 'Complete') ?? true), amounts: amounts(read(r, 'amounts', 'Amounts')), exception_count: numberValue(read(r, 'exception_count', 'ExceptionCount')), affected_revenue: numberValue(read(r, 'affected_revenue', 'AffectedRevenueCNY')) }
  }) : []
  return { generated_at: String(read(root, 'generated_at', 'GeneratedAt') ?? ''), range: String(read(root, 'range', 'Range') ?? requested) as FinancialRange, summary, accounts, exception_count: numberValue(read(root, 'exception_count', 'ExceptionCount') ?? summary.exception_count), affected_revenue: numberValue(read(root, 'affected_revenue', 'AffectedRevenueCNY') ?? summary.affected_revenue), user_unconsumed_balance_cny: numberValue(read(root, 'user_unconsumed_balance_cny', 'UserBalanceCNY')) }
}
export async function getReport(params: { range: FinancialRange }): Promise<AccountFinancialReport> { const { data } = await apiClient.get('/admin/operations/account-financial', { params }); return normalize(data, params.range) }
export async function setOAuthCost(accountId: number, payload: OAuthCostPayload) { const { data } = await apiClient.put(`/admin/accounts/${accountId}/financial/oauth-cost`, payload); return data }
export async function setTodayOverride(accountId: number, payload: TodayOverridePayload) { const { data } = await apiClient.put(`/admin/accounts/${accountId}/financial/today-override`, payload); return data }
export default { getReport, setOAuthCost, setTodayOverride }
