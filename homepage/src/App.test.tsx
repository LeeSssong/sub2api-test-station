import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { App } from './App'
import { DEFAULT_SITE_CONFIG, type ThirdPartyReport } from './domain/siteConfig'
import type { SessionState } from './domain/session'

const guest: SessionState = {
  kind: 'guest',
  ctaLabel: '立即获取密钥',
  ctaHref: '/register',
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
    />)

    const navigation = screen.getByRole('navigation', { name: '主导航' })
    for (const label of ['首页', '控制台', '模型', '状态', '文档', '关于']) {
      expect(within(navigation).getByText(label)).toBeInTheDocument()
    }
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('星桥链接世界顶尖模型')
    expect(screen.getByText('韩国首尔节点，国内直连。')).toBeInTheDocument()
    expect(screen.getByText('兼容 OpenAI 与 Anthropic API。')).toBeInTheDocument()
    expect(screen.getByText('无需翻墙，只需更改基础 URL。')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '立即获取密钥' })).toHaveAttribute('href', '/register')
    expect(screen.getAllByText('/v1/chat/completions').length).toBeGreaterThan(0)
    expect(screen.getAllByText('/v1/messages').length).toBeGreaterThan(0)
    expect(screen.getAllByText('1080152144').length).toBeGreaterThan(0)
    expect(screen.queryByText('TG群组')).not.toBeInTheDocument()
    expect(screen.queryByText('站内工单')).not.toBeInTheDocument()
    expect(screen.queryByText('MODELOC')).not.toBeInTheDocument()
  })

  it('routes signed-in roles to their matching dashboard', () => {
    const user: SessionState = {
      kind: 'user',
      ctaLabel: '前往控制台',
      ctaHref: '/dashboard',
      user: { id: 2, role: 'user' },
    }
    const admin: SessionState = {
      kind: 'admin',
      ctaLabel: '前往控制台',
      ctaHref: '/admin/dashboard',
      user: { id: 1, role: 'admin' },
    }
    const { rerender } = render(<App config={DEFAULT_SITE_CONFIG} session={user} />)
    expect(screen.getByRole('link', { name: '前往控制台' })).toHaveAttribute('href', '/dashboard')

    rerender(<App config={DEFAULT_SITE_CONFIG} session={admin} />)
    expect(screen.getByRole('link', { name: '前往控制台' })).toHaveAttribute('href', '/admin/dashboard')
  })

  it('renders reports only after a valid report is configured', () => {
    const { rerender } = render(<App config={DEFAULT_SITE_CONFIG} session={guest} />)
    expect(screen.queryByText('MODELOC')).not.toBeInTheDocument()

    rerender(<App
      config={{ ...DEFAULT_SITE_CONFIG, thirdPartyReports: [validReport] }}
      session={guest}
    />)
    expect(screen.getByText('MODELOC')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /查看模型真实性报告/ })).toHaveAttribute(
      'href',
      validReport.url,
    )
  })
})
