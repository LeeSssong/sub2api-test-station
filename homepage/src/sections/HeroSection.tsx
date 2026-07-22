import { ArrowDown, ArrowRight, Braces } from 'lucide-react'
import { useEffect, useState } from 'react'
import { CopyControl } from '../components/CopyControl'
import type { SessionState } from '../domain/session'
import { useReducedMotionPreference } from '../hooks/useReducedMotion'

const signalText = `A9K4 >_ AI ROUTE 09/CORE {TOKEN} BRIDGE 84F2 :: OPENAI / ANTHROPIC
7ZD1 REQUEST 21B MODEL LINK WORLD 53AC > RESPONSE 200 STREAM
K3M7 Claude GPT KEY PATH SEOUL DIRECT CN 8B12 :: LATENCY ROUTE
XQ-01 STAR BRIDGE GATEWAY /V1/ MESSAGES COMPLETIONS 73FD
API MODEL TOKEN 09AF CONNECT GLOBAL SIGNAL 42B8 OPENAI ANTHROPIC
5D91 REQUEST 21B MODEL LINK WORLD 53AC > RESPONSE 200 STREAM
K3M7 Claude GPT KEY PATH SEOUL DIRECT CN 8B12 :: LATENCY ROUTE
XQ-01 STAR BRIDGE GATEWAY /V1/ MESSAGES COMPLETIONS 73FD`

interface HeroSectionProps {
  apiOrigin: string
  session: SessionState
}

export function HeroSection({ apiOrigin, session }: HeroSectionProps) {
  const reduced = useReducedMotionPreference()
  const [scrolled, setScrolled] = useState(false)

  useEffect(() => {
    const update = () => setScrolled(window.scrollY > 48)
    update()
    window.addEventListener('scroll', update, { passive: true })
    return () => window.removeEventListener('scroll', update)
  }, [])

  return (
    <section className="hero" aria-labelledby="hero-title" data-motion-state={reduced ? 'final' : 'active'}>
      <pre className="hero-signal" aria-hidden="true">{signalText.repeat(10)}</pre>
      <div className="hero-shade" aria-hidden="true" />
      <div className="hero-inner">
        <div className="endpoint-kicker">
          <span className="status-dot" aria-hidden="true" />
          <span>韩国首尔 · 国内直连</span>
        </div>
        <div className="endpoint-row">
          <code>{apiOrigin}</code>
          <CopyControl value={apiOrigin} label="复制 API 地址" compact />
          <div className="protocol-cycle" aria-label="兼容接口路径">
            <code>/v1/chat/completions</code>
            <code>/v1/messages</code>
          </div>
        </div>
        <div className="hero-grid">
          <h1 id="hero-title">
            <span className="hero-brand">星桥</span>
            <span>链接世界顶尖模型</span>
          </h1>
          <div className="hero-pitch">
            <p>
              <span>韩国首尔节点，国内直连。</span>
              <span>兼容 OpenAI 与 Anthropic API。</span>
              <span>无需翻墙，只需更改基础 URL。</span>
            </p>
            <div className="hero-actions">
              <a className="primary-cta" href={session.ctaHref}>
                <Braces aria-hidden="true" />
                {session.ctaLabel}
                <ArrowRight aria-hidden="true" />
              </a>
              <a className="secondary-cta" href="#docs">查看文档</a>
            </div>
          </div>
        </div>
      </div>
      <a className={`scroll-cue${scrolled ? ' is-dismissed' : ''}`} href="#value" aria-label="向下探索更多内容">
        <span>向下探索</span>
        <ArrowDown aria-hidden="true" />
      </a>
    </section>
  )
}
