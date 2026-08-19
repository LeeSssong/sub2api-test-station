import { apiClient } from '@/api/client'

export const CODEX_RADAR_KEYS = ['daily_development', 'hard_problems', 'background_automation', 'lobster_tasks'] as const
export type CodexRadarKey = (typeof CODEX_RADAR_KEYS)[number]

export interface CodexRadarItem {
  model: string
  effort: string
  iq: number
  average_duration_minutes: number
  average_cost_usd: number
  rule: string
}

export interface CodexRadarRecommendation {
  key: CodexRadarKey
  title: string
  rule: string
  items: CodexRadarItem[]
}

export interface CodexRadarInsights {
  generated_at: string
  source_updated_at: string
  stale: boolean
  recommendations: CodexRadarRecommendation[]
}

function object(value: unknown, path: string): Record<string, unknown> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) throw new Error(`${path} must be an object`)
  return value as Record<string, unknown>
}

function string(value: unknown, path: string, max = 2048): string {
  if (typeof value !== 'string' || value.trim() === '' || [...value].length > max) throw new Error(`${path} must be a bounded string`)
  return value
}

function number(value: unknown, path: string): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) throw new Error(`${path} must be a non-negative number`)
  return value
}

export function parseCodexRadarInsights(value: unknown): CodexRadarInsights {
  const source = object(value, 'insights')
  const generatedAt = string(source.generated_at, 'generated_at', 64)
  const sourceUpdatedAt = string(source.source_updated_at, 'source_updated_at', 64)
  if (Number.isNaN(Date.parse(generatedAt)) || Number.isNaN(Date.parse(sourceUpdatedAt))) throw new Error('timestamps must be RFC3339')
  if (typeof source.stale !== 'boolean') throw new Error('stale must be boolean')
  if (!Array.isArray(source.recommendations) || source.recommendations.length !== CODEX_RADAR_KEYS.length) throw new Error('four recommendations required')
  const recommendations = source.recommendations.map((raw, index) => {
    const recommendation = object(raw, `recommendations.${index}`)
    const key = string(recommendation.key, `recommendations.${index}.key`, 64) as CodexRadarKey
    if (key !== CODEX_RADAR_KEYS[index]) throw new Error('recommendation order changed')
    if (!Array.isArray(recommendation.items) || recommendation.items.length < 1 || recommendation.items.length > 2) throw new Error('one or two items required')
    return {
      key,
      title: string(recommendation.title, `recommendations.${index}.title`, 128),
      rule: string(recommendation.rule, `recommendations.${index}.rule`),
      items: recommendation.items.map((rawItem, itemIndex) => {
        const item = object(rawItem, `recommendations.${index}.items.${itemIndex}`)
        return {
          model: string(item.model, 'model', 128),
          effort: string(item.effort, 'effort', 64),
          iq: number(item.iq, 'iq'),
          average_duration_minutes: number(item.average_duration_minutes, 'average_duration_minutes'),
          average_cost_usd: number(item.average_cost_usd, 'average_cost_usd'),
          rule: string(item.rule, 'rule'),
        }
      }),
    }
  })
  return { generated_at: generatedAt, source_updated_at: sourceUpdatedAt, stale: source.stale, recommendations }
}

export async function getCodexRadarInsights(signal?: AbortSignal): Promise<CodexRadarInsights> {
  const { data } = await apiClient.get('/monitor-v2/codexradar-insights', { signal })
  return parseCodexRadarInsights(data)
}
