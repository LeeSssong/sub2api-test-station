import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { phaseForCycleProgress, RequestJourney } from './RequestJourney'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('RequestJourney', () => {
  it('maps one automatic playback cycle to the matching phase', () => {
    expect(phaseForCycleProgress(.12)).toBe('send')
    expect(phaseForCycleProgress(.31)).toBe('route')
    expect(phaseForCycleProgress(.56)).toBe('observe')
    expect(phaseForCycleProgress(.91)).toBe('observe')
    expect(phaseForCycleProgress(.28)).toBe('send')
  })

  it('renders all phases and final metrics without automatic playback in reduced motion', () => {
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
    expect(screen.getByLabelText('一次 API 请求的完整旅程')).toHaveAttribute('data-playback-state', 'static')
    expect(screen.getByLabelText('请求观测指标')).toHaveAttribute(
      'data-telemetry-target',
      'latency-token',
    )
    const tracks = screen.getByLabelText('应用经星桥连接模型通道').querySelectorAll('.map-track')
    expect(tracks[0]).toHaveAttribute('data-flow-direction', 'forward')
    expect(tracks[1]).toHaveAttribute('data-flow-direction', 'forward')
    expect(tracks[1]).toHaveAttribute('data-telemetry-source', 'route')
    expect(tracks[0]).toHaveStyle({ '--track-progress': '1' })
    expect(tracks[1]).toHaveStyle({ '--track-progress': '1' })
  })

  it('waits for the section to enter the viewport before automatic playback', () => {
    render(<RequestJourney />)

    const journey = screen.getByLabelText('一次 API 请求的完整旅程')
    expect(journey).toHaveAttribute('data-journey-mode', 'auto')
    expect(journey).toHaveAttribute('data-playback-state', 'paused')
    expect(journey).not.toHaveAttribute('data-scroll-density')
  })
})
