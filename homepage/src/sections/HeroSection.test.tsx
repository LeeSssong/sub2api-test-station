import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
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
})
