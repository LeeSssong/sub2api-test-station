import { AppWindow, Bot, CircleGauge, Route, Send, Sparkles } from 'lucide-react'
import { useEffect, useRef, useState, type CSSProperties } from 'react'
import { useReducedMotionPreference } from '../hooks/useReducedMotion'

type Phase = 'send' | 'route' | 'observe'

const steps = [
  { id: 'send' as const, number: '01', title: '发送', text: '一个端点、一把密钥，请求原样发出，无需改动调用方式。' },
  { id: 'route' as const, number: '02', title: '路由', text: '网关为每一次调用实时选择健康的模型通道。' },
  { id: 'observe' as const, number: '03', title: '观测', text: '延迟、Token 与结果状态，每一次调用都有迹可循。' },
]

const PLAYBACK_DURATION = 7600

const clamp = (value: number) => Math.max(0, Math.min(1, value))

export function phaseForCycleProgress(progress: number): Phase {
  return progress < .3 ? 'send' : progress < .55 ? 'route' : 'observe'
}

export function RequestJourney() {
  const root = useRef<HTMLElement>(null)
  const reduced = useReducedMotionPreference()
  const [phase, setPhase] = useState<Phase>('send')
  const [progress, setProgress] = useState(reduced ? 1 : 0)
  const [playing, setPlaying] = useState(false)

  useEffect(() => {
    const element = root.current
    if (reduced || !element) return

    let frame: number | null = null
    let startedAt = 0

    const stop = () => {
      if (frame !== null) window.cancelAnimationFrame(frame)
      frame = null
      setPlaying(false)
      setProgress(0)
      setPhase('send')
    }

    const tick = (now: number) => {
      const cycleProgress = ((now - startedAt) % PLAYBACK_DURATION) / PLAYBACK_DURATION
      setProgress(cycleProgress)
      const next = phaseForCycleProgress(cycleProgress)
      setPhase((current) => current === next ? current : next)
      frame = window.requestAnimationFrame(tick)
    }

    const start = () => {
      if (frame !== null) return
      startedAt = window.performance.now()
      setPlaying(true)
      frame = window.requestAnimationFrame(tick)
    }

    const observer = new IntersectionObserver(([entry]) => {
      if (entry?.isIntersecting) start()
      else stop()
    }, { threshold: .35 })

    observer.observe(element)
    return () => {
      observer.disconnect()
      stop()
    }
  }, [reduced])

  const visiblePhase = reduced ? 'static' : phase
  const bounded = clamp(progress)
  const outgoingProgress = reduced ? 1 : clamp(bounded / .28)
  const routedProgress = reduced ? 1 : clamp((bounded - .34) / .24)
  const latency = reduced ? 187 : Math.round(187 * clamp((bounded - .58) / .34))
  const tokens = reduced ? 2148 : Math.round(2148 * clamp((bounded - .62) / .32))
  const trackStyle = (trackProgress: number) => ({
    '--track-progress': trackProgress,
    '--track-position': `${trackProgress * 100}%`,
  } as CSSProperties)

  return (
    <section
      ref={root}
      className="request-journey"
      aria-label="一次 API 请求的完整旅程"
      data-journey-phase={visiblePhase}
      data-journey-mode={reduced ? 'static' : 'auto'}
      data-playback-state={reduced ? 'static' : playing ? 'playing' : 'paused'}
    >
      <div className="journey-stage stack-panel">
        <div className="journey-glow" aria-hidden="true" />
        <div className="journey-mesh" aria-hidden="true" />
        <div className="journey-content">
          <div className="journey-head">
            <div>
              <p className="eyebrow"><span />实时链路</p>
              <h2>跟随一次请求</h2>
            </div>
            <div className="journey-metrics" aria-label="请求观测指标" data-telemetry-target="latency-token">
              <span><span>延迟</span><strong><b>{latency}</b><small>ms</small></strong></span>
              <span><span>Token</span><strong>{tokens.toLocaleString('en-US')}</strong></span>
            </div>
          </div>

          <div className="request-map" aria-label="应用经星桥连接模型通道">
            <div className="map-node app-node"><AppWindow aria-hidden="true" /><span>你的应用</span></div>
            <div className="map-track map-track--out" data-flow-direction="forward" style={trackStyle(outgoingProgress)} aria-hidden="true"><i /></div>
            <div className="map-node gateway-node"><img src="/home-assets/xingqiao-logo-256-v1.webp" alt="" /><span>星桥</span><i /></div>
            <div className="map-track map-track--route" data-flow-direction="forward" data-telemetry-source="route" style={trackStyle(routedProgress)} aria-hidden="true"><i /></div>
            <div className="provider-stack">
              <span><Bot aria-hidden="true" />OpenAI</span>
              <span><Sparkles aria-hidden="true" />Claude</span>
              <span><CircleGauge aria-hidden="true" />Gemini</span>
            </div>
          </div>

          <div className="journey-steps">
            {steps.map((step) => (
              <article key={step.id} className={phase === step.id || reduced ? 'is-active' : ''}>
                <span className="step-number">{step.number}</span>
                {step.id === 'send' ? <Send aria-hidden="true" /> : step.id === 'route' ? <Route aria-hidden="true" /> : <CircleGauge aria-hidden="true" />}
                <h3>{step.title}</h3>
                <p>{step.text}</p>
              </article>
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}
