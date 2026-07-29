import { useEffect, useRef, useState } from 'react'
import {
  buildSignalRows,
  signalPalette,
  type SignalLayer,
  type SignalRowDescriptor,
} from '../domain/signalField'
import { buildSignalTile } from '../domain/signalTiling'
import type { Theme } from '../domain/themeSchedule'
import { useReducedMotionPreference } from '../hooks/useReducedMotion'
import { HOMEPAGE_THEME_EVENT } from '../themeBootstrap'

const SIGNAL_FONT_FAMILY = 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace'

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

interface SignalRow extends SignalRowDescriptor {
  text: string
  segmentWidth: number
  x: number
}

interface HeroSignalCanvasProps {
  active: boolean
  /** Omit to render the field as decorative (aria-hidden) rather than a labelled image. */
  label?: string
  /** Faintest row opacity. */
  alphaBase?: number
  /** Additional random opacity on top of the base. */
  alphaRange?: number
}

export function HeroSignalCanvas({
  active,
  label,
  alphaBase = .09,
  alphaRange = .3,
}: HeroSignalCanvasProps) {
  const canvas = useRef<HTMLCanvasElement>(null)
  const host = useRef<HTMLDivElement>(null)
  const frame = useRef<number | null>(null)
  const reduced = useReducedMotionPreference()
  const [canvasActive, setCanvasActive] = useState(false)

  useEffect(() => {
    const element = canvas.current
    const container = host.current
    if (!active || !element || !container) return
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
    let currentTheme: Theme = document.documentElement.dataset.theme === 'light' ? 'light' : 'dark'

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
      const layerRank: Record<SignalLayer, number> = { far: 0, mid: 1, near: 2 }
      rows = buildSignalRows({ width, height })
        .sort((left, right) => layerRank[left.layer] - layerRank[right.layer])
        .map((descriptor) => {
        const index = descriptor.seedIndex
        const rowText = `${rowSeeds[index % rowSeeds.length]}  ${String(index * 73 + 19).padStart(4, '0')}`
        const segment = `${rowText}   `
        context.font = `500 ${descriptor.fontSize}px ${SIGNAL_FONT_FAMILY}`
        const tile = buildSignalTile(segment, context.measureText(segment).width, width)

        return {
          ...descriptor,
          text: tile.text,
          segmentWidth: tile.segmentWidth,
          x: -(Math.random() * tile.segmentWidth),
        }
      })
    }

    const draw = (time = 0, advance = false) => {
      context.clearRect(0, 0, width, height)
      context.textBaseline = 'middle'
      const palette = signalPalette(currentTheme)
      const pointerDistance = width > 1 ? Math.min(1, Math.abs(pointerX - width / 2) / (width / 2)) : 1
      const pointerLift = 1 + (1 - pointerDistance) * .12
      if (advance) velocity += (targetVelocity - velocity) * .055

      for (const row of rows) {
        if (advance) {
          row.x += row.speed * velocity
          if (row.x < -row.segmentWidth) row.x += row.segmentWidth
        }
        const pulse = row.active ? .5 + Math.sin(time / 920 + row.seedIndex) * .5 : 0
        context.font = `500 ${row.fontSize}px ${SIGNAL_FONT_FAMILY}`
        context.fillStyle = row.active ? palette.active : palette[row.layer]
        context.globalAlpha = Math.min(
          .82,
          (alphaBase + row.alpha * alphaRange + pulse * .18) * pointerLift,
        )
        context.shadowColor = row.active ? palette.active : 'transparent'
        context.shadowBlur = row.active ? 7 + pulse * 7 : 0
        context.fillText(row.text, row.x, row.y)
      }
      context.globalAlpha = 1
      context.shadowBlur = 0
    }

    const tick = (time: number) => {
      if (!visible || !active) {
        running = false
        return
      }
      draw(time, true)
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
      pointerX = 0
      targetVelocity = 1.35
    }
    const onThemeChange = (event: Event) => {
      const nextTheme = (event as CustomEvent<{ theme?: Theme }>).detail?.theme
      currentTheme = nextTheme === 'light' ? 'light' : 'dark'
      draw(performance.now(), false)
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
    draw(performance.now(), false)
    window.addEventListener('resize', build)
    window.addEventListener(HOMEPAGE_THEME_EVENT, onThemeChange)

    if (reduced) {
      return () => {
        window.removeEventListener('resize', build)
        window.removeEventListener(HOMEPAGE_THEME_EVENT, onThemeChange)
      }
    }

    setCanvasActive(true)
    observer.observe(container)
    container.addEventListener('pointermove', onPointerMove, { passive: true })
    container.addEventListener('pointerleave', onPointerLeave)
    start()

    return () => {
      setCanvasActive(false)
      observer.disconnect()
      container.removeEventListener('pointermove', onPointerMove)
      container.removeEventListener('pointerleave', onPointerLeave)
      window.removeEventListener('resize', build)
      window.removeEventListener(HOMEPAGE_THEME_EVENT, onThemeChange)
      if (frame.current !== null) window.cancelAnimationFrame(frame.current)
    }
  }, [active, reduced, alphaBase, alphaRange])

  return (
    <div
      ref={host}
      className="hero-signal"
      role={label ? 'img' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      data-canvas-active={canvasActive ? 'true' : 'false'}
      data-travel-direction="left"
      data-signal-density="dense"
      data-signal-speed="fast"
      data-signal-layers="3"
    >
      <canvas ref={canvas} aria-hidden="true" />
      <span aria-hidden="true">XQ / OPENAI / ANTHROPIC / SEOUL DIRECT / API READY</span>
    </div>
  )
}
