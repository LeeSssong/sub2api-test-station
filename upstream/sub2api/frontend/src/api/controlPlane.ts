import { apiClient } from './client'
import {
  getExternalizationReadMode,
  normalizeReadMode,
  type CutoverEvidence,
  type ReadMode,
} from '@/config/externalizationFlags'

export type ControlPlaneReadMode = ReadMode
export type ControlPlaneReadSurface = 'account_monitor' | 'account_profitability' | 'usage'

export function resolveControlPlaneReadMode(value: unknown): ControlPlaneReadMode {
  return normalizeReadMode(value)
}

export function getControlPlaneReadMode(surface?: ControlPlaneReadSurface): ControlPlaneReadMode {
  if (surface === 'account_monitor') return getExternalizationReadMode('monitor')
  if (surface === 'account_profitability') return getExternalizationReadMode('profitability')
  if (surface === 'usage') return getExternalizationReadMode('accounting')
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
  cutover?: CutoverEvidence
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
  refreshAccount: async (accountId: number, idempotencyKey: string) => {
    const { data } = await apiClient.post(`/xingqiao/accounts/${accountId}/refresh`, {}, {
      headers: { 'Idempotency-Key': idempotencyKey },
      skipSessionRecovery: true,
    })
    return data as { account_id: number; status: string }
  },
}
