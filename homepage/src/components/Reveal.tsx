import { createElement, useEffect, useRef, useState, type ComponentPropsWithoutRef, type ElementType, type ReactNode } from 'react'
import { useReducedMotionPreference } from '../hooks/useReducedMotion'

interface RevealProps<T extends ElementType> {
  as?: T
  children: ReactNode
  className?: string
}

export function Reveal<T extends ElementType = 'div'>({
  as,
  children,
  className = '',
  ...props
}: RevealProps<T> & Omit<ComponentPropsWithoutRef<T>, keyof RevealProps<T>>) {
  const Component = as ?? 'div'
  const node = useRef<HTMLElement | null>(null)
  const reduced = useReducedMotionPreference()
  const [visible, setVisible] = useState(reduced)

  useEffect(() => {
    if (reduced || typeof IntersectionObserver === 'undefined') {
      setVisible(true)
      return
    }
    const element = node.current
    if (!element) return
    const observer = new IntersectionObserver(([entry]) => {
      if (entry?.isIntersecting) {
        setVisible(true)
        observer.disconnect()
      }
    }, { threshold: .12 })
    observer.observe(element)
    return () => observer.disconnect()
  }, [reduced])

  return createElement(Component, {
    ...props,
    ref: node,
    className: `reveal ${visible ? 'is-visible' : ''} ${className}`.trim(),
  }, children)
}
