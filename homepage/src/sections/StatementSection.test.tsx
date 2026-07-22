import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { StatementSection } from './StatementSection'

afterEach(() => vi.unstubAllGlobals())

describe('StatementSection', () => {
  it('shows the complete statement immediately when reduced motion is requested', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))

    render(<StatementSection />)

    expect(screen.getByText('所有顶尖模型。')).toBeVisible()
    expect(screen.getByText('一个网关。')).toBeVisible()
    expect(screen.getByText('国内直连。')).toBeVisible()
    expect(screen.getByLabelText('星桥服务声明')).toHaveAttribute('data-motion-state', 'final')
  })
})
