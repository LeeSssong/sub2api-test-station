export type MarkdownCopyState = 'idle' | 'copied' | 'failed'

export interface MarkdownCopyLabels {
  idle: string
  copied: string
  failed: string
}

export type MarkdownSupportAction =
  | {
      kind: 'copy'
      text: string
      button: HTMLButtonElement
    }
  | {
      kind: 'preview'
      src: string
      alt: string
      trigger: HTMLButtonElement
    }

function isSafeRelativeAsset(src: string): boolean {
  const trimmed = src.trim()
  if (
    !trimmed
    || /^[a-z][a-z0-9+.-]*:/i.test(trimmed)
    || trimmed.startsWith('//')
    || trimmed.startsWith('/')
  ) {
    return false
  }

  const [pathPart] = trimmed.split(/([?#].*)/, 2)
  return pathPart
    .split('/')
    .filter((part) => part && part !== '.')
    .every((part) => part !== '..' && !part.includes('\\'))
}

export function rewriteRelativeImageSources(
  html: string,
  resolveSource: (src: string) => string,
): string {
  const template = document.createElement('template')
  template.innerHTML = html

  template.content.querySelectorAll<HTMLImageElement>('img[src]').forEach((image) => {
    const source = image.getAttribute('src') ?? ''
    if (isSafeRelativeAsset(source)) {
      image.setAttribute('src', resolveSource(source.trim()))
    }
  })

  return template.innerHTML
}

export function resolveMarkdownSupportAction(
  target: EventTarget | null,
): MarkdownSupportAction | null {
  if (!(target instanceof Element)) return null

  const copyButton = target.closest<HTMLButtonElement>('button[data-copy-text]')
  if (copyButton) {
    const text = copyButton.dataset.copyText?.trim() ?? ''
    return text ? { kind: 'copy', text, button: copyButton } : null
  }

  const previewTrigger = target.closest<HTMLButtonElement>('button[data-image-preview]')
  if (!previewTrigger) return null

  const image = previewTrigger.querySelector<HTMLImageElement>('img[src]')
  if (!image?.src) return null

  return {
    kind: 'preview',
    src: image.src,
    alt: image.alt,
    trigger: previewTrigger,
  }
}

export function setMarkdownCopyState(
  button: HTMLButtonElement,
  state: MarkdownCopyState,
  labels: MarkdownCopyLabels,
) {
  button.dataset.copyState = state
  const label = button.querySelector<HTMLElement>('[data-copy-label]')
  if (label) label.textContent = labels[state]
}
