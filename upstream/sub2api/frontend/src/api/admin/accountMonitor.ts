import { apiClient } from '../client'
import type { WindowStats } from '@/types'

export type AccountMonitorStatus = 'success' | 'failed' | 'unavailable' | string
export type AccountModelDetectionStatus = 'untested' | 'queued' | 'running' | 'normal' | 'abnormal' | 'insufficient' | 'failed' | 'unsupported' | string
export type AccountModelDetectorState = 'ready' | 'unconfigured' | 'unavailable' | string
export type AccountMonitorRange = '24h' | '7d' | '30d'
export type AccountMonitorAvailabilityStatus = 'normal' | 'abnormal' | 'unavailable' | 'disabled' | 'stale' | string
export type AccountMonitorScoreStatus = 'eligible' | 'capped' | 'ineligible' | string
export type AccountMonitorReasonCode = 'strategy' | 'quality_gate' | 'runtime_load' | 'cooldown' | 'tie_break' | 'data_freshness' | 'not_eligible'
export type AccountMonitorGroupRecommendationStatus = 'recommended' | 'observe' | 'blocked' | 'not_recommended' | string
export type AccountMonitorGroupRecommendationTarget = 'gpt_pro' | 'gpt_plus' | 'gpt_special' | string
export type AccountMonitorGroupRecommendationAction = 'keep' | 'migrate' | 'hold' | 'none' | string

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

export interface AccountMonitorRank { rank: number; total: number }
export interface AccountMonitorRealRequestEvidence {
  request_count: number
  success_count: number
  failure_count: number
  success_rate: number
  ttft_p95_ms?: number | null
  ttft_sample_count: number
  observed_at: string
}
export interface AccountMonitorGlobalRanks {
  success_rate?: AccountMonitorRank | null
  ttft_p95?: AccountMonitorRank | null
}
export interface AccountMonitorGroupProfitability {
  group_id: number
  group_name: string
  profit_rate?: number | null
  status: 'confirmed' | 'estimated' | 'no_real_request' | 'no_revenue' | 'cost_incomplete' | string
  reason?: string
  rank?: AccountMonitorRank | null
  revenue: number
  account_cost: number
  sample_count: number
}
export interface AccountMonitorRealRequestTimelinePoint {
  start_at: string
  end_at: string
  request_count: number
  success_count: number
  failure_count: number
  ttft_p95_ms?: number | null
}

export interface AccountMonitorBalance {
  value_usd?: number | null
  source?: 'sub2api' | 'newapi' | string
  status: 'ok' | 'stale' | 'failed' | 'unsupported' | 'unavailable' | string
  observed_at?: string | null
  last_attempt_at?: string | null
  failure_code?: string | null
}

export interface AccountMonitorScoreBreakdown {
  cost: number
  success: number
  ttft: number
  latency: number
}

export interface AccountMonitorQualityScoreComponent {
  score: number
  max: number
}

export interface AccountMonitorQualityExplanationBreakdown {
  cost: AccountMonitorQualityScoreComponent
  success: AccountMonitorQualityScoreComponent
  ttft: AccountMonitorQualityScoreComponent
  latency: AccountMonitorQualityScoreComponent
}

export interface AccountMonitorQualityExplanation {
  score?: number | null
  rank?: number | null
  rank_total?: number
  breakdown?: AccountMonitorQualityExplanationBreakdown
  window: string
  sample_count: number
  source: string
  observed_at: string
  experience_label?: string
}

export interface AccountMonitorSchedulerExplanation {
  rank?: number | null
  rank_total?: number
  candidate_total?: number
  eligible: boolean
  policy_key?: string
  policy_label: string
  effective_weights?: Record<string, number>
  effective_facts?: AccountMonitorSchedulerFact[]
  model_quota_parity?: string
  candidate_scope?: string
  snapshot_at?: string | null
  primary_reason_code?: AccountMonitorReasonCode
  primary_reason_label?: string
}

export interface AccountMonitorSchedulerFact {
  label: string
  value: string
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

export interface AccountMonitorGlobalScoreWeights {
  cost: number
  success: number
  ttft: number
  latency: number
  updated_by?: number
  updated_at?: string | null
  is_default?: boolean
}

export type AccountMonitorFourScoreWeights = Pick<AccountMonitorScoreWeights, 'cost' | 'success' | 'ttft' | 'latency'>

export interface AccountMonitorGroupRecommendation {
  status: AccountMonitorGroupRecommendationStatus
  target: AccountMonitorGroupRecommendationTarget
  target_name: string
  action: AccountMonitorGroupRecommendationAction
  reason_codes: string[]
  sample_count: number
  observed_at: string
  source: 'monitor_probe' | string
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
  output_rate_tokens_per_second?: number | null
  output_rate_sample_count?: number
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
  effective_schedulable: boolean
  effective_schedulable_at: string
  effective_unschedulable_reason: string
  management_state: string
  service_state: string
  group_eligibility: string
  monitor_bucket: AccountMonitorBucket
  priority: number
  homepage_url?: string
  group_ids: number[]
  group_names: string[]
  model_id: string
  connection_probe_model?: string
  model_detection?: AccountModelDetectionProjection | null
  latest_status: AccountMonitorStatus
  error_code?: string
  probe_sample_count: number
  probe_success_count: number
  probe_success_rate: number
  probe_ttft_p50_ms?: number | null
  probe_latency_p95_ms?: number | null
  availability_status: AccountMonitorAvailabilityStatus
  score_status: AccountMonitorScoreStatus
  sample_count: number
  success_sample_count: number
  ttft_sample_count: number
  latency_sample_count: number
  success_rate: number
  ttft_p50_ms?: number | null
  ttft_p95_ms?: number | null
  latency_p95_ms?: number | null
  output_rate_tokens_per_second?: number | null
  output_rate_sample_count?: number
  multiplier: AccountMonitorMultiplier
  effective_cost_model?: 'direct_multiplier' | 'ratio_based_upstream' | 'self_owned'
  upstream_actual_cost?: number | null
  upstream_obtained_quota?: number | null
  effective_cost_a?: number | null
  effective_cost_r?: number | null
  effective_cost_u?: number | null
  effective_cost_status?: string
  request_count: number
  lifetime_real_request_count: number
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
  score_breakdown?: AccountMonitorScoreBreakdown | null
  evidence_source?: string
  group_rank?: number | null
  quality_rank?: number | null
  quality_rank_total?: number
  scheduler_rank?: number | null
  scheduler_rank_total?: number
  best_scheduler_group_name?: string
  quality_explanation?: AccountMonitorQualityExplanation | null
  scheduler_explanation?: AccountMonitorSchedulerExplanation | null
  scheduler_unavailable?: boolean
  eligible?: boolean
  evidence?: AccountMonitorQualityEvidence
  group_recommendation?: AccountMonitorGroupRecommendation | null
  real_request_evidence?: AccountMonitorRealRequestEvidence | null
  global_ranks?: AccountMonitorGlobalRanks
  group_profitability?: AccountMonitorGroupProfitability | null
  upstream_multiplier?: AccountMonitorMultiplier | null
  real_request_timeline?: AccountMonitorRealRequestTimelinePoint[]
}

export interface AccountModelDetectionModelOption {
  id: string
  supported: boolean
  selected: boolean
  reason?: string
}

export interface AccountModelDetectionSettings {
  account_id: number
  connection_probe_model: string
  model_detection_model: string
  updated_by?: number
  updated_at?: string
}

export interface AccountModelDetectionSummary {
  status: AccountModelDetectionStatus
  model_id?: string
  claimed_model?: string
  juice_status?: string
  juice_summary?: Record<string, unknown>
  fingerprint_candidate?: string
  fingerprint_similarity?: Record<string, unknown>
  detector_version?: string
  error_code?: string
  error_message?: string
  queued_at?: string
  started_at?: string
  finished_at?: string
  run_id?: string
  source?: 'current' | 'historical_final' | string
  profile?: 'low' | 'medium' | 'high' | 'unknown' | string
  mode?: 'monitor' | 'manual' | 'escalation' | 'historical' | string
  trigger_reason?: string
  planned_requests?: number
  valid_samples?: number
  evidence_state?: 'complete' | 'insufficient' | 'unavailable' | 'historical' | string
  fingerprint_status?: string
}

export interface AccountModelDetectionProjection {
  status: AccountModelDetectionStatus
  detector_state?: AccountModelDetectorState
  settings: AccountModelDetectionSettings
  model_options: AccountModelDetectionModelOption[]
  recent?: AccountModelDetectionSummary | null
  current?: AccountModelDetectionSummary | null
}

export interface AccountModelDetectionModelsResponse {
  account_id: number
  detector_state: AccountModelDetectorState
  connection_probe_model: string
  model_detection_model: string
  connection_models: AccountModelDetectionModelOption[]
  detection_models: AccountModelDetectionModelOption[]
}

export interface AccountModelDetectionRunResponse {
  status: AccountModelDetectionStatus
  run_id: string
  reused: boolean
}

export interface AccountModelDetectionHistoryResponse {
  items: AccountModelDetectionSummary[]
  next_cursor?: string
}

export interface AccountModelDetectionHistoryParams {
  limit?: number
  cursor?: string
  status?: string
  profile?: string
  mode?: string
  juice_status?: string
  fingerprint_status?: string
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
  require_privacy_set?: boolean
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

export async function getModelDetectionModels(accountID: number): Promise<AccountModelDetectionModelsResponse> {
  const { data } = await apiClient.get<AccountModelDetectionModelsResponse>(`/admin/account-monitors/${accountID}/models`)
  return data
}

export async function saveModelDetectionModels(accountID: number, payload: Pick<AccountModelDetectionModelsResponse, 'connection_probe_model' | 'model_detection_model'>): Promise<AccountModelDetectionModelsResponse> {
  const { data } = await apiClient.put<AccountModelDetectionModelsResponse>(`/admin/account-monitors/${accountID}/models`, payload)
  return data
}

export async function enqueueModelDetection(accountID: number): Promise<AccountModelDetectionRunResponse> {
  const { data } = await apiClient.post<AccountModelDetectionRunResponse>(`/admin/account-monitors/${accountID}/detection`)
  return data
}

export async function modelDetectionHistory(accountID: number, params: AccountModelDetectionHistoryParams = {}): Promise<AccountModelDetectionHistoryResponse> {
  const { data } = await apiClient.get<AccountModelDetectionHistoryResponse>(`/admin/account-monitors/${accountID}/detection`, { params })
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

export async function getGlobalScoreWeights(): Promise<AccountMonitorGlobalScoreWeights> {
  const { data } = await apiClient.get<AccountMonitorGlobalScoreWeights>('/admin/account-monitors/global-score-weights')
  return data
}

export async function updateGlobalScoreWeights(weights: AccountMonitorFourScoreWeights): Promise<AccountMonitorGlobalScoreWeights> {
  const { data } = await apiClient.put<AccountMonitorGlobalScoreWeights>('/admin/account-monitors/global-score-weights', weights)
  return data
}

export async function resetGlobalScoreWeights(): Promise<AccountMonitorGlobalScoreWeights> {
  const { data } = await apiClient.delete<AccountMonitorGlobalScoreWeights>('/admin/account-monitors/global-score-weights')
  return data
}

const accountMonitorAPI = {
  list,
  getConcurrency,
  updateSettings,
  runAll,
  runOne,
  history,
  getModelDetectionModels,
  saveModelDetectionModels,
  enqueueModelDetection,
  modelDetectionHistory,
  getGroupScoreWeights,
  updateGroupScoreWeights,
  resetGroupScoreWeights,
  getGlobalScoreWeights,
  updateGlobalScoreWeights,
  resetGlobalScoreWeights,
}

export default accountMonitorAPI
