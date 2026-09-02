import { apiClient } from '../client'

export type SchedulerLogRange = '1h' | '24h' | '7d'

export interface SchedulerLogSummary {
  logical_request_id: string
  started_at: string
  canonical_model?: string
  group_id?: number | null
  selected_account_id?: number | null
  algorithm_version: string
  runtime_retry_budget: number
  switch_count: number
  final_outcome: string
}

export interface SchedulerLogEvent {
  id: number
  event_at: string
  attempt_id?: string
  attempt_number: number
  event_name: string
  account_id?: number | null
  selection_layer?: string
  outcome?: string
  decision?: Record<string, unknown>
}

export interface SchedulerLogListResponse {
  items: SchedulerLogSummary[]
  next_cursor?: string | null
  incomplete: boolean
  dropped_count: number
}

export interface SchedulerLogDetail {
  logical_request_id: string
  algorithm_version: string
  runtime_retry_budget: number
  switch_count: number
  final_outcome: string
  events: SchedulerLogEvent[]
}

export interface SchedulerLogListParams {
  time_range?: SchedulerLogRange
  cursor?: string
  limit?: number
  group_id?: number
  account_id?: number
  outcome?: string
  mechanism?: string
  q?: string
}

export async function list(params: SchedulerLogListParams): Promise<SchedulerLogListResponse> {
  const { data } = await apiClient.get<SchedulerLogListResponse>('/admin/scheduler/logs', { params })
  type RawSummary = Partial<SchedulerLogSummary> & { decision?: Record<string, unknown>; event_at?: string; event_name?: string; account_id?: number; outcome?: string }
  const raw = data as Omit<SchedulerLogListResponse, 'items'> & { items?: RawSummary[] }
  const items: SchedulerLogSummary[] = (raw.items ?? []).map((item) => {
    const decision = item.decision ?? {}
    return { logical_request_id: item.logical_request_id || '', algorithm_version: item.algorithm_version || 'unknown', ...item, started_at: item.started_at || item.event_at || '', selected_account_id: item.selected_account_id ?? item.account_id ?? (Number(decision.selected_account_id) || null), runtime_retry_budget: item.runtime_retry_budget ?? (Number(decision.extra_retry_count) || 0), switch_count: item.switch_count ?? (Number(decision.switch_count) || 0), final_outcome: item.final_outcome || item.outcome || 'unknown' }
  })
  return { ...raw, items }
}

export async function getDetail(logicalRequestID: string): Promise<SchedulerLogDetail> {
  const { data } = await apiClient.get<SchedulerLogDetail>(`/admin/scheduler/logs/${encodeURIComponent(logicalRequestID)}`)
  const raw = data as SchedulerLogDetail & { attempts?: SchedulerLogEvent[] }
  return { ...raw, events: raw.events ?? raw.attempts ?? [] }
}

export default { list, getDetail }
