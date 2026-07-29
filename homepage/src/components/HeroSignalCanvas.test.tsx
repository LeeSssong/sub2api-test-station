import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { applyHomepageTheme } from '../themeBootstrap'
import { HeroSignalCanvas } from './HeroSignalCanvas'

function createCanvasContext() {
  return {
    clearRect: vi.fn(),
    fillText: vi.fn(),
    measureText: vi.fn((text: string) => ({ width: text.length * 7 })),
    setTransform: vi.fn(),
    save: vi.fn(),
    restore: vi.fn(),
    textBaseline: 'middle',
    font: '',
    fillStyle: '',
    globalAlpha: 1,
    shadowBlur: 0,
    shadowColor: '',
  }
}

describe('HeroSignalCanvas', () => {
  beforeEach(() => {
    document.documentElement.dataset.theme = 'dark'
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('renders three layers and redraws when the homepage theme changes', () => {
    const context = createCanvasContext()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context as unknown as CanvasRenderingContext2D)
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))
    vi.stubGlobal('requestAnimationFrame', vi.fn(() => 7))
    vi.stubGlobal('cancelAnimationFrame', vi.fn())

    render(<HeroSignalCanvas active label="实时信号" />)
    const signal = screen.getByRole('img', { name: '实时信号' })
    expect(signal).toHaveAttribute('data-signal-layers', '3')
    const drawsBeforeThemeChange = context.clearRect.mock.calls.length

    applyHomepageTheme('light')

    expect(context.clearRect.mock.calls.length).toBeGreaterThan(drawsBeforeThemeChange)
  })

  it('draws one static layered frame without starting RAF for reduced motion', () => {
    const context = createCanvasContext()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(context as unknown as CanvasRenderingContext2D)
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }))
    const requestAnimationFrame = vi.fn(() => 7)
    vi.stubGlobal('requestAnimationFrame', requestAnimationFrame)
    vi.stubGlobal('cancelAnimationFrame', vi.fn())

    render(<HeroSignalCanvas active label="实时信号" />)

    expect(context.fillText).toHaveBeenCalled()
    expect(requestAnimationFrame).not.toHaveBeenCalled()
    expect(screen.getByRole('img', { name: '实时信号' })).toHaveAttribute('data-canvas-active', 'false')
  })
})
