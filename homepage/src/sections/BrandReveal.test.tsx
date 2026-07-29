import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BrandReveal } from './BrandReveal'

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('BrandReveal', () => {
  it('keeps a static Xingqiao fallback when canvas motion is disabled', () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))

    render(<BrandReveal theme="dark" />)

    expect(screen.getByRole('heading', { name: '星桥' })).toBeVisible()
    expect(screen.getByLabelText('星桥品牌揭幕')).toHaveAttribute('data-canvas-active', 'false')
  })

  it.each([
    ['dark', '#dce4fa'],
    ['light', '#354e78'],
  ] as const)('draws %s theme particles with visible contrast', (theme, expectedColor) => {
    const drawColors: string[] = []
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))
    vi.stubGlobal('IntersectionObserver', class {
      observe() {}
      disconnect() {}
    })
    vi.stubGlobal('requestAnimationFrame', vi.fn().mockReturnValue(1))
    vi.spyOn(Element.prototype, 'getBoundingClientRect').mockReturnValue({
      width: 100,
      height: 100,
      top: 0,
      left: 0,
      right: 100,
      bottom: 100,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(() => {
      const context = {
        fillStyle: '',
        font: '',
        textAlign: '',
        textBaseline: '',
        setTransform: vi.fn(),
        clearRect: vi.fn(),
        fillText: vi.fn(),
        fillRect: vi.fn(function (this: { fillStyle: string }) {
          drawColors.push(this.fillStyle)
        }),
        getImageData: vi.fn().mockReturnValue({
          data: new Uint8ClampedArray(100 * 100 * 4).fill(255),
        }),
      }
      return context as unknown as CanvasRenderingContext2D
    })

    render(<BrandReveal theme={theme} />)

    expect(drawColors).toContain(expectedColor)
    expect(screen.getByLabelText('星桥品牌揭幕')).toHaveAttribute('data-theme', theme)
  })
})
