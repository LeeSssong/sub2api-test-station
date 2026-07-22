import { ArrowRight, Braces, Terminal } from 'lucide-react'
import { Reveal } from '../components/Reveal'

const examples = [
  { label: 'OpenAI', path: '/v1/chat/completions', sdk: 'OPENAI_BASE_URL' },
  { label: 'Anthropic', path: '/v1/messages', sdk: 'ANTHROPIC_BASE_URL' },
]

export function IntegrationSection() {
  return (
    <section className="integration-band grid-surface" id="docs" aria-labelledby="integration-title" tabIndex={-1}>
      <div className="section-inner integration-inner">
        <Reveal as="header" className="section-intro">
          <p className="eyebrow"><span />兼容接入</p>
          <h2 id="integration-title">只改基础 URL，保留熟悉的 SDK</h2>
          <p>OpenAI 与 Anthropic 使用各自原生接口路径，共用星桥网关和密钥管理。</p>
        </Reveal>
        <Reveal className="integration-grid">
          {examples.map((item) => (
            <article key={item.label} className="integration-item">
              <div className="integration-label"><Braces aria-hidden="true" /><span>{item.label}</span><ArrowRight aria-hidden="true" /></div>
              <div className="terminal-block">
                <div className="terminal-bar"><Terminal aria-hidden="true" /><span>{item.path}</span></div>
                <code>{item.sdk}=https://你的星桥域名</code>
              </div>
            </article>
          ))}
        </Reveal>
      </div>
    </section>
  )
}
