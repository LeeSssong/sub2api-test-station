import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { HeroSection } from './HeroSection'
import type { SessionState } from '../domain/session'

describe('HeroSection', () => {
  it('shows the exact direct-connect promise and both compatible paths', () => {
    const session: SessionState = {
      kind: 'guest',
      ctaLabel: '立即获取密钥',
      ctaHref: '/register',
    }

    render(<HeroSection apiOrigin="https://api.example.com" session={session} />)

    expect(screen.getByText('https://api.example.com')).toBeInTheDocument()
    expect(screen.getByText('/v1/chat/completions')).toBeInTheDocument()
    expect(screen.getByText('/v1/messages')).toBeInTheDocument()
    expect(screen.getByText('向下探索')).toBeInTheDocument()
  })

  it('uses semantic static fallbacks when reduced motion is requested', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))

    render(<HeroSection apiOrigin="https://api.example.com" session={{ kind: 'guest', ctaLabel: '立即获取密钥', ctaHref: '/register' }} />)

    expect(screen.getByLabelText('星桥首页首屏')).toHaveAttribute('data-entry-state', 'final')
    expect(screen.getByLabelText('星桥实时信号背景')).toHaveAttribute('data-canvas-active', 'false')
    expect(screen.getByLabelText('星桥实时信号背景')).toHaveAttribute('data-travel-direction', 'left')
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})
