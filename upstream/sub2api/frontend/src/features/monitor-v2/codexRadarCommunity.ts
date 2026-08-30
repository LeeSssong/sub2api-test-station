import { apiClient } from '@/api/client'

export const CODEX_RADAR_COMMUNITY_KEYS = ['comprehensive', 'software', 'visual'] as const
export type CodexRadarCommunityKey = (typeof CODEX_RADAR_COMMUNITY_KEYS)[number]

export interface CodexRadarCommunityPoint {
  model: string
  effort: string
  samples: number
  iq: number
  average_cost_usd: number | null
  average_duration_minutes: number | null
  software_samples?: number
  visual_samples?: number
  software_iq?: number
  visual_iq?: number
}

export interface CodexRadarCommunityTab {
  key: CodexRadarCommunityKey
  source_updated_at: string
  status: 'fresh' | 'unavailable'
  error_code?: string
  points: CodexRadarCommunityPoint[]
}

export interface CodexRadarCommunity {
  generated_at: string
  source_status: 'fresh' | 'partial' | 'stale'
  stale: boolean
  tabs: CodexRadarCommunityTab[]
}

function object(value: unknown, path: string): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) throw new Error(`${path} must be an object`)
  return value as Record<string, unknown>
}

function string(value: unknown, path: string, max: number): string {
  if (typeof value !== 'string' || value.trim() === '' || [...value].length > max) throw new Error(`${path} must be a bounded string`)
  return value
}

function number(value: unknown, path: string): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) throw new Error(`${path} must be a finite number`)
  return value
}

function nullableNumber(value: unknown, path: string): number | null {
  return value === null ? null : number(value, path)
}

function integer(value: unknown, path: string): number {
  const parsed = number(value, path)
  if (!Number.isInteger(parsed)) throw new Error(`${path} must be an integer`)
  return parsed
}

function timestamp(value: unknown, path: string): string {
  const parsed = string(value, path, 64)
  if (Number.isNaN(Date.parse(parsed))) throw new Error(`${path} must be RFC3339`)
  return parsed
}

function optionalTimestamp(value: unknown, path: string): string {
  if (value === '') return ''
  return timestamp(value, path)
}

export function parseCodexRadarCommunity(value: unknown): CodexRadarCommunity {
  const source = object(value, 'community')
  const generatedAt = timestamp(source.generated_at, 'generated_at')
  if (typeof source.stale !== 'boolean') throw new Error('stale must be boolean')
  if (!Array.isArray(source.tabs) || source.tabs.length !== CODEX_RADAR_COMMUNITY_KEYS.length) throw new Error('three tabs required')
  const tabs = source.tabs.map((rawTab, tabIndex) => {
    const tab = object(rawTab, `tabs.${tabIndex}`)
    const key = string(tab.key, `tabs.${tabIndex}.key`, 32) as CodexRadarCommunityKey
    if (key !== CODEX_RADAR_COMMUNITY_KEYS[tabIndex]) throw new Error('community tab order changed')
    if (!Array.isArray(tab.points)) throw new Error('points must be an array')
    const status: CodexRadarCommunityTab['status'] = tab.status === undefined ? 'fresh' : string(tab.status, `tabs.${tabIndex}.status`, 16) as CodexRadarCommunityTab['status']
    if (status !== 'fresh' && status !== 'unavailable') throw new Error('invalid tab status')
    const errorCode = tab.error_code === undefined ? undefined : string(tab.error_code, `tabs.${tabIndex}.error_code`, 64)
    if (status === 'unavailable' && !errorCode) throw new Error('unavailable tab requires error_code')
    const points = tab.points.map((rawPoint, pointIndex) => {
      const point = object(rawPoint, `tabs.${tabIndex}.points.${pointIndex}`)
      const model = string(point.model, 'model', 128)
      const effort = string(point.effort, 'effort', 64)
      const parsed: CodexRadarCommunityPoint = {
        model,
        effort,
        samples: integer(point.samples, 'samples'),
        iq: number(point.iq, 'iq'),
        average_cost_usd: nullableNumber(point.average_cost_usd, 'average_cost_usd'),
        average_duration_minutes: nullableNumber(point.average_duration_minutes, 'average_duration_minutes'),
      }
      if (key === 'comprehensive') {
        parsed.software_samples = integer(point.software_samples, 'software_samples')
        parsed.visual_samples = integer(point.visual_samples, 'visual_samples')
        parsed.software_iq = number(point.software_iq, 'software_iq')
        parsed.visual_iq = number(point.visual_iq, 'visual_iq')
      }
      return parsed
    })
    return { key, source_updated_at: status === 'unavailable' ? optionalTimestamp(tab.source_updated_at, 'source_updated_at') : timestamp(tab.source_updated_at, 'source_updated_at'), status, error_code: errorCode, points }
  })
  const sourceStatus = source.source_status === undefined ? (source.stale ? 'stale' : 'fresh') : string(source.source_status, 'source_status', 16)
  if (!['fresh', 'partial', 'stale'].includes(sourceStatus)) throw new Error('invalid source_status')
  return { generated_at: generatedAt, source_status: sourceStatus as CodexRadarCommunity['source_status'], stale: source.stale, tabs }
}

export async function getCodexRadarCommunity(signal?: AbortSignal): Promise<CodexRadarCommunity> {
  const { data } = await apiClient.get('/public/codexradar/community', { signal })
  return parseCodexRadarCommunity(data)
}
