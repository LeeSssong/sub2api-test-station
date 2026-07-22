import { BoundarySection } from './sections/BoundarySection'
import { Header } from './components/Header'
import { HeroSection } from './sections/HeroSection'
import { IntegrationSection } from './sections/IntegrationSection'
import { ValueSections } from './sections/ValueSections'
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
        <section className="statement-shell" aria-label="星桥服务声明">
          <p><span>所有顶尖模型。</span><span>一个网关。</span><span>国内直连。</span></p>
        </section>
        <section className="journey-shell grid-surface" aria-labelledby="journey-shell-title">
          <div className="section-inner">
            <h2 id="journey-shell-title">跟随一次请求</h2>
            <div className="journey-metrics"><span>延迟 <strong>187 ms</strong></span><span>Token <strong>2,148</strong></span></div>
            <div className="journey-preview">
              <span>01 发送</span><span>02 路由</span><span>03 观测</span>
            </div>
          </div>
        </section>
        <IntegrationSection />
      </main>
      <footer className="site-footer">
        <div><span>© 2026 星桥</span><span>世界顶尖模型触手可及</span></div>
        <section className="brand-shell" aria-label="星桥品牌揭幕"><h2>星桥</h2></section>
      </footer>
    </>
  )
}
