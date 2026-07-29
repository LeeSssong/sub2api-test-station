import { Radio } from 'lucide-react'
import { motion, useScroll, useTransform } from 'motion/react'
import { useRef } from 'react'
import { useReducedMotionPreference } from '../hooks/useReducedMotion'

const models = ['OpenAI', 'Claude', 'Gemini', 'DeepSeek', 'Qwen', 'GLM']

export function ModelFlow() {
  const root = useRef<HTMLDivElement>(null)
  const reduced = useReducedMotionPreference()
  const { scrollYProgress } = useScroll({ target: root, offset: ['start .92', 'start .45'] })
  const requestFill = useTransform(scrollYProgress, [0, .42], [0, 1])
  const modelFill = useTransform(scrollYProgress, [.58, 1], [0, 1])
  const requestGlow = useTransform(scrollYProgress, [0, .42, .52], [1, 1, 0])
  const modelGlow = useTransform(scrollYProgress, [0, .57, .58, 1], [0, 0, 1, 1])

  return <div ref={root} className="model-flow" aria-label="模型路由示意" data-motion-state={reduced ? 'final' : 'active'}>
    <span className="request-chip"><Radio aria-hidden="true" />你的请求</span>
    <span className="flow-line" data-flow-segment="request-to-gateway" data-scroll-range="0-0.42" aria-hidden="true">
      <motion.i style={reduced ? { scaleX: 1 } : { scaleX: requestFill }} />
      <motion.b style={reduced ? { opacity: 0 } : { opacity: requestGlow }} />
    </span>
    <span className="gateway-chip"><img src="/home-assets/xingqiao-logo-256-v1.webp" alt="" />星桥</span>
    <span className="flow-line" data-flow-segment="gateway-to-models" data-scroll-range="0.58-1" aria-hidden="true">
      <motion.i style={reduced ? { scaleX: 1 } : { scaleX: modelFill }} />
      <motion.b style={reduced ? { opacity: 0 } : { opacity: modelGlow }} />
    </span>
    <div className="model-chips">{models.map((model, index) => <span key={model} style={{ '--flow-stagger': `${index * .45}s` } as React.CSSProperties}>{model}</span>)}</div>
  </div>
}
