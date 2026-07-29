import { Header } from './components/Header'
import { HeroSection } from './sections/HeroSection'
import { IntegrationSection } from './sections/IntegrationSection'
import { ValueSections } from './sections/ValueSections'
import { StatementSection } from './sections/StatementSection'
import { RequestJourney } from './sections/RequestJourney'
import { BrandReveal } from './sections/BrandReveal'
import type { SiteConfig } from './domain/siteConfig'
import type { SessionState } from './domain/session'
import type { Theme } from './domain/themeSchedule'

interface AppProps {
  config: SiteConfig
  session: SessionState
  theme: Theme
  onToggleTheme: () => void
}

export function App({ config, session, theme, onToggleTheme }: AppProps) {
  return (
    <>
      <a className="skip-link" href="#main-content">跳到主内容</a>
      <Header session={session} theme={theme} onToggleTheme={onToggleTheme} />
      <main id="main-content">
        <HeroSection session={session} />
        <ValueSections config={config} />
        <StatementSection />
        <RequestJourney />
        <IntegrationSection config={config} />
      </main>
      <footer className="site-footer">
        <div><span>© 2026 星桥</span><span>世界顶尖模型触手可及</span></div>
        <BrandReveal theme={theme} />
      </footer>
    </>
  )
}
