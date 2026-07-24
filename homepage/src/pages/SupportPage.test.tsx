import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SupportPage } from './SupportPage'

describe('SupportPage', () => {
  it('shows the preserved QQ group and QR code', () => {
    render(<SupportPage qqGroup="1080152144" />)

    expect(screen.getByRole('heading', { name: '联系客服' })).toBeInTheDocument()
    expect(screen.getByText('1080152144')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'QQ群 1080152144 二维码' }))
      .toHaveAttribute('src', '/support/qq-group-1080152144.png')
  })

  it('copies the group number through the shared resilient control', async () => {
    const user = userEvent.setup()
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    render(<SupportPage qqGroup="1080152144" />)

    await user.click(screen.getByRole('button', { name: '复制 QQ 群号' }))

    expect(writeText).toHaveBeenCalledWith('1080152144')
  })
})

afterEach(() => {
  cleanup()
})
