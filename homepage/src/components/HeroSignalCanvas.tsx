import { useEffect, useRef, useState } from 'react'
import { buildSignalTile } from '../domain/signalTiling'
import { useReducedMotionPreference } from '../hooks/useReducedMotion'

const SIGNAL_FONT = '500 14px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace'

const rowSeeds = [
  'XQ ROUTE 09 CORE TOKEN BRIDGE OPENAI ANTHROPIC RESPONSE 200 STREAM',
  'SEOUL DIRECT CN MODEL LINK GLOBAL LATENCY HEALTHY UPSTREAM',
  'V1 CHAT COMPLETIONS MESSAGES EMBEDDINGS TOOLS MODELS',
  'REQUEST TRACE STAR BRIDGE GATEWAY TOKEN COST STATUS READY',
  'GPT CLAUDE GEMINI ROUTE CONNECT STREAM CACHE EDGE ONLINE',
  'TLS ENCRYPTED NO TRACKING REQUEST HEALTH VERIFIED SECURE',
  'LATENCY PACKET UPLINK DOWNLINK SESSION ACTIVE RELAY STABLE',
  'MODEL RESPONSE TOOL CALL JSON EVENT SOURCE COMPLETE OK',
]

interface SignalRow {
  text: string
  segmentWidth: number
  x: number
  y: number
  speed: number
  alpha: number
}

interface HeroSignalCanvasProps {
  active: boolean
}

export function HeroSignalCanvas({ active }: HeroSignalCanvasProps) {
  const canvas = useRef<HTMLCanvasElement>(null)
  const host = useRef<HTMLDivElement>(null)
  const frame = useRef<number | null>(null)
  const reduced = useReducedMotionPreference()
  const [canvasActive, setCanvasActive] = useState(false)

  useEffect(() => {
    const element = canvas.current
    const container = host.current
    if (!active || reduced || !element || !container) return
    const context = element.getContext('2d')
    if (!context) return

    let rows: SignalRow[] = []
    let visible = true
    let running = false
    let width = 1
    let height = 1
    let dpr = 1
    let pointerX = 0
    let velocity = 1.35
    let targetVelocity = 1.35

    const build = () => {
      const rect = container.getBoundingClientRect()
      dpr = Math.min(window.devicePixelRatio || 1, 2)
      width = Math.max(1, Math.round(rect.width))
      height = Math.max(1, Math.round(rect.height))
      element.width = Math.round(width * dpr)
      element.height = Math.round(height * dpr)
      element.style.width = `${width}px`
      element.style.height = `${height}px`
      context.setTransform(dpr, 0, 0, dpr, 0, 0)
      context.font = SIGNAL_FONT
      const count = Math.max(18, Math.ceil(height / 22) + 6)
      rows = Array.from({ length: count }, (_, index) => {
        const rowText = `${rowSeeds[index % rowSeeds.length]}  ${String(index * 73 + 19).padStart(4, '0')}`
        const segment = `${rowText}   `
        const tile = buildSignalTile(segment, context.measureText(segment).width, width)

        return {
          text: tile.text,
          segmentWidth: tile.segmentWidth,
          x: -(Math.random() * tile.segmentWidth),
          y: index * 20,
          speed: -(.62 * Math.random() + .48),
          alpha: .22 * Math.random() + .05,
        }
      })
    }

    const draw = () => {
      context.clearRect(0, 0, width, height)
      context.font = SIGNAL_FONT
      context.textBaseline = 'middle'
      velocity += (targetVelocity - velocity) * .055
      for (const row of rows) {
        row.x += row.speed * velocity
        if (row.x < -row.segmentWidth) row.x += row.segmentWidth
        context.fillStyle = `rgba(157, 174, 198, ${row.alpha})`
        context.fillText(row.text, row.x, row.y)
      }
    }

    const tick = () => {
      if (!visible || !active) {
        running = false
        return
      }
      draw()
      frame.current = window.requestAnimationFrame(tick)
    }

    const start = () => {
      if (running || !visible) return
      running = true
      frame.current = window.requestAnimationFrame(tick)
    }
    const onPointerMove = (event: PointerEvent) => {
      const rect = container.getBoundingClientRect()
      pointerX = event.clientX - rect.left
      const center = width / 2
      targetVelocity = .9 + 3.4 * Math.abs((pointerX - center) / center)
    }
    const onPointerLeave = () => {
      targetVelocity = 1.35
    }
    const observer = new IntersectionObserver(([entry]) => {
      visible = entry?.isIntersecting ?? true
      if (visible) start()
      else if (frame.current !== null) {
        window.cancelAnimationFrame(frame.current)
        frame.current = null
        running = false
      }
    })

    build()
    draw()
    setCanvasActive(true)
    observer.observe(container)
    container.addEventListener('pointermove', onPointerMove, { passive: true })
    container.addEventListener('pointerleave', onPointerLeave)
    window.addEventListener('resize', build)
    start()

    return () => {
      setCanvasActive(false)
      observer.disconnect()
      container.removeEventListener('pointermove', onPointerMove)
      container.removeEventListener('pointerleave', onPointerLeave)
      window.removeEventListener('resize', build)
      if (frame.current !== null) window.cancelAnimationFrame(frame.current)
    }
  }, [active, reduced])

  return (
    <div
      ref={host}
      className="hero-signal"
      role="img"
      aria-label="星桥实时信号背景"
      data-canvas-active={canvasActive ? 'true' : 'false'}
      data-travel-direction="left"
      data-signal-density="dense"
      data-signal-speed="fast"
    >
      <canvas ref={canvas} aria-hidden="true" />
      <span aria-hidden="true">XQ / OPENAI / ANTHROPIC / SEOUL DIRECT / API READY</span>
    </div>
  )
}
