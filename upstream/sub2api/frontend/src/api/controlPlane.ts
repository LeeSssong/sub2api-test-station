import { apiClient } from './client'
import {
  normalizeReadMode,
  type ExternalizationPage,
  type ReadMode,
  type ServerPageReadDecision,
} from '@/config/externalizationFlags'

export type ControlPlaneReadMode = ReadMode
export type ControlPlaneReadSurface = 'account_monitor' | 'account_profitability' | 'usage'

export function resolveControlPlaneReadMode(value: unknown): ControlPlaneReadMode {
  return normalizeReadMode(value)
}

export function getControlPlaneReadMode(_surface?: ControlPlaneReadSurface): ControlPlaneReadMode {

  return 'legacy_only'
}

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
  generated_at?: string
  source_watermark?: string
  freshness_seconds?: number
  completeness?: string
  calculation_version?: string
  degraded?: boolean
  degraded_reason?: string
}

function normalizedFreshness(data: ControlPlaneResponse<unknown>): ReadModelFreshness | undefined {
  const freshness = data.freshness ?? {}
  const generatedAt = data.generated_at ?? freshness.generated_at
  const sourceWatermark = data.source_watermark ?? freshness.source_watermark
  const freshnessSeconds = data.freshness_seconds ?? freshness.freshness_seconds
  const completeness = data.completeness ?? freshness.completeness
  const calculationVersion = data.calculation_version ?? freshness.calculation_version

  if (!generatedAt && !sourceWatermark && freshnessSeconds === undefined && !completeness && !calculationVersion) {
    return undefined
  }
  return {
    generated_at: generatedAt,
    source_watermark: sourceWatermark,
    freshness_seconds: freshnessSeconds,
    completeness,
    calculation_version: calculationVersion,
  }
}

const get = async <T>(path: string, params?: Record<string, string | number | undefined>) => {
  const { data } = await apiClient.get<ControlPlaneResponse<T>>(path, {
    params,
    // A control-plane 401 is a local read failure, never a reason to clear
    // the active Sub2API administrator session.
    skipSessionRecovery: true,
  })
  return { ...data, freshness: normalizedFreshness(data) }
}

export const controlPlaneAPI = {
  monitor: (params?: Record<string, string | number | undefined>) => get('/xingqiao/accounts/monitor', params),
  profitability: (params?: Record<string, string | number | undefined>) => get('/xingqiao/operations/profitability', params),
  ledger: (params?: Record<string, string | number | undefined>) => get('/xingqiao/accounting/ledger', params),
  reconciliation: (params?: Record<string, string | number | undefined>) => get('/xingqiao/reconciliation', params),
  decision: async (page: ExternalizationPage) => {
    const { data } = await apiClient.get<ServerPageReadDecision>(`/xingqiao/externalization/pages/${page}`, {
      skipSessionRecovery: true,
    })
    return data
  },
  setReadMode: async (page: ExternalizationPage, mode: ReadMode, idempotencyKey: string) => {
    const { data } = await apiClient.post(`/xingqiao/externalization/pages/${page}/mode`, { mode }, {
      headers: { 'Idempotency-Key': idempotencyKey },
      skipSessionRecovery: true,
    })
    return data
  },
  refreshAccount: async (accountId: number, idempotencyKey: string) => {
    const { data } = await apiClient.post(`/xingqiao/accounts/${accountId}/refresh`, {}, {
      headers: { 'Idempotency-Key': idempotencyKey },
      skipSessionRecovery: true,
    })
    return data as { account_id: number; status: string }
  },
}
