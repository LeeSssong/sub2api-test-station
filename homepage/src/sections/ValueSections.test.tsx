import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_SITE_CONFIG } from '../domain/siteConfig'
import { ValueSections } from './ValueSections'

describe('ValueSections', () => {
  it('renders the fixed public price and Korea direct-connect promises', () => {
    render(<ValueSections config={DEFAULT_SITE_CONFIG} />)

    expect(screen.getByText('官方价格 100%')).toBeInTheDocument()
    expect(screen.getByText('星桥价格 官方价格的 0.1–0.3 倍')).toBeInTheDocument()
    expect(screen.getByText('额度换算 1 元 = 1 美元额度')).toBeInTheDocument()
    expect(screen.getByText('韩国首尔服务器，国内无需翻墙即可直连')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '复制 QQ 群号' })).toBeInTheDocument()
  })

  it('keeps the route complete and static for reduced motion', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
    render(<ValueSections config={DEFAULT_SITE_CONFIG} />)

    expect(screen.getByLabelText('模型路由示意')).toHaveAttribute('data-motion-state', 'final')
    expect(screen.getByText('OpenAI')).toHaveStyle({ '--flow-stagger': '0s' })
    expect(screen.getByText('GLM')).toHaveStyle({ '--flow-stagger': '2.25s' })
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})
