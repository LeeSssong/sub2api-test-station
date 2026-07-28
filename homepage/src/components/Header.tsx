import { Menu, Moon, Sun, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import type { SessionState } from '../domain/session'
import type { Theme } from '../domain/themeSchedule'

interface HeaderProps {
  session: SessionState
  theme: Theme
  onToggleTheme: () => void
}

export function Header({ session, theme, onToggleTheme }: HeaderProps) {
  const [open, setOpen] = useState(false)
  const [scrolled, setScrolled] = useState(false)
  const dashboardHref = session.kind === 'admin' ? '/admin/dashboard' : '/dashboard'
  const links = [
    { label: '首页', href: '/' },
    { label: '控制台', href: dashboardHref },
    { label: '模型', href: '/admin/channels/pricing' },
    { label: '状态', href: '/monitor' },
    { label: '文档', href: '/docs/' },
  ]
  const themeActionLabel = theme === 'dark' ? '切换到白天模式' : '切换到黑夜模式'

  useEffect(() => {
    const update = () => setScrolled(window.scrollY > 24)
    update()
    window.addEventListener('scroll', update, { passive: true })
    return () => window.removeEventListener('scroll', update)
  }, [])

  return (
    <header
      className={`site-header${open ? ' site-header--open' : ''}`}
      data-scrolled={scrolled ? 'true' : 'false'}
    >
      <nav className="nav-shell" aria-label="主导航">
        <a className="brand-link" href="/" aria-label="星桥首页">
          <img src="/home-assets/xingqiao-logo.png" alt="" width="34" height="34" />
          <span>星桥</span>
        </a>
        <div className="desktop-nav">
          {links.map((link) => (
            <a key={link.label} href={link.href} aria-current={link.href === '/' ? 'page' : undefined}>
              {link.label}
            </a>
          ))}
        </div>
        <div className="nav-actions">
          <button
            className="icon-button theme-indicator"
            type="button"
            aria-label={themeActionLabel}
            title={themeActionLabel}
            onClick={onToggleTheme}
          >
            {theme === 'dark' ? <Sun aria-hidden="true" /> : <Moon aria-hidden="true" />}
          </button>
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
        </nav>
      )}
    </header>
  )
}
