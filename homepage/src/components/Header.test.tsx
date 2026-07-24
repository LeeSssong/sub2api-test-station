import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'
import { Header } from './Header'

describe('Header', () => {
  it('keeps authentication actions in the hero instead of duplicating them in the header', async () => {
    const user = userEvent.setup()
    render(<Header session={{ kind: 'guest', ctaLabel: '立即开始', ctaHref: '/dashboard' }} />)

    const navigation = screen.getByRole('navigation', { name: '主导航' })
    expect(within(navigation).queryByRole('link', { name: '登录' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '打开导航菜单' }))
    const mobileNavigation = screen.getByRole('navigation', { name: '移动导航' })
    expect(within(mobileNavigation).queryByRole('link', { name: '登录' })).not.toBeInTheDocument()
  })

  it('routes desktop and mobile documentation commands to the local guide', async () => {
    const user = userEvent.setup()
    render(<Header session={{ kind: 'guest', ctaLabel: '立即开始', ctaHref: '/dashboard' }} />)

    expect(screen.getByRole('link', { name: '文档' })).toHaveAttribute('href', '/docs/')

    await user.click(screen.getByRole('button', { name: '打开导航菜单' }))
    expect(screen.getAllByRole('link', { name: '文档' })[1]).toHaveAttribute('href', '/docs/')
  })
})

afterEach(cleanup)
