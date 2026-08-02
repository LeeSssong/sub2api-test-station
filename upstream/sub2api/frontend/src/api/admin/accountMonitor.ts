import { apiClient } from '../client'
import type { WindowStats } from '@/types'

export type AccountMonitorStatus = 'success' | 'failed' | 'unavailable' | string

export interface AccountMonitorSettings {
  interval_seconds: number
  updated_by: number
  updated_at: string
}

export interface AccountMonitorUsageWindow {
  name: string
  utilization: number
  resets_at?: string | null
  requests: number
  tokens: number
}

export interface AccountMonitorLatest {
  status: AccountMonitorStatus
  error_code?: string
  http_status?: number | null
  ttft_ms?: number | null
  latency_ms?: number | null
  checked_at: string
}

export type AccountMonitorMultiplierSource = 'declared' | 'measured' | string
export type AccountMonitorMultiplierStatus = 'ok' | 'stale' | 'unsupported' | 'failed' | 'unavailable' | string

export interface AccountMonitorMultiplier {
  value?: number | null
  source?: AccountMonitorMultiplierSource
  status: AccountMonitorMultiplierStatus
  observed_at?: string | null
}

export interface AccountMonitorScoreWeights {
  cost: number
  success: number
  ttft: number
  latency: number
  updated_by?: number
  updated_at?: string
}

export interface AccountMonitorHealthSummary {
  total_accounts: number
  available_accounts: number
  unavailable_accounts: number
  pending_accounts: number
  paused_accounts: number
  success_rate: number
  ttft_p50_ms?: number | null
  latency_p95_ms?: number | null
}

export interface AccountMonitorQualityEvidence {
  source: 'group' | 'global_fallback' | 'stale' | string
  sample_count: number
  success_rate: number
  ttft_p50_ms?: number | null
  latency_p95_ms?: number | null
  observed_at?: string | null
}

export interface AccountMonitorAccount {
  account_id: number
  name: string
  platform: string
  account_type: string
  status: string
  schedulable: boolean
  priority: number
  homepage_url?: string
  group_ids: number[]
  group_names: string[]
  model_id: string
  latest_status: AccountMonitorStatus
  error_code?: string
  sample_count: number
  success_rate: number
  ttft_p50_ms?: number | null
  ttft_p95_ms?: number | null
  latency_p95_ms?: number | null
  multiplier: AccountMonitorMultiplier
  request_count: number
  error_count: number
  today_stats?: WindowStats | null
  usage_windows?: AccountMonitorUsageWindow[]
  latest?: AccountMonitorLatest | null
  checked_at?: string | null
  stale: boolean
  quality_score?: number | null
  group_rank?: number | null
  eligible?: boolean
  evidence?: AccountMonitorQualityEvidence
}

export interface AccountMonitorGroup {
  id: number
  name: string
  rate_multiplier: number
  customer_visible: boolean
  native_order: number
  score_weights: AccountMonitorScoreWeights
  operational_state: 'operational' | 'unavailable' | 'closed' | string
  health: AccountMonitorHealthSummary
  accounts?: AccountMonitorAccount[]
}

export interface AccountMonitorProjection {
  schema_version: number
  observed_at: string
  stale: boolean
  settings: AccountMonitorSettings
  health: AccountMonitorHealthSummary
  groups?: AccountMonitorGroup[]
  accounts: AccountMonitorAccount[]
}

export interface AccountMonitorRunResponse {
  completed: number
}

export interface AccountMonitorRunOneResponse {
  account_id: number
  model_id?: string
  status: AccountMonitorStatus
  error_code?: string
  checked_at?: string
}

export interface AccountMonitorHistoryItem {
  account_id: number
  model_id: string
  status: AccountMonitorStatus
  error_code?: string
  http_status?: number | null
  ttft_ms?: number | null
  latency_ms?: number | null
  checked_at: string
}

export interface AccountMonitorHistoryResponse {
  items: AccountMonitorHistoryItem[]
}

export async function list(options?: { signal?: AbortSignal }): Promise<AccountMonitorProjection> {
  const response = options?.signal
    ? await apiClient.get<AccountMonitorProjection>('/admin/account-monitors', { signal: options.signal })
    : await apiClient.get<AccountMonitorProjection>('/admin/account-monitors')
  const { data } = response
  return data
}

export async function updateSettings(intervalSeconds: number): Promise<AccountMonitorSettings> {
  const { data } = await apiClient.put<AccountMonitorSettings>(
    '/admin/account-monitors/settings',
    { interval_seconds: intervalSeconds },
  )
  return data
}

export async function runAll(): Promise<AccountMonitorRunResponse> {
  const { data } = await apiClient.post<AccountMonitorRunResponse>('/admin/account-monitors/run')
  return data
}

export async function runOne(accountID: number): Promise<AccountMonitorRunOneResponse> {
  const { data } = await apiClient.post<AccountMonitorRunOneResponse>(
    `/admin/account-monitors/${accountID}/run`,
  )
  return data
}

export async function history(accountID: number, limit = 25): Promise<AccountMonitorHistoryResponse> {
  const { data } = await apiClient.get<AccountMonitorHistoryResponse>(
    `/admin/account-monitors/${accountID}/history`,
    { params: { limit } },
  )
  return data
}

export async function getGroupScoreWeights(groupID: number): Promise<AccountMonitorScoreWeights> {
  const { data } = await apiClient.get<AccountMonitorScoreWeights>(
    `/admin/account-monitors/groups/${groupID}/score-weights`,
  )
  return data
}

export async function updateGroupScoreWeights(
  groupID: number,
  weights: Pick<AccountMonitorScoreWeights, 'cost' | 'success' | 'ttft' | 'latency'>,
): Promise<AccountMonitorScoreWeights> {
  const { data } = await apiClient.put<AccountMonitorScoreWeights>(
    `/admin/account-monitors/groups/${groupID}/score-weights`,
    weights,
  )
  return data
}

export async function resetGroupScoreWeights(groupID: number): Promise<AccountMonitorScoreWeights> {
  const { data } = await apiClient.delete<AccountMonitorScoreWeights>(
    `/admin/account-monitors/groups/${groupID}/score-weights`,
  )
  return data
}

const accountMonitorAPI = {
  list,
  updateSettings,
  runAll,
  runOne,
  history,
  getGroupScoreWeights,
  updateGroupScoreWeights,
  resetGroupScoreWeights,
}

export default accountMonitorAPI
