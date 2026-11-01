import { apiClient } from '@/api/client'
import type { PlazaOfficialPricing } from '@/api/modelPlaza'
export interface GroupModels { group_id: number; supported_models: string[]; official_pricing?: Record<string, PlazaOfficialPricing | null> }
export async function getGroupModels(signal?: AbortSignal, groupIDs?: number[], includePricing = false): Promise<GroupModels[]> {
  const params = { ...(groupIDs ? { group_ids: groupIDs.join(',') } : {}), ...(includePricing ? { include_pricing: true } : {}) }
  const { data } = await apiClient.get<GroupModels[]>('/groups/available-models', { signal, params })
  return data
}

export interface ModelPopularity { model: string; requests: number }
export interface ModelPopularityResponse { models: ModelPopularity[]; start_time: string; end_time: string }
export async function getModelPopularity(signal?: AbortSignal): Promise<ModelPopularityResponse> {
  const { data } = await apiClient.get<ModelPopularityResponse>('/usage/models/popularity', { signal })
  return data
}
