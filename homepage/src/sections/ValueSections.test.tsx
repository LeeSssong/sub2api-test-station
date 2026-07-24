import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DEFAULT_SITE_CONFIG } from '../domain/siteConfig'
import { ValueSections } from './ValueSections'

describe('ValueSections', () => {
  it('renders discount pricing without the official-price baseline', () => {
    render(<ValueSections config={DEFAULT_SITE_CONFIG} />)

    expect(screen.getByText('星桥价格 官方价格的0.1——0.3折')).toBeInTheDocument()
    expect(screen.queryByText('官方价格 100%')).not.toBeInTheDocument()
    expect(screen.queryByText(/0.1–0.3 倍/)).not.toBeInTheDocument()
    expect(screen.queryByText(/倍率/)).not.toBeInTheDocument()
    expect(screen.getByText('额度换算 1 元 = 1 美元额度')).toBeInTheDocument()
  })

  it('states the MODELOC verification without creating a report link', () => {
    render(<ValueSections config={DEFAULT_SITE_CONFIG} />)

    expect(screen.getByRole('heading', { name: '安全与透明' })).toBeInTheDocument()
    expect(screen.getByText('HTTPS 加密传输')).toBeInTheDocument()
    expect(screen.getByText('无第三方追踪')).toBeInTheDocument()
    expect(screen.getByText('已获得 MODELOC 真实性验证')).toBeInTheDocument()
    expect(screen.queryByText('待公开')).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /MODELOC/ })).not.toBeInTheDocument()
  })

  it('uses the Xingqiao network promise and removes QQ support', () => {
    render(<ValueSections config={DEFAULT_SITE_CONFIG} />)

    expect(screen.getByText('韩国首尔服务器，国内无需翻墙即可直连')).toBeInTheDocument()
    expect(screen.getByText('星桥，缩短国内用户访问世界模型的网络路径。')).toBeInTheDocument()
    expect(screen.queryByText('QQ群支持')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '复制 QQ 群号' })).not.toBeInTheDocument()
  })

  it('keeps the route complete and static for reduced motion', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
    render(<ValueSections config={DEFAULT_SITE_CONFIG} />)

    expect(screen.getByLabelText('模型路由示意')).toHaveAttribute('data-motion-state', 'final')
    expect(screen.getByText('OpenAI')).toHaveStyle({ '--flow-stagger': '0s' })
    expect(screen.getByText('GLM')).toHaveStyle({ '--flow-stagger': '2.25s' })
  })

  it('routes through the gateway before drawing the model-side connection', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
    render(<ValueSections config={DEFAULT_SITE_CONFIG} />)

    const route = screen.getByLabelText('模型路由示意')
    const segments = route.querySelectorAll('.flow-line')

    expect(segments[0]).toHaveAttribute('data-flow-segment', 'request-to-gateway')
    expect(segments[0]).toHaveAttribute('data-scroll-range', '0-0.42')
    expect(segments[1]).toHaveAttribute('data-flow-segment', 'gateway-to-models')
    expect(segments[1]).toHaveAttribute('data-scroll-range', '0.58-1')
  })
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})
