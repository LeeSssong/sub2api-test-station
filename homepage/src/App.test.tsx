import { cleanup, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { App } from './App'
import { DEFAULT_SITE_CONFIG, type ThirdPartyReport } from './domain/siteConfig'
import type { SessionState } from './domain/session'

const guest: SessionState = {
  kind: 'guest',
  ctaLabel: '立即开始',
  ctaHref: '/dashboard',
}
const themeProps = {
  theme: 'dark' as const,
  onToggleTheme: () => {},
}

const validReport: ThirdPartyReport = {
  id: 'modeloc-1',
  provider: 'MODELOC',
  title: '模型真实性报告',
  url: 'https://modeloc.com/r/xingqiao',
  status: 'verified',
}

describe('App', () => {
  it('renders the complete Xingqiao guest homepage contract', () => {
    render(<App
      config={{ ...DEFAULT_SITE_CONFIG, apiOrigin: 'https://api.example.com' }}
      session={guest}
      {...themeProps}
    />)

    const navigation = screen.getByRole('navigation', { name: '主导航' })
    for (const label of ['首页', '控制台', '模型', '状态', '文档']) {
      expect(within(navigation).getByText(label)).toBeInTheDocument()
    }
    expect(within(navigation).queryByText('关于')).not.toBeInTheDocument()
    expect(within(navigation).getByRole('link', { name: '模型' })).toHaveAttribute(
      'href',
      '/admin/channels/pricing',
    )
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('星桥链接世界顶尖模型')
    expect(screen.getByText('首尔节点 · 稳定运行')).toBeInTheDocument()
    expect(screen.getByText('GPT、Claude、Gemini 一站接入。')).toBeInTheDocument()
    expect(screen.getByText('国内网络直接连接，注册即可使用。')).toBeInTheDocument()
    expect(screen.getByText('OPENAI_BASE_URL=https://api.example.com')).toBeVisible()
    expect(screen.getByText('ANTHROPIC_BASE_URL=https://api.example.com')).toBeVisible()
    expect(screen.getByRole('link', { name: '立即开始' })).toHaveAttribute('href', '/dashboard')
    expect(screen.queryByRole('link', { name: '登录' })).not.toBeInTheDocument()
    expect(within(navigation).queryByRole('link', { name: '登录' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '复制 API 地址' })).not.toBeInTheDocument()
    expect(screen.queryByText('1080152144')).not.toBeInTheDocument()
    expect(screen.queryByText('TG群组')).not.toBeInTheDocument()
    expect(screen.queryByText('站内工单')).not.toBeInTheDocument()
    expect(screen.getByText('已获得 MODELOC 真实性验证')).toBeInTheDocument()
    expect(screen.queryByText('待公开')).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: '边界清晰，承诺才有意义' })).not.toBeInTheDocument()
  })

  it('routes signed-in roles to their matching dashboard', () => {
    const user: SessionState = {
      kind: 'user',
      ctaLabel: '进入控制台',
      ctaHref: '/dashboard',
      user: { id: 2, role: 'user' },
    }
    const admin: SessionState = {
      kind: 'admin',
      ctaLabel: '进入控制台',
      ctaHref: '/admin/dashboard',
      user: { id: 1, role: 'admin' },
    }
    const { rerender } = render(<App config={DEFAULT_SITE_CONFIG} session={user} {...themeProps} />)
    expect(screen.getByRole('link', { name: '进入控制台' })).toHaveAttribute('href', '/dashboard')

    rerender(<App config={DEFAULT_SITE_CONFIG} session={admin} {...themeProps} />)
    expect(screen.getByRole('link', { name: '进入控制台' })).toHaveAttribute('href', '/admin/dashboard')
  })

  it('does not link away from the homepage when a report URL is configured', () => {
    render(<App
      config={{ ...DEFAULT_SITE_CONFIG, thirdPartyReports: [validReport] }}
      session={guest}
      {...themeProps}
    />)
    expect(screen.queryByText('待公开')).not.toBeInTheDocument()
    expect(screen.getByText('已获得 MODELOC 真实性验证')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /MODELOC|真实性/ })).not.toBeInTheDocument()
  })
})

afterEach(cleanup)
