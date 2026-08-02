import { apiClient, buildGatewayUrl } from '../client'

export interface ReconciliationSummary {
  total_attempts: number
  matched_attempts: number
  pending_attempts: number
  conflict_attempts: number
  coverage_known: boolean
  coverage_ratio: number
  upstream_cost: string | number
  user_charge: string | number
  paper_profit: string | number
  profit_margin?: string | number | null
  unattributed_attempts?: number
  unattributed_user_charge?: string | number
  unattributed_upstream_cost?: string | number
  currency: string
  observed_at: string
}

export type OperationsScopeParams = {
  group_id?: number
  account_id?: number
  start?: string
  end?: string
  currency?: string
  timezone?: string
}

export interface OperationsDailyRow {
  day: string
  total_attempts?: number
  matched_attempts?: number
  pending_attempts?: number
  conflict_attempts?: number
  coverage_known?: boolean
  coverage_ratio?: string | number
  upstream_cost: string | number
  user_charge: string | number
  paper_profit: string | number
  profit_margin?: string | number | null
  currency: string
}

export interface ReconciliationException {
  id: number
  reason_code: string
  details: string
  retry_count: number
  first_detected_at: string
  last_checked_at: string
  attempt: { id: number; attempt_id: string; local_request_id: string; upstream_request_id?: string; account_id: number; model: string; user_charge: string | number; currency: string; completed_at: string; reconcile_status: string }
}

const base = () => buildGatewayUrl('/relay-ops/api/reconciliation')
function objectResponse(value: unknown, operation: string): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error(`${operation}返回了无效数据，请检查账务服务连接`)
  }
  return value as Record<string, unknown>
}
function summaryResponse(value: unknown, operation: string): ReconciliationSummary {
  const record = objectResponse(value, operation)
  for (const field of ['total_attempts', 'matched_attempts', 'pending_attempts', 'conflict_attempts']) {
    if (typeof record[field] !== 'number') throw new Error(`${operation}返回缺少${field}，请检查账务服务连接`)
  }
  return record as unknown as ReconciliationSummary
}
export async function summary(params: { account_id?: number } = {}): Promise<ReconciliationSummary> {
  const { data } = await apiClient.get<unknown>(`${base()}/summary`, { params }); return summaryResponse(data, '账务汇总')
}
export async function operations(params: OperationsScopeParams = {}): Promise<ReconciliationSummary> {
  const { data } = await apiClient.get<unknown>(`${base()}/operations`, { params })
  return summaryResponse(data, '经营数据')
}
export async function history(params: OperationsScopeParams = {}): Promise<{ items: OperationsDailyRow[] }> {
  const { data } = await apiClient.get<unknown>(`${base()}/operations/history`, { params })
  const record = objectResponse(data, '历史按日')
  if (!Array.isArray(record.items)) throw new Error('历史按日返回了无效列表，请检查账务服务连接')
  return { items: record.items as OperationsDailyRow[] }
}
export async function exceptions(params: { account_id?: number; limit?: number } = {}): Promise<{ items: ReconciliationException[] }> {
  const { data } = await apiClient.get<unknown>(`${base()}/exceptions`, { params })
  const record = objectResponse(data, '异常明细')
  if (!Array.isArray(record.items)) throw new Error('异常明细返回了无效列表，请检查账务服务连接')
  return { items: record.items as ReconciliationException[] }
}
export async function refresh(params: { account_id?: number } = {}): Promise<ReconciliationSummary> {
  const { data } = await apiClient.post<unknown>(`${base()}/refresh`, null, { params }); return summaryResponse(data, '账务刷新')
}
export async function adjust(attemptID: number, amount: string, notes = ''): Promise<unknown> {
  const { data } = await apiClient.post(`${base()}/exceptions/${attemptID}/adjust`, { amount, notes }); return data
}
export default { summary, operations, history, exceptions, refresh, adjust }
