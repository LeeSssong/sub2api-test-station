import Lenis from 'lenis'
import { StrictMode, useEffect } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import type { SiteConfig } from './domain/siteConfig'
import { useSession } from './hooks/useSession'
import { useSiteConfig } from './hooks/useSiteConfig'
import { useHomepageTheme } from './hooks/useHomepageTheme'
import { SupportPage } from './pages/SupportPage'
import { bootstrapHomepageTheme } from './themeBootstrap'
import './styles.css'

export function selectRuntimePage(pathname: string): 'support' | 'home' {
  return pathname === '/support' || pathname === '/support/' ? 'support' : 'home'
}

function useSmoothScroll() {
  useEffect(() => {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    const lenis = new Lenis({ lerp: .11, smoothWheel: true })
    let frame = 0
    const tick = (time: number) => {
      lenis.raf(time)
      frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => {
      cancelAnimationFrame(frame)
      lenis.destroy()
    }
  }, [])
}

function HomeRuntimeApp({ config }: { config: SiteConfig }) {
  useSmoothScroll()
  const session = useSession()
  const { theme, toggleTheme } = useHomepageTheme()
  return <App config={config} session={session} theme={theme} onToggleTheme={toggleTheme} />
}

function RuntimeApp() {
  const config = useSiteConfig()

  if (selectRuntimePage(window.location.pathname) === 'support') {
    return <SupportPage qqGroup={config.support.qqGroup} />
  }

  return <HomeRuntimeApp config={config} />
}

const rootElement = document.getElementById('root')

if (rootElement) {
  bootstrapHomepageTheme(window.location.pathname)
  createRoot(rootElement).render(
    <StrictMode><RuntimeApp /></StrictMode>,
  )
}
