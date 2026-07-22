import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BrandReveal } from './BrandReveal'

afterEach(() => vi.unstubAllGlobals())

describe('BrandReveal', () => {
  it('keeps a static Xingqiao fallback when canvas motion is disabled', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))

    render(<BrandReveal />)

    expect(screen.getByRole('heading', { name: '星桥' })).toBeVisible()
    expect(screen.getByLabelText('星桥品牌揭幕')).toHaveAttribute('data-canvas-active', 'false')
  })
})
