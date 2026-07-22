import { Menu, Moon, X } from 'lucide-react'
import { useState } from 'react'
import type { SessionState } from '../domain/session'

interface HeaderProps {
  session: SessionState
}

export function Header({ session }: HeaderProps) {
  const [open, setOpen] = useState(false)
  const dashboardHref = session.kind === 'admin' ? '/admin/dashboard' : '/dashboard'
  const links = [
    { label: '首页', href: '/' },
    { label: '控制台', href: dashboardHref },
    { label: '模型', href: '/pricing' },
    { label: '状态', href: '/monitor' },
    { label: '文档', href: '#docs' },
    { label: '关于', href: '#about' },
  ]

  return (
    <header className={`site-header${open ? ' site-header--open' : ''}`}>
      <nav className="nav-shell" aria-label="主导航">
        <a className="brand-link" href="/" aria-label="星桥首页">
          <img src="/home-assets/xingqiao-logo.png" alt="" width="34" height="34" />
          <span>星桥</span>
        </a>
        <div className="desktop-nav">
          {links.map((link) => <a key={link.label} href={link.href}>{link.label}</a>)}
        </div>
        <div className="nav-actions">
          <button className="icon-button theme-indicator" type="button" aria-label="深色主题" title="深色主题">
            <Moon aria-hidden="true" />
          </button>
          <a className="login-link" href={session.kind === 'guest' ? '/login' : dashboardHref}>
            {session.kind === 'guest' ? '登录' : '控制台'}
          </a>
          <button
            className="icon-button menu-button"
            type="button"
            aria-label={open ? '关闭导航菜单' : '打开导航菜单'}
            aria-expanded={open}
            onClick={() => setOpen((value) => !value)}
          >
            {open ? <X aria-hidden="true" /> : <Menu aria-hidden="true" />}
          </button>
        </div>
      </nav>
      {open && (
        <nav className="mobile-nav" aria-label="移动导航">
          {links.map((link) => (
            <a key={link.label} href={link.href} onClick={() => setOpen(false)}>{link.label}</a>
          ))}
          <a className="mobile-login" href={session.kind === 'guest' ? '/login' : dashboardHref}>
            {session.kind === 'guest' ? '登录' : '控制台'}
          </a>
        </nav>
      )}
    </header>
  )
}
