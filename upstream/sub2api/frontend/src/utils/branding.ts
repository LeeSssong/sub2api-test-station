import { sanitizeUrl } from '@/utils/url'

export const DEFAULT_SITE_LOGO = '/xingqiao/logo-brand.png'

/** Use the admin's original image and MIME type, including uploaded data URLs. */
export function updateFavicon(logoUrl: string): void {
  const sanitizedLogoUrl = logoUrl.trim()
    ? sanitizeUrl(logoUrl, { allowRelative: true, allowDataUrl: true })
    : DEFAULT_SITE_LOGO
  if (!sanitizedLogoUrl) return

  const links = Array.from(document.querySelectorAll<HTMLLinkElement>('link[rel="icon"], link[rel="shortcut icon"]'))
  let link = links.shift()
  if (!link) {
    link = document.createElement('link')
    document.head.appendChild(link)
  }
  links.forEach((duplicate) => duplicate.remove())
  link.rel = 'icon'
  const dataMime = sanitizedLogoUrl.match(/^data:(image\/[a-z0-9.+-]+)[;,]/i)?.[1]
  const extension = new URL(sanitizedLogoUrl, document.baseURI).pathname.split('.').pop()?.toLowerCase()
  const mimeByExtension: Record<string, string> = {
    svg: 'image/svg+xml', png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg',
    ico: 'image/x-icon', webp: 'image/webp', gif: 'image/gif',
  }
  const mime = dataMime || mimeByExtension[extension || '']
  if (mime) link.type = mime
  else link.removeAttribute('type')
  link.href = sanitizedLogoUrl
}
