import { useEffect, useRef, useState } from 'react'
import { useReducedMotionPreference } from '../hooks/useReducedMotion'

const rowSeeds = [
  'XQ ROUTE 09 CORE TOKEN BRIDGE OPENAI ANTHROPIC RESPONSE 200 STREAM',
  'SEOUL DIRECT CN MODEL LINK GLOBAL LATENCY HEALTHY UPSTREAM',
  'V1 CHAT COMPLETIONS MESSAGES EMBEDDINGS TOOLS MODELS',
  'REQUEST TRACE STAR BRIDGE GATEWAY TOKEN COST STATUS READY',
]

interface SignalRow {
  text: string
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
    let pointerX = -1000
    let pointerY = -1000

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
      const count = Math.max(12, Math.ceil(height / 34) + 4)
      rows = Array.from({ length: count }, (_, index) => ({
        text: `${rowSeeds[index % rowSeeds.length]}  ${String(index * 73 + 19).padStart(4, '0')}`,
        x: -40 - ((index * 83) % 260),
        y: index * 34 - 56,
        speed: .18 + (index % 5) * .055,
        alpha: .075 + (index % 4) * .022,
      }))
    }

    const draw = () => {
      context.clearRect(0, 0, width, height)
      context.font = '600 13px ui-monospace, SFMono-Regular, Menlo, monospace'
      context.textBaseline = 'middle'
      for (const row of rows) {
        const distance = Math.hypot(pointerX - width * .5, pointerY - row.y)
        const influence = Math.max(0, 1 - distance / 320)
        row.x += row.speed * (1 + influence * 2.6)
        if (row.x > 40) row.x = -Math.max(360, context.measureText(row.text).width * .48)
        context.fillStyle = `rgba(157, 174, 198, ${row.alpha + influence * .16})`
        context.fillText(`${row.text}   ${row.text}`, row.x, row.y)
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
      pointerY = event.clientY - rect.top
    }
    const onPointerLeave = () => {
      pointerX = -1000
      pointerY = -1000
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
    >
      <canvas ref={canvas} aria-hidden="true" />
      <span aria-hidden="true">XQ / OPENAI / ANTHROPIC / SEOUL DIRECT / API READY</span>
    </div>
  )
}
