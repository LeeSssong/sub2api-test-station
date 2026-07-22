export type ReportStatus = 'verified' | 'reference' | 'archived'

export interface ThirdPartyReport {
  id: string
  provider: string
  title: string
  description?: string
  url: string
  status: ReportStatus
}

export interface SiteConfig {
  version: 1
  apiOrigin: string
  support: { qqGroup: string }
  thirdPartyReports: ThirdPartyReport[]
}

export const DEFAULT_SITE_CONFIG: SiteConfig = {
  version: 1,
  apiOrigin: '',
  support: { qqGroup: '1080152144' },
  thirdPartyReports: [],
}

type Fetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

const reportStatuses = new Set<ReportStatus>(['verified', 'reference', 'archived'])

function cleanString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function resolveApiOrigin(value: unknown, fallbackOrigin: string): string {
  const candidate = cleanString(value) || fallbackOrigin

  try {
    const url = new URL(candidate)
    return url.protocol === 'http:' || url.protocol === 'https:' ? url.origin : fallbackOrigin
  } catch {
    return fallbackOrigin
  }
}

function parseReport(value: unknown): ThirdPartyReport | null {
  if (!value || typeof value !== 'object') return null

  const source = value as Record<string, unknown>
  const id = cleanString(source.id)
  const provider = cleanString(source.provider)
  const title = cleanString(source.title)
  const description = cleanString(source.description)
  const url = cleanString(source.url)
  const status = cleanString(source.status) as ReportStatus

  if (!id || !provider || !title || !reportStatuses.has(status)) return null

  try {
    if (new URL(url).protocol !== 'https:') return null
  } catch {
    return null
  }

  return {
    id,
    provider,
    title,
    ...(description ? { description } : {}),
    url,
    status,
  }
}

function fallback(origin: string): SiteConfig {
  return {
    ...DEFAULT_SITE_CONFIG,
    apiOrigin: resolveApiOrigin('', origin),
    support: { ...DEFAULT_SITE_CONFIG.support },
    thirdPartyReports: [],
  }
}

export async function loadSiteConfig(fetcher: Fetcher, origin: string): Promise<SiteConfig> {
  const safeFallback = fallback(origin)

  try {
    const response = await fetcher(new URL('/home-assets/site-config.json', origin), {
      cache: 'no-store',
      headers: { Accept: 'application/json' },
    })
    if (!response.ok) return safeFallback

    const value: unknown = await response.json()
    if (!value || typeof value !== 'object') return safeFallback

    const source = value as Record<string, unknown>
    if (source.version !== 1) return safeFallback

    const support = source.support && typeof source.support === 'object'
      ? source.support as Record<string, unknown>
      : {}
    const qqGroup = cleanString(support.qqGroup)
    const reports = Array.isArray(source.thirdPartyReports)
      ? source.thirdPartyReports.map(parseReport).filter((report): report is ThirdPartyReport => report !== null)
      : []

    return {
      version: 1,
      apiOrigin: resolveApiOrigin(source.apiOrigin, origin),
      support: { qqGroup: /^\d{10}$/.test(qqGroup) ? qqGroup : DEFAULT_SITE_CONFIG.support.qqGroup },
      thirdPartyReports: reports,
    }
  } catch {
    return safeFallback
  }
}
