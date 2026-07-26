import { ArrowDown, ArrowRight } from 'lucide-react'
import { motion, useScroll, useTransform } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { HeroSignalCanvas } from '../components/HeroSignalCanvas'
import type { SessionState } from '../domain/session'
import { useHeroEntry } from '../hooks/useHeroEntry'

interface HeroSectionProps {
  session: SessionState
}

export function HeroSection({ session }: HeroSectionProps) {
  const root = useRef<HTMLElement>(null)
  const entry = useHeroEntry()
  const [scrolled, setScrolled] = useState(false)
  // The hero is sticky, so its own bounding box never moves once pinned. Scrub the
  // parallax off the raw window offset across one viewport height instead.
  const [span, setSpan] = useState(() => Math.max(1, window.innerHeight || 800))
  const { scrollY } = useScroll()
  const heroScale = useTransform(scrollY, [0, span], [1, .94])
  const contentY = useTransform(scrollY, [0, span], [0, -110])
  const signalY = useTransform(scrollY, [0, span], [0, 130])
  const scrimOpacity = useTransform(scrollY, [0, span * .82], [0, .84])

  useEffect(() => {
    const update = () => {
      setScrolled(window.scrollY > 48)
      setSpan(Math.max(1, window.innerHeight || 800))
    }
    update()
    window.addEventListener('scroll', update, { passive: true })
    window.addEventListener('resize', update)
    return () => {
      window.removeEventListener('scroll', update)
      window.removeEventListener('resize', update)
    }
  }, [])

  return (
    <motion.section
      ref={root}
      className={`hero${entry.started ? ' hero-entry-started' : ''}`}
      aria-labelledby="hero-title"
      aria-label="星桥首页首屏"
      data-entry-state={entry.reduced ? 'final' : entry.started ? 'started' : 'waiting'}
      style={{
        '--hero-entry-x': entry.origin.x,
        '--hero-entry-y': entry.origin.y,
        ...(entry.reduced ? {} : { scale: heroScale }),
      } as React.CSSProperties}
      onPointerMove={(event) => {
        const rect = event.currentTarget.getBoundingClientRect()
        entry.updateOrigin(event.clientX - rect.left, event.clientY - rect.top)
        entry.start()
      }}
      onFocusCapture={entry.start}
    >
      <motion.div className="hero-signal-layer" style={entry.reduced ? undefined : { y: signalY }}>
        <HeroSignalCanvas active={entry.started} label="星桥实时信号背景" />
      </motion.div>
      <div className="hero-ambient" aria-hidden="true" />
      <div className="hero-grid-layer" aria-hidden="true" />
      <div className="hero-ripple" aria-hidden="true" />
      <div className="hero-shade" aria-hidden="true" />
      <motion.div className="hero-inner" style={entry.reduced ? undefined : { y: contentY }}>
        <div className="endpoint-kicker">
          <span className="status-dot" aria-hidden="true" />
          <span>首尔节点 · 稳定运行</span>
        </div>
        <div className="hero-grid" data-layout="diagonal" data-composition="raised-diagonal">
          <h1 id="hero-title" className="hero-title">
            <span className="hero-brand">星桥</span>
            <span className="hero-tagline">链接世界顶尖模型</span>
          </h1>
          <div className="hero-pitch">
            <p>
              <span>GPT、Claude、Gemini 一站接入。</span>
              <span>国内网络直接连接，注册即可使用。</span>
            </p>
            <div className="hero-actions">
              <a className="primary-cta" href={session.ctaHref}>
                {session.ctaLabel}
                <ArrowRight aria-hidden="true" />
              </a>
              {session.kind !== 'guest' && <a className="secondary-cta" href="/docs/">查看文档</a>}
            </div>
          </div>
        </div>
      </motion.div>
      <motion.div
        className="hero-scrim"
        aria-hidden="true"
        style={entry.reduced ? undefined : { opacity: scrimOpacity }}
      />
      <a className={`scroll-cue${scrolled ? ' is-dismissed' : ''}`} href="#value" aria-label="向下探索更多内容">
        <span>向下探索</span>
        <ArrowDown aria-hidden="true" />
      </a>
    </motion.section>
  )
}
