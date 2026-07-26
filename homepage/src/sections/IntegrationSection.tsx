import { ArrowRight, Terminal } from 'lucide-react'
import { Reveal } from '../components/Reveal'
import type { SiteConfig } from '../domain/siteConfig'

interface IntegrationSectionProps {
  config: SiteConfig
}

const examples = [
  { label: 'OpenAI', path: '/v1/chat/completions', sdk: 'OPENAI_BASE_URL' },
  { label: 'Anthropic', path: '/v1/messages', sdk: 'ANTHROPIC_BASE_URL' },
]

export function IntegrationSection({ config }: IntegrationSectionProps) {
  return (
    <section className="integration-band grid-surface stack-panel" id="docs" aria-labelledby="integration-title" tabIndex={-1}>
      <div className="section-inner integration-inner">
        <Reveal as="header" className="section-intro">
          <p className="eyebrow"><span />兼容接入</p>
          <h2 id="integration-title"><span className="mask-line"><span>只改基础 URL，保留熟悉的 SDK</span></span></h2>
          <p>OpenAI 与 Anthropic 使用各自原生接口路径，共用星桥网关和密钥管理。</p>
        </Reveal>
        <div className="integration-grid">
          {examples.map((item, index) => (
            <Reveal as="article" animation="scale-in" delay={index * 110} key={item.label} className="integration-item">
              <div className="integration-label" data-testid="integration-label"><span>{item.label}</span><ArrowRight aria-hidden="true" /></div>
              <div className="terminal-block">
                <div className="terminal-bar"><Terminal aria-hidden="true" /><span>{item.path}</span><b className="terminal-cursor" aria-hidden="true">|</b></div>
                <code>{item.sdk}={config.apiOrigin}</code>
              </div>
            </Reveal>
          ))}
        </div>
      </div>
    </section>
  )
}
