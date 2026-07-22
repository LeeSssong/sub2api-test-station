import { BoundarySection } from './sections/BoundarySection'
import { Header } from './components/Header'
import { HeroSection } from './sections/HeroSection'
import { IntegrationSection } from './sections/IntegrationSection'
import { ValueSections } from './sections/ValueSections'
import { StatementSection } from './sections/StatementSection'
import { RequestJourney } from './sections/RequestJourney'
import { BrandReveal } from './sections/BrandReveal'
import type { SiteConfig } from './domain/siteConfig'
import type { SessionState } from './domain/session'

interface AppProps {
  config: SiteConfig
  session: SessionState
}

export function App({ config, session }: AppProps) {
  return (
    <>
      <a className="skip-link" href="#main-content">跳到主内容</a>
      <Header session={session} />
      <main id="main-content">
        <HeroSection apiOrigin={config.apiOrigin || window.location.origin} session={session} />
        <ValueSections config={config} />
        <BoundarySection />
        <StatementSection />
        <RequestJourney />
        <IntegrationSection />
      </main>
      <footer className="site-footer">
        <div><span>© 2026 星桥</span><span>世界顶尖模型触手可及</span></div>
        <BrandReveal />
      </footer>
    </>
  )
}
