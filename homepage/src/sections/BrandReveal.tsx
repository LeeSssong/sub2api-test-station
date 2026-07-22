import { useEffect, useRef, useState } from 'react'
import { useReducedMotionPreference } from '../hooks/useReducedMotion'

interface Cell {
  x: number
  y: number
  ox: number
  oy: number
  color: string
}

export function BrandReveal() {
  const root = useRef<HTMLElement>(null)
  const canvas = useRef<HTMLCanvasElement>(null)
  const frame = useRef<number | null>(null)
  const cells = useRef<Cell[]>([])
  const visible = useRef(true)
  const [active, setActive] = useState(false)
  const reduced = useReducedMotionPreference()

  useEffect(() => {
    if (reduced || !canvas.current || !root.current) return
    const element = canvas.current
    const section = root.current
    const context = element.getContext('2d')
    if (!context) return
    setActive(true)

    const build = () => {
      const rect = section.getBoundingClientRect()
      const width = Math.max(1, Math.round(rect.width))
      const height = Math.max(1, Math.round(rect.height))
      const dpr = Math.min(window.devicePixelRatio || 1, 2)
      element.width = width * dpr
      element.height = height * dpr
      element.style.width = `${width}px`
      element.style.height = `${height}px`
      context.setTransform(dpr, 0, 0, dpr, 0, 0)

      const source = document.createElement('canvas')
      source.width = width
      source.height = height
      const sourceContext = source.getContext('2d')
      if (!sourceContext) return
      const fontSize = Math.min(width * .31, height * .72)
      sourceContext.fillStyle = '#dce4fa'
      sourceContext.font = `750 ${fontSize}px "PingFang SC", "Microsoft YaHei", sans-serif`
      sourceContext.textAlign = 'center'
      sourceContext.textBaseline = 'middle'
      sourceContext.fillText('星桥', width / 2, height * .62)
      const pixels = sourceContext.getImageData(0, 0, width, height).data
      const size = Math.max(14, Math.min(30, Math.round(width / 60)))
      const next: Cell[] = []
      for (let y = 0; y < height; y += size) {
        for (let x = 0; x < width; x += size) {
          const sampleX = Math.min(width - 1, x + Math.floor(size / 2))
          const sampleY = Math.min(height - 1, y + Math.floor(size / 2))
          if (pixels[(sampleY * width + sampleX) * 4 + 3] > 32) {
            next.push({ x, y, ox: 0, oy: 0, color: '#dce4fa' })
          }
        }
      }
      cells.current = next
      draw()
    }

    const draw = () => {
      const rect = section.getBoundingClientRect()
      const size = Math.max(14, Math.min(30, Math.round(rect.width / 60)))
      context.clearRect(0, 0, rect.width, rect.height)
      for (const cell of cells.current) {
        context.fillStyle = cell.color
        context.fillRect(cell.x + cell.ox, cell.y + cell.oy, Math.max(2, size - 2), Math.max(2, size - 2))
      }
    }

    const animate = () => {
      let moving = false
      for (const cell of cells.current) {
        cell.ox *= .88
        cell.oy *= .88
        if (Math.abs(cell.ox) < .04) cell.ox = 0
        if (Math.abs(cell.oy) < .04) cell.oy = 0
        if (cell.ox || cell.oy) moving = true
      }
      draw()
      frame.current = moving && visible.current ? requestAnimationFrame(animate) : null
    }

    const disturb = (event: PointerEvent) => {
      const rect = element.getBoundingClientRect()
      const x = event.clientX - rect.left
      const y = event.clientY - rect.top
      const radius = Math.min(180, rect.width * .18)
      for (const cell of cells.current) {
        const dx = cell.x - x
        const dy = cell.y - y
        const distance = Math.hypot(dx, dy)
        if (distance >= radius || distance === 0) continue
        const force = (1 - distance / radius) * 42
        cell.ox += (dx / distance) * force
        cell.oy += (dy / distance) * force
      }
      if (frame.current === null) frame.current = requestAnimationFrame(animate)
    }

    const observer = new IntersectionObserver(([entry]) => {
      visible.current = entry?.isIntersecting ?? true
      if (visible.current && frame.current === null && cells.current.some((cell) => cell.ox || cell.oy)) {
        frame.current = requestAnimationFrame(animate)
      }
    })
    observer.observe(section)
    element.addEventListener('pointermove', disturb, { passive: true })
    window.addEventListener('resize', build)
    build()

    return () => {
      setActive(false)
      observer.disconnect()
      element.removeEventListener('pointermove', disturb)
      window.removeEventListener('resize', build)
      if (frame.current !== null) cancelAnimationFrame(frame.current)
    }
  }, [reduced])

  return (
    <section
      ref={root}
      className="brand-reveal"
      aria-label="星桥品牌揭幕"
      data-canvas-active={active ? 'true' : 'false'}
    >
      <canvas ref={canvas} aria-hidden="true" />
      <h2 className="brand-static">星桥</h2>
    </section>
  )
}
