import { motion, useScroll, useTransform, type MotionValue } from 'motion/react'
import { useRef } from 'react'
import { HeroSignalCanvas } from '../components/HeroSignalCanvas'
import { useReducedMotionPreference } from '../hooks/useReducedMotion'

const lines = [
  { text: '所有顶尖模型。', range: [0, .4] },
  { text: '一个网关。', range: [.24, .68] },
  { text: '国内直连。', range: [.5, .96] },
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
  const opacityValue = useTransform(progress, inputRange, [.08, 1])
  const yValue = useTransform(progress, inputRange, [34, 0])
  const blurValue = useTransform(progress, inputRange, ['blur(12px)', 'blur(0px)'])
  return (
    <motion.span
      style={reduced ? { opacity: 1, y: 0 } : { opacity: opacityValue, y: yValue, filter: blurValue }}
    >
      {text}
    </motion.span>
  )
}

export function StatementSection() {
  const section = useRef<HTMLElement>(null)
  const reduced = useReducedMotionPreference()
  const { scrollYProgress } = useScroll({ target: section, offset: ['start 0.95', 'start 0.18'] })

  return (
    <section
      ref={section}
      className="statement-section"
      aria-label="星桥服务声明"
      data-motion-state={reduced ? 'final' : 'scroll'}
    >
      <div className="statement-signal">
        <HeroSignalCanvas active alphaBase={.05} alphaRange={.14} />
      </div>
      <p>{lines.map((line) => <StatementLine key={line.text} {...line} progress={scrollYProgress} reduced={reduced} />)}</p>
    </section>
  )
}
