import { Radio } from 'lucide-react'
import { motion, useScroll, useTransform } from 'motion/react'
import { useRef } from 'react'
import { useReducedMotionPreference } from '../hooks/useReducedMotion'

const models = ['OpenAI', 'Claude', 'Gemini', 'DeepSeek', 'Qwen', 'GLM']

export function ModelFlow() {
  const root = useRef<HTMLDivElement>(null)
  const reduced = useReducedMotionPreference()
  const { scrollYProgress } = useScroll({ target: root, offset: ['start .92', 'start .45'] })
  const fill = useTransform(scrollYProgress, [0, 1], [0, 1])

  return <div ref={root} className="model-flow" aria-label="模型路由示意" data-motion-state={reduced ? 'final' : 'active'}>
    <span className="request-chip"><Radio aria-hidden="true" />你的请求</span>
    <span className="flow-line" aria-hidden="true"><motion.i style={reduced ? { scaleX: 1 } : { scaleX: fill }} /><b /></span>
    <span className="gateway-chip"><img src="/home-assets/xingqiao-logo.png" alt="" />星桥</span>
    <span className="flow-line" aria-hidden="true"><motion.i style={reduced ? { scaleX: 1 } : { scaleX: fill }} /><b /></span>
    <div className="model-chips">{models.map((model, index) => <span key={model} style={{ '--flow-stagger': `${index * .45}s` } as React.CSSProperties}>{model}</span>)}</div>
  </div>
}
