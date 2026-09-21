import { beforeEach, describe, expect, it } from 'vitest'
import { DEFAULT_SITE_LOGO, updateFavicon } from '@/utils/branding'

describe('updateFavicon', () => {
  beforeEach(() => {
    document.head.innerHTML = '<link rel="icon" href="/logo.svg">'
  })

  it('replaces the default favicon with the configured logo', () => {
    updateFavicon('https://example.com/custom-logo.png')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.href).toBe('https://example.com/custom-logo.png')
  })

  it.each([
    ['https://example.com/logo.png?v=2', 'image/png'],
    ['/uploads/logo.jpeg', 'image/jpeg'],
    ['/uploads/logo.svg?version=2', 'image/svg+xml'],
    ['data:image/png;base64,AA==', 'image/png'],
    ['data:image/svg+xml,%3Csvg%3E%3C/svg%3E', 'image/svg+xml'],
  ])('uses the actual MIME type for %s', (url, mime) => {
    updateFavicon(url)
    expect(document.querySelector('link[rel="icon"]')?.getAttribute('type')).toBe(mime)
  })

  it('resets to the brand fallback when an administrator removes their logo', () => {
    updateFavicon('/uploads/custom.png')
    updateFavicon('')
    expect(document.querySelector('link[rel="icon"]')?.getAttribute('href')).toBe(DEFAULT_SITE_LOGO)
  })

  it('removes competing icons and lets the browser detect unknown MIME types', () => {
    document.head.innerHTML += '<link rel="shortcut icon" href="/old.ico">'
    updateFavicon('/uploads/image?id=2')
    expect(document.querySelectorAll('link[rel="icon"],link[rel="shortcut icon"]')).toHaveLength(1)
    expect(document.querySelector('link[rel="icon"]')?.hasAttribute('type')).toBe(false)
  })

  it('ignores unsafe logo URLs', () => {
    updateFavicon('javascript:alert(1)')

    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]')
    expect(link?.getAttribute('href')).toBe('/logo.svg')
  })
})
