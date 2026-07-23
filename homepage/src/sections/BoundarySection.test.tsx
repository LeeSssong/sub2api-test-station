import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { BoundarySection } from './BoundarySection'

describe('BoundarySection', () => {
  it('uses the shared dark grid theme instead of a standalone light theme', () => {
    render(<BoundarySection />)

    const section = screen.getByRole('region', { name: '边界清晰，承诺才有意义' })
    expect(section).toHaveClass('boundary-band', 'grid-surface')
    expect(section.querySelector('header')).not.toHaveClass('section-intro--light')
  })
})
