import type { ComponentPropsWithoutRef, ElementType, ReactNode } from 'react'

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
  return <Component className={`reveal ${className}`.trim()} {...props}>{children}</Component>
}
