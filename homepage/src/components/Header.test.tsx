import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Header } from './Header'

const guest = { kind: 'guest', ctaLabel: '立即开始', ctaHref: '/dashboard' } as const

describe('Header', () => {
  it('keeps authentication actions in the hero instead of duplicating them in the header', async () => {
    const user = userEvent.setup()
    render(<Header session={guest} theme="dark" onToggleTheme={() => {}} />)

    const navigation = screen.getByRole('navigation', { name: '主导航' })
    expect(within(navigation).queryByRole('link', { name: '登录' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '打开导航菜单' }))
    const mobileNavigation = screen.getByRole('navigation', { name: '移动导航' })
    expect(within(mobileNavigation).queryByRole('link', { name: '登录' })).not.toBeInTheDocument()
  })

  it('routes desktop and mobile documentation commands to the local guide', async () => {
    const user = userEvent.setup()
    render(<Header session={guest} theme="dark" onToggleTheme={() => {}} />)

    expect(screen.getByRole('link', { name: '文档' })).toHaveAttribute('href', '/docs/')

    await user.click(screen.getByRole('button', { name: '打开导航菜单' }))
    expect(screen.getAllByRole('link', { name: '文档' })[1]).toHaveAttribute('href', '/docs/')
  })

  it('exposes the target theme and invokes the toggle action', async () => {
    const user = userEvent.setup()
    const onToggleTheme = vi.fn()
    const { rerender } = render(
      <Header session={guest} theme="dark" onToggleTheme={onToggleTheme} />,
    )

    const lightButton = screen.getByRole('button', { name: '切换到白天模式' })
    expect(lightButton.querySelector('.lucide-sun')).toBeInTheDocument()
    await user.click(lightButton)
    expect(onToggleTheme).toHaveBeenCalledOnce()

    rerender(<Header session={guest} theme="light" onToggleTheme={onToggleTheme} />)
    const darkButton = screen.getByRole('button', { name: '切换到黑夜模式' })
    expect(darkButton.querySelector('.lucide-moon')).toBeInTheDocument()
  })
})

afterEach(cleanup)
