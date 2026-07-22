import { Check, Copy } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { copyText } from '../domain/clipboard'

interface CopyControlProps {
  value: string
  label: string
  compact?: boolean
}

export function CopyControl({ value, label, compact = false }: CopyControlProps) {
  const [copied, setCopied] = useState(false)
  const timer = useRef<number | null>(null)

  useEffect(() => () => {
    if (timer.current !== null) window.clearTimeout(timer.current)
  }, [])

  async function handleCopy() {
    await copyText(value, navigator.clipboard, document)
    setCopied(true)
    if (timer.current !== null) window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setCopied(false), 1800)
  }

  return (
    <button
      className={`copy-control${compact ? ' copy-control--compact' : ''}`}
      type="button"
      aria-label={label}
      onClick={handleCopy}
      title={label}
    >
      {copied ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}
      <span aria-live="polite">{copied ? '已复制' : '复制'}</span>
    </button>
  )
}
