import { createElement, useEffect, useRef, useState, type ComponentPropsWithoutRef, type ElementType, type ReactNode } from 'react'
import { useReducedMotionPreference } from '../hooks/useReducedMotion'

interface RevealProps<T extends ElementType> {
  as?: T
  children: ReactNode
  className?: string
  animation?: 'fade-up' | 'fade-in' | 'scale-in' | 'mask-rise' | 'rule-line'
  delay?: number
  threshold?: number
  once?: boolean
}

export function Reveal<T extends ElementType = 'div'>({
  as,
  children,
  className = '',
  animation = 'fade-up',
  delay = 0,
  threshold = .15,
  once = true,
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
        if (once) observer.disconnect()
      } else if (!once) {
        setVisible(false)
      }
    }, { threshold, rootMargin: '0px 0px -40px 0px' })
    observer.observe(element)
    return () => observer.disconnect()
  }, [reduced])

  return createElement(Component, {
    ...props,
    ref: node,
    className: `reveal reveal--${animation} ${visible ? 'is-visible' : ''} ${className}`.trim(),
    style: { animationDelay: delay ? `${delay}ms` : undefined },
  }, children)
}
