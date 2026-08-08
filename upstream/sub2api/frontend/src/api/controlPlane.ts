import { apiClient } from './client'

export type ReadModelFreshness = {
  generated_at?: string
  source_watermark?: string
  freshness_seconds?: number
  completeness?: string
  calculation_version?: string
}

export type ControlPlaneResponse<T = unknown> = {
  items: T
  total?: number
  freshness?: ReadModelFreshness
  degraded?: boolean
  degraded_reason?: string
}

const get = async <T>(path: string, params?: Record<string, string | number | undefined>) => {
  const { data } = await apiClient.get<ControlPlaneResponse<T>>(path, { params })
  return data
}

export const controlPlaneAPI = {
  monitor: (params?: Record<string, string | number | undefined>) => get('/xingqiao/accounts/monitor', params),
  profitability: (params?: Record<string, string | number | undefined>) => get('/xingqiao/operations/profitability', params),
  ledger: (params?: Record<string, string | number | undefined>) => get('/xingqiao/accounting/ledger', params),
  reconciliation: (params?: Record<string, string | number | undefined>) => get('/xingqiao/reconciliation', params),
  refreshAccount: async (accountId: number, idempotencyKey: string) => {
    const { data } = await apiClient.post(`/xingqiao/accounts/${accountId}/refresh`, {}, { headers: { 'Idempotency-Key': idempotencyKey } })
    return data as { account_id: number; status: string }
  },
}

