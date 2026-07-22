import { describe, expect, it, vi } from 'vitest'
import { DEFAULT_SITE_CONFIG, loadSiteConfig } from './siteConfig'

describe('loadSiteConfig', () => {
  it('falls back without reports and retains the Xingqiao QQ group', async () => {
    const fetcher = vi.fn().mockRejectedValue(new Error('offline'))

    await expect(loadSiteConfig(fetcher, 'https://api.example.com')).resolves.toEqual({
      ...DEFAULT_SITE_CONFIG,
      apiOrigin: 'https://api.example.com',
    })
  })

  it('keeps only complete HTTPS third-party reports', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: 1,
      apiOrigin: '',
      support: { qqGroup: '1080152144' },
      thirdPartyReports: [
        {
          id: 'modeloc-1',
          provider: 'MODELOC',
          title: '模型真实性报告',
          url: 'https://modeloc.com/r/report',
          status: 'verified',
        },
        {
          id: 'unsafe',
          provider: 'MODELOC',
          title: '不安全',
          url: 'http://example.com',
          status: 'verified',
        },
      ],
    })))

    const config = await loadSiteConfig(fetcher, 'https://api.example.com')

    expect(config.thirdPartyReports.map((report) => report.id)).toEqual(['modeloc-1'])
    expect(config.apiOrigin).toBe('https://api.example.com')
  })
})
