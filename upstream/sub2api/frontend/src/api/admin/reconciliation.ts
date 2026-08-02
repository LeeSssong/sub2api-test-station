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
export async function summary(params: { account_id?: number } = {}): Promise<ReconciliationSummary> {
  const { data } = await apiClient.get<ReconciliationSummary>(`${base()}/summary`, { params }); return data
}
export async function operations(params: OperationsScopeParams = {}): Promise<ReconciliationSummary> {
  const { data } = await apiClient.get<ReconciliationSummary>(`${base()}/operations`, { params })
  return data
}
export async function history(params: OperationsScopeParams = {}): Promise<{ items: OperationsDailyRow[] }> {
  const { data } = await apiClient.get<{ items: OperationsDailyRow[] }>(`${base()}/operations/history`, { params })
  return data
}
export async function exceptions(params: { account_id?: number; limit?: number } = {}): Promise<{ items: ReconciliationException[] }> {
  const { data } = await apiClient.get<{ items: ReconciliationException[] }>(`${base()}/exceptions`, { params }); return data
}
export async function refresh(params: { account_id?: number } = {}): Promise<ReconciliationSummary> {
  const { data } = await apiClient.post<ReconciliationSummary>(`${base()}/refresh`, null, { params }); return data
}
export async function adjust(attemptID: number, amount: string, notes = ''): Promise<unknown> {
  const { data } = await apiClient.post(`${base()}/exceptions/${attemptID}/adjust`, { amount, notes }); return data
}
export default { summary, operations, history, exceptions, refresh, adjust }
