import { useEffect, useState } from 'react'
import { DEFAULT_SITE_CONFIG, loadSiteConfig, type SiteConfig } from '../domain/siteConfig'

function initialConfig(): SiteConfig {
  return {
    ...DEFAULT_SITE_CONFIG,
    apiOrigin: window.location.origin,
    support: { ...DEFAULT_SITE_CONFIG.support },
    thirdPartyReports: [],
  }
}

export function useSiteConfig() {
  const [config, setConfig] = useState<SiteConfig>(initialConfig)

  useEffect(() => {
    let active = true
    loadSiteConfig(fetch, window.location.origin).then((next) => {
      if (active) setConfig(next)
    })
    return () => { active = false }
  }, [])

  return config
}
