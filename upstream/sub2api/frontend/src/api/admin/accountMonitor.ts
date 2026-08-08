import { apiClient } from '../client'
import type { WindowStats } from '@/types'

export type AccountMonitorStatus = 'success' | 'failed' | 'unavailable' | string
export type AccountMonitorRange = '24h' | '7d' | '30d'

export interface AccountMonitorConcurrencyItem {
  account_id: number
  current: number
  limit: number
}

export interface AccountMonitorConcurrencyResponse {
  items: AccountMonitorConcurrencyItem[]
}

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

export type AccountMonitorMultiplierSource = 'declared' | 'manual' | string
export type AccountMonitorMultiplierStatus = 'ok' | 'stale' | 'unsupported' | 'failed' | 'unavailable' | string

export interface AccountMonitorMultiplier {
  value?: number | null
  source?: AccountMonitorMultiplierSource
  status: AccountMonitorMultiplierStatus
  observed_at?: string | null
  sample_count: number
}

export interface AccountMonitorBalance {
  value_usd?: number | null
  source?: 'sub2api' | 'newapi' | string
  status: 'ok' | 'stale' | 'failed' | 'unsupported' | 'unavailable' | string
  observed_at?: string | null
  last_attempt_at?: string | null
  failure_code?: string | null
}

export interface AccountMonitorScoreWeights {
  cost: number
  success: number
  ttft: number
  latency: number
  ttft_target_ms?: number
  ttft_limit_ms?: number
  latency_target_ms?: number
  latency_limit_ms?: number
  updated_by?: number
  updated_at?: string
}

export interface AccountMonitorHealthSummary {
  total_accounts: number
  monitoring_accounts: number
  available_accounts: number
  unavailable_accounts: number
  pending_accounts: number
  paused_accounts: number
  success_rate: number
  success_sample_count: number
  ttft_sample_count: number
  latency_sample_count: number
  ttft_p50_ms?: number | null
  latency_p95_ms?: number | null
}

export type AccountMonitorBucket = 'available' | 'unavailable' | 'cost_ineligible' | 'pending' | 'paused'

export interface AccountMonitorQualityEvidence {
  source: 'group' | 'global_fallback' | 'stale' | string
  sample_count: number
  success_sample_count: number
  ttft_sample_count: number
  latency_sample_count: number
  success_rate: number
  ttft_p50_ms?: number | null
  latency_p95_ms?: number | null
  observed_at?: string | null
}

export interface AccountMonitorTimelinePoint {
  status: AccountMonitorStatus
  error_code?: string
  http_status?: number | null
  ttft_ms?: number | null
  latency_ms?: number | null
  checked_at: string
}

export interface AccountMonitorAccount {
  account_id: number
  name: string
  platform: string
  account_type: string
  status: string
  schedulable: boolean
  management_state: string
  service_state: string
  group_eligibility: string
  monitor_bucket: AccountMonitorBucket
  priority: number
  homepage_url?: string
  group_ids: number[]
  group_names: string[]
  model_id: string
  latest_status: AccountMonitorStatus
  error_code?: string
  sample_count: number
  success_sample_count: number
  ttft_sample_count: number
  latency_sample_count: number
  success_rate: number
  ttft_p50_ms?: number | null
  ttft_p95_ms?: number | null
  latency_p95_ms?: number | null
  multiplier: AccountMonitorMultiplier
  request_count: number
  error_count: number
  range?: AccountMonitorRange
  base_cost?: number
  effective_multiplier?: number | null
  equivalent_site_multiplier?: number | null
  cost_mode?: 'multiplier' | 'procurement' | string
  cost_score?: number
  procurement_cost_cny?: number | null
  estimated_usable_quota_usd?: number | null
  procurement_cost_effective_at?: string | null
  balance?: AccountMonitorBalance | null
  expires_at?: string | null
  today_stats?: WindowStats | null
  usage_windows?: AccountMonitorUsageWindow[]
  latest?: AccountMonitorLatest | null
  timeline: AccountMonitorTimelinePoint[]
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
  status?: string
  platform?: string
  rate_multiplier: number
  rpm_limit?: number
  account_count?: number
  active_account_count?: number
  rate_limited_account_count?: number
  customer_visible: boolean
  native_order: number
  score_weights: AccountMonitorScoreWeights
  operational_state: 'operational' | 'unavailable' | 'closed' | string
  health: AccountMonitorHealthSummary
  accounts?: AccountMonitorAccount[]
}

export interface AccountMonitorProjection {
  schema_version: number
  range: AccountMonitorRange
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

export function list(range?: AccountMonitorRange, options?: { signal?: AbortSignal }): Promise<AccountMonitorProjection>
export function list(options?: { signal?: AbortSignal }): Promise<AccountMonitorProjection>
export async function list(
  rangeOrOptions: AccountMonitorRange | { signal?: AbortSignal } = '24h',
  options?: { signal?: AbortSignal },
): Promise<AccountMonitorProjection> {
  const range = typeof rangeOrOptions === 'string' ? rangeOrOptions : '24h'
  const requestOptions = typeof rangeOrOptions === 'string' ? options : rangeOrOptions
  const response = await apiClient.get<AccountMonitorProjection>('/admin/accounts/monitor', {
    params: { range },
    signal: requestOptions?.signal,
  })
  const { data } = response
  return data
}

export async function getConcurrency(accountIDs: number[]): Promise<AccountMonitorConcurrencyResponse> {
  const { data } = await apiClient.post<AccountMonitorConcurrencyResponse>(
    '/admin/accounts/monitor/concurrency',
    { account_ids: accountIDs },
  )
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
    undefined,
    { timeout: 240_000 },
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
  weights: Pick<AccountMonitorScoreWeights, 'cost' | 'success' | 'ttft' | 'latency' | 'ttft_target_ms' | 'ttft_limit_ms' | 'latency_target_ms' | 'latency_limit_ms'>,
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
  getConcurrency,
  updateSettings,
  runAll,
  runOne,
  history,
  getGroupScoreWeights,
  updateGroupScoreWeights,
  resetGroupScoreWeights,
}

export default accountMonitorAPI
