import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RequestJourney } from './RequestJourney'

afterEach(() => vi.unstubAllGlobals())

describe('RequestJourney', () => {
  it('renders all phases and final metrics without scroll scrubbing in reduced motion', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))

    render(<RequestJourney />)

    for (const heading of ['发送', '路由', '观测']) {
      expect(screen.getByRole('heading', { name: heading })).toBeVisible()
    }
    expect(screen.getByText('187')).toBeVisible()
    expect(screen.getByText('2,148')).toBeVisible()
    expect(screen.getByLabelText('一次 API 请求的完整旅程')).toHaveAttribute('data-journey-phase', 'static')
    expect(screen.getByLabelText('一次 API 请求的完整旅程')).toHaveAttribute('data-journey-mode', 'static')
  })
})
