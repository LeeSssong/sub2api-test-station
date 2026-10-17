import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const source = readFileSync('index.html', 'utf8')

describe('v24 homepage integration', () => {
  it('keeps the confirmed copy and five independent model controls', () => {
    const page = new DOMParser().parseFromString(source, 'text/html')
    expect(page.querySelector('.brand')?.textContent).toBe('星桥AI Link')
    expect(page.querySelector('h1')?.textContent).toBe('星桥，连接世界前沿模型')
    expect([...page.querySelectorAll('button.provider')].map(el => el.getAttribute('data-provider')))
      .toEqual(['codex', 'claude', 'deepseek', 'grok', 'gemini'])
    expect(page.querySelector('.hub .start-button')?.textContent).toBe('开始使用')
    expect(page.querySelector('meta[name="xingqiao-homepage-version"]')?.getAttribute('content')).toBe('24')
  })

  it('keeps the test station entry and documentation on the same origin without enabling registration', () => {
    const page = new DOMParser().parseFromString(source, 'text/html')
    const origin = 'http://43.133.75.82'
    expect(page.querySelector('.start-button')?.getAttribute('href')).toBe('/login')
    expect([...page.querySelectorAll('.site-nav a')].map(el => el.getAttribute('href'))).toEqual(['/', '/docs/'])
    for (const a of page.querySelectorAll('a[href]')) {
      expect(new URL(a.getAttribute('href')!, origin).origin).toBe(origin)
    }
    expect(source).not.toContain('api.xingqiaolab.top')
    expect(source).not.toContain('href="/register"')
    expect(page.querySelector('script[src]')?.getAttribute('type')).toBe('module')
  })
})
