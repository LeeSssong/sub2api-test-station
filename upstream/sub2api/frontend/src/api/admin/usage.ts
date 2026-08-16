/**
 * Admin Usage API endpoints
 * Handles admin-level usage logs and statistics retrieval
 */

import { apiClient } from '../client'
import type { AdminUsageCostExceptionList, AdminUsageLog, UsageCostEvidenceDetail, UsageCostFilteredReviewResult, UsageCostReviewResult, UsageQueryParams, PaginatedResponse, UsageRequestType } from '@/types'
import type { EndpointStat } from '@/types'

// ==================== Types ====================

export interface AdminUsageStatsResponse {
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_tokens: number
  total_cache_creation_tokens: number
  total_cache_read_tokens: number
  total_tokens: number
  total_cost: number
  total_actual_cost: number
  total_account_cost: number
  average_duration_ms: number
  endpoints?: EndpointStat[]
  upstream_endpoints?: EndpointStat[]
  endpoint_paths?: EndpointStat[]
}

export interface SimpleUser {
  id: number
  email: string
  deleted: boolean
}

export interface SimpleApiKey {
  id: number
  name: string
  user_id: number
}

export interface UsageCleanupFilters {
  start_time: string
  end_time: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string | null
  request_type?: UsageRequestType | null
  stream?: boolean | null
  billing_type?: number | null
}

export interface UsageCleanupTask {
  id: number
  status: string
  filters: UsageCleanupFilters
  created_by: number
  deleted_rows: number
  error_message?: string | null
  canceled_by?: number | null
  canceled_at?: string | null
  started_at?: string | null
  finished_at?: string | null
  created_at: string
  updated_at: string
}

export interface CreateUsageCleanupTaskRequest {
  start_date: string
  end_date: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string | null
  request_type?: UsageRequestType | null
  stream?: boolean | null
  billing_type?: number | null
  timezone?: string
}

export interface AdminUsageQueryParams extends UsageQueryParams {
  user_id?: number
  exact_total?: boolean
  billing_mode?: string
  upstream_model_mismatch?: boolean
  sort_by?: string
  sort_order?: 'asc' | 'desc'
  // 错误请求 tab 专属筛选(仅传给错误列表接口;共用同一 filters 对象)
  error_phase?: string | null
  error_category?: string | null
  status_code?: number | null
}

export interface CostExceptionQueryParams {
  page?: number
  page_size?: number
  account_id?: number
  start_time?: string
  end_time?: string
  search?: string
  evidence_status?: string
  review_status?: string
}

// ==================== API Functions ====================

/**
 * List all usage logs with optional filters (admin only)
 * @param params - Query parameters for filtering and pagination
 * @returns Paginated list of usage logs
 */
export async function list(
  params: AdminUsageQueryParams,
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<AdminUsageLog>> {
  const { data } = await apiClient.get<PaginatedResponse<AdminUsageLog>>('/admin/usage', {
    params,
    signal: options?.signal
  })
  return data
}

/**
 * Get a usage log by ID (admin only)
 * @param id - Usage log ID
 * @returns Usage log detail
 */
export async function getById(id: number): Promise<AdminUsageLog> {
  const { data } = await apiClient.get<AdminUsageLog>(`/admin/usage/${id}`)
  return data
}

function normalizeCostEvidence(data: unknown): UsageCostEvidenceDetail {
  if (data === null || typeof data !== 'object' || Array.isArray(data)) {
    return {} as UsageCostEvidenceDetail
  }

  const response = data as Record<string, unknown>
  const normalized: Record<string, unknown> = {}
  const fields = [
    ['usage_log_id', 'UsageLogID'],
    ['source', 'Source'],
    ['evidence_status', 'EvidenceStatus'],
    ['reason_code', 'ReasonCode'],
    ['normalized_cost_cny', 'NormalizedCostCNY'],
    ['review_id', 'ReviewID'],
    ['review_cost_cny', 'ReviewCostCNY']
  ] as const

  for (const [snakeCase, pascalCase] of fields) {
    const value = response[snakeCase] !== undefined ? response[snakeCase] : response[pascalCase]
    if (value !== undefined) {
      normalized[snakeCase] = value
    }
  }

  return normalized as UsageCostEvidenceDetail
}

/** Read persisted local evidence/review facts for one administrator usage row. */
export async function getCostEvidence(usageId: number): Promise<UsageCostEvidenceDetail> {
  const { data } = await apiClient.get<unknown>(`/admin/usage/${usageId}/upstream-cost`)
  return normalizeCostEvidence(data)
}

export async function listCostExceptions(params: CostExceptionQueryParams, options?: { signal?: AbortSignal }): Promise<AdminUsageCostExceptionList> {
  const { data } = await apiClient.get<AdminUsageCostExceptionList>('/admin/usage/cost-exceptions', { params, signal: options?.signal })
  return data
}

export async function reviewOne(usageLogId: number, payload: { manual_cost_cny?: number }): Promise<UsageCostReviewResult> {
  const { data } = await apiClient.post<UsageCostReviewResult>(`/admin/usage/cost-exceptions/${usageLogId}/review`, payload)
  return data
}

export async function reviewSelected(payload: { usage_log_ids: number[]; manual_cost_cny?: number }): Promise<UsageCostReviewResult[]> {
  const { data } = await apiClient.post<UsageCostReviewResult[]>('/admin/usage/cost-exceptions/review-selected', payload)
  return data
}

export async function reviewFiltered(payload: { filter: Omit<CostExceptionQueryParams, 'page' | 'page_size'>; max_usage_log_id: number; manual_cost_cny?: number }): Promise<UsageCostFilteredReviewResult> {
  const { data } = await apiClient.post<UsageCostFilteredReviewResult>('/admin/usage/cost-exceptions/review-filtered', payload)
  return data
}

/**
 * Get usage statistics with optional filters (admin only)
 * @param params - Query parameters for filtering
 * @returns Usage statistics
 */
export async function getStats(params: {
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string
  request_type?: UsageRequestType
  stream?: boolean
  upstream_model_mismatch?: boolean
  period?: string
  start_date?: string
  end_date?: string
  timezone?: string
  nocache?: number
}): Promise<AdminUsageStatsResponse> {
  const { data } = await apiClient.get<AdminUsageStatsResponse>('/admin/usage/stats', {
    params
  })
  return data
}

/**
 * Search users by email keyword (admin only)
 * @param keyword - Email keyword to search
 * @returns List of matching users (max 30)
 */
export async function searchUsers(keyword: string): Promise<SimpleUser[]> {
  const { data } = await apiClient.get<SimpleUser[]>('/admin/usage/search-users', {
    params: { q: keyword }
  })
  return data
}

/**
 * Search API keys by user ID and/or keyword (admin only)
 * @param userId - Optional user ID to filter by
 * @param keyword - Optional keyword to search in key name
 * @returns List of matching API keys (max 30)
 */
export async function searchApiKeys(userId?: number, keyword?: string): Promise<SimpleApiKey[]> {
  const params: Record<string, unknown> = {}
  if (userId !== undefined) {
    params.user_id = userId
  }
  if (keyword) {
    params.q = keyword
  }
  const { data } = await apiClient.get<SimpleApiKey[]>('/admin/usage/search-api-keys', {
    params
  })
  return data
}

/**
 * List usage cleanup tasks (admin only)
 * @param params - Query parameters for pagination
 * @returns Paginated list of cleanup tasks
 */
export async function listCleanupTasks(
  params: { page?: number; page_size?: number },
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<UsageCleanupTask>> {
  const { data } = await apiClient.get<PaginatedResponse<UsageCleanupTask>>('/admin/usage/cleanup-tasks', {
    params,
    signal: options?.signal
  })
  return data
}

/**
 * Create a usage cleanup task (admin only)
 * @param payload - Cleanup task parameters
 * @returns Created cleanup task
 */
export async function createCleanupTask(payload: CreateUsageCleanupTaskRequest): Promise<UsageCleanupTask> {
  const { data } = await apiClient.post<UsageCleanupTask>('/admin/usage/cleanup-tasks', payload)
  return data
}

/**
 * Cancel a usage cleanup task (admin only)
 * @param taskId - Task ID to cancel
 */
export async function cancelCleanupTask(taskId: number): Promise<{ id: number; status: string }> {
  const { data } = await apiClient.post<{ id: number; status: string }>(
    `/admin/usage/cleanup-tasks/${taskId}/cancel`
  )
  return data
}

export const adminUsageAPI = {
  list,
  getById,
  getCostEvidence,
  listCostExceptions,
  reviewOne,
  reviewSelected,
  reviewFiltered,
  getStats,
  searchUsers,
  searchApiKeys,
  listCleanupTasks,
  createCleanupTask,
  cancelCleanupTask
}

export default adminUsageAPI
