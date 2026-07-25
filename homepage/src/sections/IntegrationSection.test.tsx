import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { DEFAULT_SITE_CONFIG } from '../domain/siteConfig'
import { IntegrationSection } from './IntegrationSection'

describe('IntegrationSection', () => {
  it('reuses the configured Xingqiao base URL without brace icons', () => {
    render(
      <IntegrationSection
        config={{
          ...DEFAULT_SITE_CONFIG,
          apiOrigin: 'https://api.xingqiao.test',
        }}
      />,
    )

    expect(screen.getByText('OPENAI_BASE_URL=https://api.xingqiao.test')).toBeVisible()
    expect(screen.getByText('ANTHROPIC_BASE_URL=https://api.xingqiao.test')).toBeVisible()
    expect(screen.queryByText('https://你的星桥域名')).not.toBeInTheDocument()

    const labels = screen.getAllByTestId('integration-label')
    for (const label of labels) {
      expect(label.querySelectorAll('svg')).toHaveLength(1)
    }
  })
})

afterEach(cleanup)
