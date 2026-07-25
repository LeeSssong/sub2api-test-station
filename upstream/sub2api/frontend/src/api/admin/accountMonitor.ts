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

export interface AccountMonitorAccount {
  account_id: number
  name: string
  platform: string
  account_type: string
  status: string
  schedulable: boolean
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
}

export interface AccountMonitorProjection {
  schema_version: number
  observed_at: string
  stale: boolean
  settings: AccountMonitorSettings
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

const accountMonitorAPI = {
  list,
  updateSettings,
  runAll,
  runOne,
  history,
}

export default accountMonitorAPI
