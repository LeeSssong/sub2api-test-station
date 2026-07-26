import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { HeroSection } from './HeroSection'
import type { SessionState } from '../domain/session'

describe('HeroSection', () => {
  it('gives guests one native dashboard entry in plain language', () => {
    const session: SessionState = {
      kind: 'guest',
      ctaLabel: '立即开始',
      ctaHref: '/dashboard',
    }

    render(<HeroSection session={session} />)

    expect(screen.getByText('GPT、Claude、Gemini 一站接入。')).toBeInTheDocument()
    expect(screen.getByText('国内网络直接连接，注册即可使用。')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '立即开始' })).toHaveAttribute('href', '/dashboard')
    expect(screen.getByRole('link', { name: '立即开始' }).querySelectorAll('svg')).toHaveLength(1)
    expect(screen.queryByRole('link', { name: '登录' })).not.toBeInTheDocument()
    const heroGrid = screen.getByLabelText('星桥首页首屏').querySelector('.hero-grid')
    expect(heroGrid).toHaveAttribute('data-layout', 'diagonal')
    expect(heroGrid).toHaveAttribute('data-composition', 'raised-diagonal')
    expect(screen.queryByText(/基础 URL/)).not.toBeInTheDocument()
    expect(screen.queryByText('https://api.example.com')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '复制 API 地址' })).not.toBeInTheDocument()
    expect(screen.queryByText('/v1/chat/completions')).not.toBeInTheDocument()
    expect(screen.queryByText('/v1/messages')).not.toBeInTheDocument()
    expect(screen.getByText('向下探索')).toBeInTheDocument()
  })

  it('routes signed-in users to the local documentation guide', () => {
    render(<HeroSection session={{ kind: 'admin', ctaLabel: '进入控制台', ctaHref: '/admin/dashboard', user: { id: 1, role: 'admin' } }} />)

    expect(screen.getByRole('link', { name: '查看文档' })).toHaveAttribute('href', '/docs/')
  })

  it('keeps the signal field dense and fast without adding a heavy hero image', () => {
    render(<HeroSection session={{ kind: 'guest', ctaLabel: '立即开始', ctaHref: '/dashboard' }} />)

    expect(screen.queryByRole('img', { name: '首尔边缘节点示意' })).not.toBeInTheDocument()
    expect(screen.getByLabelText('星桥实时信号背景')).toHaveAttribute('data-signal-density', 'dense')
    expect(screen.getByLabelText('星桥实时信号背景')).toHaveAttribute('data-signal-speed', 'fast')
  })

  it('separates the two headline rows for cross-platform font metrics', () => {
    render(<HeroSection session={{ kind: 'guest', ctaLabel: '立即开始', ctaHref: '/dashboard' }} />)

    const heading = screen.getByRole('heading', { name: '星桥链接世界顶尖模型' })
    expect(heading).toHaveClass('hero-title')
    expect(screen.getByText('星桥')).toHaveClass('hero-brand')
    expect(screen.getByText('链接世界顶尖模型')).toHaveClass('hero-tagline')
  })

  it('keeps a pronounced desktop diagonal and resets the lift on smaller screens', () => {
    const styles = readFileSync(resolve(process.cwd(), 'src/styles.css'), 'utf8')

    expect(styles).toContain('padding-bottom: clamp(12rem, 28vh, 18rem)')
    expect(styles).toMatch(/@media \(max-width: 980px\)[\s\S]*?\.hero-title \{ padding-bottom: 0; \}/)
  })

  it('uses semantic static fallbacks when reduced motion is requested', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))

    render(<HeroSection session={{ kind: 'guest', ctaLabel: '立即开始', ctaHref: '/dashboard' }} />)

    expect(screen.getByLabelText('星桥首页首屏')).toHaveAttribute('data-entry-state', 'final')
    expect(screen.getByLabelText('星桥实时信号背景')).toHaveAttribute('data-canvas-active', 'false')
    expect(screen.getByLabelText('星桥实时信号背景')).toHaveAttribute('data-travel-direction', 'left')
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})
