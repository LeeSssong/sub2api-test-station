import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { DEFAULT_SITE_CONFIG } from '../domain/siteConfig'
import { ValueSections } from './ValueSections'

describe('ValueSections', () => {
  it('renders the fixed public price and Korea direct-connect promises', () => {
    render(<ValueSections config={DEFAULT_SITE_CONFIG} />)

    expect(screen.getByText('官方价格 100%')).toBeInTheDocument()
    expect(screen.getByText('星桥价格 官方价格的 0.1–0.3 倍')).toBeInTheDocument()
    expect(screen.getByText('额度换算 1 元 = 1 美元额度')).toBeInTheDocument()
    expect(screen.getByText('韩国首尔服务器，国内无需翻墙即可直连')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '复制 QQ 群号' })).toBeInTheDocument()
  })
})
