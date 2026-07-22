import { motion, useScroll, useTransform, type MotionValue } from 'motion/react'
import { useRef } from 'react'
import { useReducedMotionPreference } from '../hooks/useReducedMotion'

const lines = [
  { text: '所有顶尖模型。', range: [0, .38] },
  { text: '一个网关。', range: [.28, .7] },
  { text: '国内直连。', range: [.58, 1] },
]

function StatementLine({
  text,
  range,
  progress,
  reduced,
}: {
  text: string
  range: number[]
  progress: MotionValue<number>
  reduced: boolean
}) {
  const inputRange = [range[0] ?? 0, range[1] ?? 1]
  const opacityValue = useTransform(progress, inputRange, [.15, 1])
  const yValue = useTransform(progress, inputRange, [18, 0])
  return <motion.span style={reduced ? { opacity: 1, y: 0 } : { opacity: opacityValue, y: yValue }}>{text}</motion.span>
}

export function StatementSection() {
  const section = useRef<HTMLElement>(null)
  const reduced = useReducedMotionPreference()
  const { scrollYProgress } = useScroll({ target: section, offset: ['start 0.9', 'start 0.35'] })

  return (
    <section
      ref={section}
      className="statement-section"
      aria-label="星桥服务声明"
      data-motion-state={reduced ? 'final' : 'scroll'}
    >
      <p>{lines.map((line) => <StatementLine key={line.text} {...line} progress={scrollYProgress} reduced={reduced} />)}</p>
    </section>
  )
}
